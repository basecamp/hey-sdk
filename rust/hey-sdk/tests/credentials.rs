use std::env;
use std::fs;
use std::path::PathBuf;
use std::sync::Arc;

use chrono::Utc;
use hey_sdk::auth::{AuthStrategy, BearerAuth, TokenProvider};
use hey_sdk::credentials::{
    AuthManager, CredentialStore, Credentials, DefaultCredentialStore, FileCredentialStore,
    InMemoryCredentialStore,
};
use hey_sdk::{Config, ErrorCode, SensitiveString, StaticTokenProvider};
use serde_json::json;
use tokio::sync::Mutex;
use wiremock::matchers::{body_string_contains, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

/// `HEY_TOKEN` and `HEY_NO_KEYRING` belong to the whole process, so every test that reads
/// or writes one takes a turn.
static ENVIRONMENT: Mutex<()> = Mutex::const_new(());

const ORIGIN: &str = "https://app.hey.com";

#[tokio::test]
async fn a_static_provider_refuses_to_hand_out_an_empty_token() {
    assert_eq!(
        "handed-in-token",
        StaticTokenProvider::new("handed-in-token")
            .access_token()
            .await
            .unwrap()
    );

    let error = StaticTokenProvider::new("")
        .access_token()
        .await
        .unwrap_err();
    assert_eq!(ErrorCode::Auth, error.code());
    assert_eq!("no token configured", error.message());
}

#[tokio::test]
async fn bearer_auth_puts_the_token_on_the_request() {
    let auth = BearerAuth::new(StaticTokenProvider::new("bearer-tok"));
    let mut request = reqwest::Request::new(
        reqwest::Method::GET,
        "https://example.com/api".parse().unwrap(),
    );

    auth.authenticate(&mut request).await.unwrap();

    assert_eq!("Bearer bearer-tok", request.headers()["authorization"]);
}

#[tokio::test]
async fn a_token_inside_the_five_minute_skew_is_renewed_and_one_outside_it_is_not() {
    let _environment = environment().await;
    let server = token_server(json!({
        "access_token": "renewed-access",
        "token_type": "Bearer",
        "expires_in": 3600
    }))
    .await;

    let (manager, store, origin) = manager_for(&server, Utc::now().timestamp() + 299).await;
    assert_eq!("renewed-access", manager.access_token().await.unwrap());
    assert_eq!(
        "renewed-access",
        store.load(&origin).unwrap().unwrap().access_token.expose()
    );

    let (fresh, _, _) = manager_for(&server, Utc::now().timestamp() + 400).await;
    assert_eq!("stale-access", fresh.access_token().await.unwrap());
}

#[tokio::test]
async fn the_refresh_token_rotates_only_when_the_server_sends_a_new_one() {
    let _environment = environment().await;

    let quiet = token_server(json!({
        "access_token": "renewed-access",
        "token_type": "Bearer",
        "expires_in": 3600
    }))
    .await;
    let (manager, store, origin) = manager_for(&quiet, 1).await;
    manager.refresh().await.unwrap();
    let kept = store.load(&origin).unwrap().unwrap();
    assert_eq!("renewed-access", kept.access_token.expose());
    assert_eq!(
        Some("stale-refresh"),
        kept.refresh_token.as_ref().map(SensitiveString::expose)
    );

    let rotating = token_server(json!({
        "access_token": "renewed-access",
        "refresh_token": "rotated-refresh",
        "token_type": "Bearer",
        "expires_in": 3600
    }))
    .await;
    let (manager, store, origin) = manager_for(&rotating, 1).await;
    manager.refresh().await.unwrap();
    assert_eq!(
        Some("rotated-refresh"),
        store
            .load(&origin)
            .unwrap()
            .unwrap()
            .refresh_token
            .as_ref()
            .map(SensitiveString::expose)
    );
}

#[tokio::test]
async fn credentials_a_go_client_wrote_with_blank_fields_have_nothing_to_renew_with() {
    let _environment = environment().await;

    let store = Arc::new(InMemoryCredentialStore::new());
    let config = Config::default();
    store
        .save(
            config.origin(),
            &Credentials {
                access_token: "access".into(),
                refresh_token: Some("".into()),
                token_endpoint: Some(String::new()),
                expires_at: Some(0),
                ..Credentials::default()
            },
        )
        .unwrap();
    let manager = AuthManager::new(config, reqwest::Client::new(), store);

    let error = manager.access_token().await.unwrap_err();

    assert_eq!(ErrorCode::Auth, error.code());
    assert_eq!("no refresh token available", error.message());
}

#[tokio::test]
async fn hey_token_beats_the_credentials_on_disk() {
    let _environment = environment().await;

    let config = Config::default();
    let store = Arc::new(InMemoryCredentialStore::new());
    store
        .save(
            config.origin(),
            &Credentials {
                access_token: "stored-access".into(),
                expires_at: Some(Utc::now().timestamp() + 3600),
                ..Credentials::default()
            },
        )
        .unwrap();
    let manager = AuthManager::new(config, reqwest::Client::new(), store);

    unsafe { env::set_var("HEY_TOKEN", "token-from-the-environment") };
    let token = manager.access_token().await;
    unsafe { env::remove_var("HEY_TOKEN") };

    assert_eq!("token-from-the-environment", token.unwrap());
}

#[tokio::test]
async fn logging_out_leaves_nothing_behind() {
    let _environment = environment().await;

    let config = Config::default();
    let store = Arc::new(InMemoryCredentialStore::new());
    let manager = AuthManager::new(config.clone(), reqwest::Client::new(), store.clone());
    manager
        .save(&Credentials {
            access_token: "access".into(),
            ..Credentials::default()
        })
        .unwrap();
    assert!(manager.is_authenticated().await);

    manager.logout().unwrap();

    assert!(!manager.is_authenticated().await);
    assert_eq!(None, store.load(config.origin()).unwrap());
}

#[tokio::test]
async fn the_user_id_is_read_and_written_beside_the_tokens() {
    let _environment = environment().await;

    let config = Config::default();
    let store = Arc::new(InMemoryCredentialStore::new());
    let manager = AuthManager::new(config.clone(), reqwest::Client::new(), store.clone());
    manager
        .save(&Credentials {
            access_token: "access".into(),
            ..Credentials::default()
        })
        .unwrap();

    assert_eq!(None, manager.user_id().unwrap());

    manager.set_user_id(Some("user-42".to_string())).unwrap();
    assert_eq!(Some("user-42".to_string()), manager.user_id().unwrap());
    assert_eq!(
        Some("user-42".to_string()),
        store.load(config.origin()).unwrap().unwrap().user_id
    );
    assert_eq!(
        "access",
        manager.load().unwrap().unwrap().access_token.expose()
    );

    manager.set_user_id(None).unwrap();
    assert_eq!(None, manager.user_id().unwrap());
}

#[tokio::test]
async fn setting_a_user_id_without_credentials_says_so() {
    let _environment = environment().await;

    let manager = AuthManager::new(
        Config::default(),
        reqwest::Client::new(),
        Arc::new(InMemoryCredentialStore::new()),
    );

    assert_eq!(None, manager.user_id().unwrap());
    let error = manager
        .set_user_id(Some("user-42".to_string()))
        .unwrap_err();
    assert_eq!(ErrorCode::Auth, error.code());
}

#[tokio::test]
async fn the_manager_hands_back_the_store_it_was_built_with() {
    let _environment = environment().await;

    let config = Config::default();
    let store = Arc::new(InMemoryCredentialStore::new());
    let manager = AuthManager::new(config.clone(), reqwest::Client::new(), store);

    manager
        .save(&Credentials {
            access_token: "access".into(),
            ..Credentials::default()
        })
        .unwrap();

    assert_eq!(
        "access",
        manager
            .store()
            .load(config.origin())
            .unwrap()
            .unwrap()
            .access_token
            .expose()
    );
}

#[tokio::test]
async fn hey_no_keyring_takes_the_file_store() {
    let _environment = environment().await;
    let directory = scratch_dir();

    unsafe { env::set_var("HEY_NO_KEYRING", "1") };
    let store = DefaultCredentialStore::open(&directory);
    unsafe { env::remove_var("HEY_NO_KEYRING") };

    assert!(!store.using_keyring());

    store
        .save(
            ORIGIN,
            &Credentials {
                access_token: "access-tok".into(),
                refresh_token: Some("refresh-tok".into()),
                expires_at: Some(9_999_999_999),
                scope: Some("full".to_string()),
                user_id: Some("user-1".to_string()),
                ..Credentials::default()
            },
        )
        .unwrap();

    let loaded = store.load(ORIGIN).unwrap().unwrap();
    assert_eq!("access-tok", loaded.access_token.expose());
    assert_eq!(Some("user-1".to_string()), loaded.user_id);
    assert_eq!(None, store.load("https://nowhere.example.com").unwrap());

    store.delete(ORIGIN).unwrap();
    assert_eq!(None, store.load(ORIGIN).unwrap());

    fs::remove_dir_all(&directory).unwrap();
}

#[test]
fn the_file_store_keeps_every_origin_apart() {
    let directory = scratch_dir();
    let store = FileCredentialStore::new(directory.join("credentials.json"));

    store
        .save(
            "https://a.example.com",
            &Credentials {
                access_token: "tok-a".into(),
                ..Credentials::default()
            },
        )
        .unwrap();
    store
        .save(
            "https://b.example.com",
            &Credentials {
                access_token: "tok-b".into(),
                ..Credentials::default()
            },
        )
        .unwrap();

    assert_eq!(
        "tok-a",
        store
            .load("https://a.example.com")
            .unwrap()
            .unwrap()
            .access_token
            .expose()
    );
    assert_eq!(
        "tok-b",
        store
            .load("https://b.example.com")
            .unwrap()
            .unwrap()
            .access_token
            .expose()
    );

    store.delete("https://a.example.com").unwrap();
    assert_eq!(None, store.load("https://a.example.com").unwrap());
    assert_eq!(
        "tok-b",
        store
            .load("https://b.example.com")
            .unwrap()
            .unwrap()
            .access_token
            .expose()
    );

    fs::remove_dir_all(&directory).unwrap();
}

#[test]
fn the_file_store_reads_what_the_go_sdk_wrote() {
    let directory = scratch_dir();
    let path = directory.join("credentials.json");
    fs::create_dir_all(&directory).unwrap();
    fs::write(
        &path,
        r#"{
  "https://app.hey.com": {
    "access_token": "access",
    "refresh_token": "refresh",
    "expires_at": 1800000000,
    "scope": "full",
    "token_endpoint": "https://app.hey.com/oauth/token",
    "user_id": "1234"
  }
}"#,
    )
    .unwrap();

    let loaded = FileCredentialStore::new(&path)
        .load(ORIGIN)
        .unwrap()
        .unwrap();

    assert_eq!("access", loaded.access_token.expose());
    assert_eq!(
        Some("refresh"),
        loaded.refresh_token.as_ref().map(SensitiveString::expose)
    );
    assert_eq!(Some(1_800_000_000), loaded.expires_at);
    assert_eq!(Some("full".to_string()), loaded.scope);
    assert_eq!(
        Some("https://app.hey.com/oauth/token".to_string()),
        loaded.token_endpoint
    );
    assert_eq!(Some("1234".to_string()), loaded.user_id);

    fs::remove_dir_all(&directory).unwrap();
}

#[test]
fn the_file_store_writes_what_the_go_sdk_can_read() {
    let directory = scratch_dir();
    let path = directory.join("credentials.json");

    FileCredentialStore::new(&path)
        .save(
            ORIGIN,
            &Credentials {
                access_token: "access".into(),
                refresh_token: Some("refresh".into()),
                expires_at: Some(1_800_000_000),
                scope: Some("full".to_string()),
                token_endpoint: Some("https://app.hey.com/oauth/token".to_string()),
                user_id: Some("1234".to_string()),
            },
        )
        .unwrap();

    let written: serde_json::Value = serde_json::from_slice(&fs::read(&path).unwrap()).unwrap();

    assert_eq!(
        json!({
            "https://app.hey.com": {
                "access_token": "access",
                "refresh_token": "refresh",
                "expires_at": 1800000000,
                "scope": "full",
                "token_endpoint": "https://app.hey.com/oauth/token",
                "user_id": "1234"
            }
        }),
        written
    );

    fs::remove_dir_all(&directory).unwrap();
}

async fn environment() -> tokio::sync::MutexGuard<'static, ()> {
    let guard = ENVIRONMENT.lock().await;
    unsafe { env::remove_var("HEY_TOKEN") };
    unsafe { env::remove_var("HEY_NO_KEYRING") };
    guard
}

async fn token_server(response: serde_json::Value) -> MockServer {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/token"))
        .and(body_string_contains("grant_type=refresh_token"))
        .and(body_string_contains("refresh_token=stale-refresh"))
        .respond_with(ResponseTemplate::new(200).set_body_json(response))
        .mount(&server)
        .await;
    server
}

async fn manager_for(
    server: &MockServer,
    expires_at: i64,
) -> (AuthManager, Arc<InMemoryCredentialStore>, String) {
    let config = Config::default().with_base_url(server.uri());
    let origin = config.origin().to_string();
    let store = Arc::new(InMemoryCredentialStore::new());
    store
        .save(
            &origin,
            &Credentials {
                access_token: "stale-access".into(),
                refresh_token: Some("stale-refresh".into()),
                expires_at: Some(expires_at),
                token_endpoint: Some(format!("{}/token", server.uri())),
                ..Credentials::default()
            },
        )
        .unwrap();

    let manager = AuthManager::new(config, reqwest::Client::new(), store.clone());
    (manager, store, origin)
}

fn scratch_dir() -> PathBuf {
    let suffix = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    env::temp_dir().join(format!("hey-sdk-credentials-{suffix}"))
}
