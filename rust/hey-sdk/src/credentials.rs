use std::collections::HashMap;
use std::env;
use std::fmt;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use chrono::Utc;
use serde::{Deserialize, Serialize};
use tokio::sync::Mutex as AsyncMutex;

use crate::auth::TokenProvider;
use crate::config::{Config, default_config_dir};
use crate::error::Error;
use crate::oauth::{OAuthClient, RefreshRequest};
use crate::types::SensitiveString;

/// How long before it expires a token is treated as spent.
const REFRESH_LEEWAY_SECONDS: i64 = 300;

/// The secret store every HEY SDK keeps its credentials under.
const KEYRING_SERVICE: &str = "hey-sdk";

/// The tokens held for one HEY origin, and what is needed to renew them.
///
/// The JSON is the contract hey-cli and the Go SDK read and write too, so `user_id` is a
/// string here as it is there. The two tokens are [`SensitiveString`]s, which are
/// serde-transparent — the file is unchanged — but print as `[REDACTED]`, so a `{:?}` of
/// the whole record cannot put credentials in a log.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Credentials {
    pub access_token: SensitiveString,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub refresh_token: Option<SensitiveString>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub scope: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub token_endpoint: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user_id: Option<String>,
}

/// Where credentials live between runs, keyed by the origin they belong to.
pub trait CredentialStore: Send + Sync {
    fn load(&self, origin: &str) -> Result<Option<Credentials>, Error>;
    fn save(&self, origin: &str, credentials: &Credentials) -> Result<(), Error>;
    fn delete(&self, origin: &str) -> Result<(), Error>;
}

/// Keeps every origin's credentials in one JSON file, readable only by its owner.
#[derive(Debug, Clone)]
pub struct FileCredentialStore {
    path: PathBuf,
}

impl FileCredentialStore {
    pub fn new(path: impl Into<PathBuf>) -> FileCredentialStore {
        FileCredentialStore { path: path.into() }
    }

    pub fn default_location() -> FileCredentialStore {
        FileCredentialStore::new(default_config_dir().join("credentials.json"))
    }

    fn read_all(&self) -> Result<HashMap<String, Credentials>, Error> {
        match fs::read(&self.path) {
            Ok(bytes) => serde_json::from_slice(&bytes).map_err(|error| {
                Error::usage(format!("{}: {error}", self.path.display())).with_source(error)
            }),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(HashMap::new()),
            Err(error) => Err(file_error(&self.path, error)),
        }
    }

    /// Writes the whole file out beside itself and renames it over the original, so a
    /// reader never sees a half-written set of credentials.
    fn write_all(&self, all: &HashMap<String, Credentials>) -> Result<(), Error> {
        let directory = self.path.parent().unwrap_or_else(|| Path::new("."));
        create_private_dir(directory).map_err(|error| file_error(directory, error))?;

        let temporary = temporary_path(&self.path);
        let json = serde_json::to_vec_pretty(all)?;
        if let Err(error) = write_private_file(&temporary, &json) {
            fs::remove_file(&temporary).ok();
            return Err(file_error(&temporary, error));
        }
        if let Err(error) = fs::rename(&temporary, &self.path) {
            fs::remove_file(&temporary).ok();
            return Err(file_error(&self.path, error));
        }
        Ok(())
    }
}

impl CredentialStore for FileCredentialStore {
    fn load(&self, origin: &str) -> Result<Option<Credentials>, Error> {
        Ok(self.read_all()?.remove(origin))
    }

    fn save(&self, origin: &str, credentials: &Credentials) -> Result<(), Error> {
        let mut all = self.read_all()?;
        all.insert(origin.to_string(), credentials.clone());
        self.write_all(&all)
    }

    fn delete(&self, origin: &str) -> Result<(), Error> {
        let mut all = self.read_all()?;
        all.remove(origin);
        self.write_all(&all)
    }
}

/// Keeps credentials for the life of the process: for tests, and for applications that
/// hold on to them themselves.
#[derive(Debug, Default)]
pub struct InMemoryCredentialStore {
    entries: Mutex<HashMap<String, Credentials>>,
}

impl InMemoryCredentialStore {
    pub fn new() -> InMemoryCredentialStore {
        InMemoryCredentialStore::default()
    }
}

impl CredentialStore for InMemoryCredentialStore {
    fn load(&self, origin: &str) -> Result<Option<Credentials>, Error> {
        Ok(self.entries.lock().unwrap().get(origin).cloned())
    }

    fn save(&self, origin: &str, credentials: &Credentials) -> Result<(), Error> {
        self.entries
            .lock()
            .unwrap()
            .insert(origin.to_string(), credentials.clone());
        Ok(())
    }

    fn delete(&self, origin: &str) -> Result<(), Error> {
        self.entries.lock().unwrap().remove(origin);
        Ok(())
    }
}

/// Keeps credentials in the operating system's own secret store: the Keychain on macOS,
/// the Credential Manager on Windows, the Secret Service on the other Unixes. Each origin
/// is one entry, holding the same JSON the file store writes.
#[derive(Debug, Clone, Default)]
pub struct KeyringCredentialStore;

impl KeyringCredentialStore {
    pub fn new() -> KeyringCredentialStore {
        KeyringCredentialStore
    }

    /// Whether the platform's secret store answers, decided the way the Go SDK decides it:
    /// by writing a throwaway entry and taking it straight back out.
    pub fn is_available() -> bool {
        match keyring_entry("test") {
            Ok(entry) => {
                let wrote = entry.set_password("test").is_ok();
                if wrote {
                    entry.delete_credential().ok();
                }
                wrote
            }
            Err(_) => false,
        }
    }
}

impl CredentialStore for KeyringCredentialStore {
    fn load(&self, origin: &str) -> Result<Option<Credentials>, Error> {
        match keyring_entry(origin)?.get_password() {
            Ok(secret) => serde_json::from_str(&secret).map(Some).map_err(|error| {
                Error::usage(format!("keyring credentials for {origin}: {error}"))
                    .with_source(error)
            }),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(error) => Err(keyring_error(error)),
        }
    }

    fn save(&self, origin: &str, credentials: &Credentials) -> Result<(), Error> {
        let secret = serde_json::to_string(credentials)?;
        keyring_entry(origin)?
            .set_password(&secret)
            .map_err(keyring_error)
    }

    fn delete(&self, origin: &str) -> Result<(), Error> {
        match keyring_entry(origin)?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(error) => Err(keyring_error(error)),
        }
    }
}

/// The store an application gets when it does not pick one: the platform's secret store
/// when it answers, and a private JSON file under `fallback_dir` when it does not. Setting
/// `HEY_NO_KEYRING` to anything non-empty takes the file every time.
pub struct DefaultCredentialStore {
    inner: Box<dyn CredentialStore>,
    using_keyring: bool,
}

impl DefaultCredentialStore {
    pub fn open(fallback_dir: impl Into<PathBuf>) -> DefaultCredentialStore {
        if wants_keyring() && KeyringCredentialStore::is_available() {
            DefaultCredentialStore {
                inner: Box::new(KeyringCredentialStore::new()),
                using_keyring: true,
            }
        } else {
            let path = fallback_dir.into().join("credentials.json");
            DefaultCredentialStore {
                inner: Box::new(FileCredentialStore::new(path)),
                using_keyring: false,
            }
        }
    }

    /// The store rooted at the config directory hey-cli and the Go SDK share.
    pub fn default_location() -> DefaultCredentialStore {
        DefaultCredentialStore::open(default_config_dir())
    }

    pub fn using_keyring(&self) -> bool {
        self.using_keyring
    }
}

impl CredentialStore for DefaultCredentialStore {
    fn load(&self, origin: &str) -> Result<Option<Credentials>, Error> {
        self.inner.load(origin)
    }

    fn save(&self, origin: &str, credentials: &Credentials) -> Result<(), Error> {
        self.inner.save(origin, credentials)
    }

    fn delete(&self, origin: &str) -> Result<(), Error> {
        self.inner.delete(origin)
    }
}

impl fmt::Debug for DefaultCredentialStore {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("DefaultCredentialStore")
            .field("using_keyring", &self.using_keyring)
            .finish()
    }
}

/// Hands out an access token for the configured origin, renewing it when it is close to
/// expiring. `HEY_TOKEN` wins over anything stored.
pub struct AuthManager {
    config: Config,
    store: Arc<dyn CredentialStore>,
    oauth: OAuthClient,
    lock: AsyncMutex<()>,
}

impl AuthManager {
    pub fn new(
        config: Config,
        http: reqwest::Client,
        store: Arc<dyn CredentialStore>,
    ) -> AuthManager {
        AuthManager {
            config,
            store,
            oauth: OAuthClient::new(http),
            lock: AsyncMutex::new(()),
        }
    }

    pub async fn access_token(&self) -> Result<String, Error> {
        match env_token() {
            Some(token) => Ok(token),
            None => self.stored_access_token().await,
        }
    }

    pub async fn refresh(&self) -> Result<(), Error> {
        let _guard = self.lock.lock().await;

        let credentials = self.load()?.ok_or_else(not_authenticated)?;
        self.renew(credentials).await?;
        Ok(())
    }

    pub async fn is_authenticated(&self) -> bool {
        if env_token().is_some() {
            true
        } else {
            let _guard = self.lock.lock().await;
            match self.load() {
                Ok(Some(credentials)) => !credentials.access_token.is_empty(),
                Ok(None) | Err(_) => false,
            }
        }
    }

    pub fn logout(&self) -> Result<(), Error> {
        self.store.delete(self.config.origin())
    }

    pub fn save(&self, credentials: &Credentials) -> Result<(), Error> {
        self.store.save(self.config.origin(), credentials)
    }

    pub fn load(&self) -> Result<Option<Credentials>, Error> {
        self.store.load(self.config.origin())
    }

    pub fn store(&self) -> &Arc<dyn CredentialStore> {
        &self.store
    }

    /// The user id stored beside the tokens, which hey-cli reads and writes too.
    pub fn user_id(&self) -> Result<Option<String>, Error> {
        Ok(self.load()?.and_then(|credentials| credentials.user_id))
    }

    pub fn set_user_id(&self, user_id: Option<String>) -> Result<(), Error> {
        let mut credentials = self.load()?.ok_or_else(not_authenticated)?;
        credentials.user_id = user_id;
        self.save(&credentials)
    }

    /// Holds the lock across the renewal so concurrent callers make one request between
    /// them: whoever arrives second reads the token the first one stored.
    async fn stored_access_token(&self) -> Result<String, Error> {
        let _guard = self.lock.lock().await;

        let credentials = self.load()?.ok_or_else(not_authenticated)?;
        if is_expiring(&credentials) {
            let renewed = self.renew(credentials).await?;
            Ok(renewed.access_token.into_inner())
        } else {
            Ok(credentials.access_token.into_inner())
        }
    }

    /// The Go SDK and hey-cli write the whole record every time, so a credentials file
    /// they wrote carries `""` where this crate would leave the field out. Either way
    /// there is nothing to renew with.
    async fn renew(&self, mut credentials: Credentials) -> Result<Credentials, Error> {
        let refresh_token = stored_secret(&credentials.refresh_token).ok_or_else(|| {
            Error::auth("no refresh token available").with_hint("Run login again")
        })?;
        let token_endpoint = stored_value(&credentials.token_endpoint)
            .ok_or_else(|| Error::auth("no token endpoint stored").with_hint("Run login again"))?;

        let request = RefreshRequest {
            token_endpoint,
            refresh_token,
            client_id: self.config.oauth_client_id.clone(),
            client_secret: None,
        };
        let token = self.oauth.refresh(&request).await?;

        credentials.access_token = token.access_token;
        if let Some(rotated) = token.refresh_token {
            credentials.refresh_token = Some(rotated);
        }
        if let Some(expires_at) = token.expires_at {
            credentials.expires_at = Some(expires_at.timestamp());
        }

        self.save(&credentials)?;
        Ok(credentials)
    }
}

#[async_trait]
impl TokenProvider for AuthManager {
    async fn access_token(&self) -> Result<String, Error> {
        AuthManager::access_token(self).await
    }

    async fn refresh(&self) -> bool {
        AuthManager::refresh(self).await.is_ok()
    }
}

fn keyring_entry(origin: &str) -> Result<keyring::Entry, Error> {
    keyring::Entry::new(KEYRING_SERVICE, &format!("hey-sdk::{origin}")).map_err(keyring_error)
}

fn keyring_error(error: keyring::Error) -> Error {
    Error::usage(format!("keyring: {error}")).with_source(error)
}

fn wants_keyring() -> bool {
    match env::var("HEY_NO_KEYRING") {
        Ok(value) => value.is_empty(),
        Err(_) => true,
    }
}

fn env_token() -> Option<String> {
    env::var("HEY_TOKEN").ok().filter(|token| !token.is_empty())
}

fn not_authenticated() -> Error {
    Error::auth("not authenticated").with_hint("Run login first")
}

fn stored_value(field: &Option<String>) -> Option<String> {
    field.clone().filter(|value| !value.is_empty())
}

fn stored_secret(field: &Option<SensitiveString>) -> Option<SensitiveString> {
    field.clone().filter(|value| !value.is_empty())
}

fn is_expiring(credentials: &Credentials) -> bool {
    match credentials.expires_at {
        Some(expires_at) => Utc::now().timestamp() >= expires_at - REFRESH_LEEWAY_SECONDS,
        None => false,
    }
}

fn temporary_path(path: &Path) -> PathBuf {
    let mut temporary = path.to_path_buf().into_os_string();
    temporary.push(format!(".{}.tmp", std::process::id()));
    PathBuf::from(temporary)
}

fn create_private_dir(path: &Path) -> std::io::Result<()> {
    let mut builder = fs::DirBuilder::new();
    builder.recursive(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::DirBuilderExt;
        builder.mode(0o700);
    }
    match builder.create(path) {
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => Ok(()),
        result => result,
    }
}

fn write_private_file(path: &Path, contents: &[u8]) -> std::io::Result<()> {
    let mut options = fs::OpenOptions::new();
    options.write(true).create(true).truncate(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }

    let mut file = options.open(path)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        file.set_permissions(fs::Permissions::from_mode(0o600))?;
    }
    file.write_all(contents)?;
    file.sync_all()
}

fn file_error(path: &Path, error: std::io::Error) -> Error {
    Error::usage(format!("{}: {error}", path.display())).with_source(error)
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use wiremock::matchers::{body_string_contains, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use super::*;

    /// `HEY_TOKEN` is process-wide, so every test that reads it takes a turn.
    static ENVIRONMENT: AsyncMutex<()> = AsyncMutex::const_new(());

    #[tokio::test]
    async fn expiring_credentials_are_renewed_and_stored() {
        let _environment = ENVIRONMENT.lock().await;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/token"))
            .and(body_string_contains("grant_type=refresh_token"))
            .and(body_string_contains("refresh_token=stale-refresh"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "access_token": "renewed-access",
                "refresh_token": "rotated-refresh",
                "token_type": "Bearer",
                "expires_in": 3600
            })))
            .mount(&server)
            .await;

        let config = Config::default().with_base_url(server.uri());
        let store = Arc::new(InMemoryCredentialStore::new());
        store
            .save(
                config.origin(),
                &Credentials {
                    access_token: "stale-access".into(),
                    refresh_token: Some("stale-refresh".into()),
                    expires_at: Some(Utc::now().timestamp() + 60),
                    token_endpoint: Some(format!("{}/token", server.uri())),
                    ..Credentials::default()
                },
            )
            .unwrap();

        let manager = AuthManager::new(config.clone(), reqwest::Client::new(), store.clone());
        assert_eq!("renewed-access", manager.access_token().await.unwrap());

        let stored = store.load(config.origin()).unwrap().unwrap();
        assert_eq!("renewed-access", stored.access_token.expose());
        assert_eq!(
            Some("rotated-refresh"),
            stored.refresh_token.as_ref().map(SensitiveString::expose)
        );
        assert!(stored.expires_at.unwrap() > Utc::now().timestamp());
    }

    #[tokio::test]
    async fn a_live_token_is_handed_back_untouched() {
        let _environment = ENVIRONMENT.lock().await;

        let server = MockServer::start().await;
        let config = Config::default().with_base_url(server.uri());
        let store = Arc::new(InMemoryCredentialStore::new());
        store
            .save(
                config.origin(),
                &Credentials {
                    access_token: "live-access".into(),
                    refresh_token: Some("live-refresh".into()),
                    expires_at: Some(Utc::now().timestamp() + 3600),
                    token_endpoint: Some(format!("{}/token", server.uri())),
                    ..Credentials::default()
                },
            )
            .unwrap();

        let manager = AuthManager::new(config, reqwest::Client::new(), store);

        assert_eq!("live-access", manager.access_token().await.unwrap());
        assert!(server.received_requests().await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn an_empty_store_is_not_authenticated() {
        let _environment = ENVIRONMENT.lock().await;

        let manager = AuthManager::new(
            Config::default(),
            reqwest::Client::new(),
            Arc::new(InMemoryCredentialStore::new()),
        );
        let error = manager.access_token().await.unwrap_err();

        assert_eq!(crate::error::ErrorCode::Auth, error.code());
        assert_eq!("not authenticated", error.message());
        assert_eq!(Some("Run login first"), error.hint());
    }

    #[tokio::test]
    async fn hey_token_wins_over_the_store() {
        let _environment = ENVIRONMENT.lock().await;

        let store = Arc::new(InMemoryCredentialStore::new());
        let manager = AuthManager::new(Config::default(), reqwest::Client::new(), store);

        unsafe { env::set_var("HEY_TOKEN", "hey-token-from-the-environment") };
        let token = manager.access_token().await;
        let authenticated = manager.is_authenticated().await;
        unsafe { env::remove_var("HEY_TOKEN") };

        assert_eq!("hey-token-from-the-environment", token.unwrap());
        assert!(authenticated);
    }

    #[tokio::test]
    async fn logout_deletes_the_stored_credentials() {
        let config = Config::default();
        let store = Arc::new(InMemoryCredentialStore::new());
        let manager = AuthManager::new(config.clone(), reqwest::Client::new(), store.clone());

        manager
            .save(&Credentials {
                access_token: "access".into(),
                ..Credentials::default()
            })
            .unwrap();
        assert!(store.load(config.origin()).unwrap().is_some());

        manager.logout().unwrap();
        assert_eq!(None, store.load(config.origin()).unwrap());
    }

    #[test]
    fn the_file_store_round_trips_credentials() {
        let directory = env::temp_dir().join(format!("hey-sdk-{}", rand::random::<u64>()));
        let store = FileCredentialStore::new(directory.join("credentials.json"));
        let credentials = Credentials {
            access_token: "access".into(),
            refresh_token: Some("refresh".into()),
            expires_at: Some(1_800_000_000),
            token_endpoint: Some("https://app.hey.com/token".to_string()),
            user_id: Some("42".to_string()),
            ..Credentials::default()
        };

        store.save("https://app.hey.com", &credentials).unwrap();
        store
            .save("https://staging.example.com", &Credentials::default())
            .unwrap();

        assert_eq!(
            Some(credentials),
            store.load("https://app.hey.com").unwrap()
        );
        assert_eq!(None, store.load("https://nothing.example.com").unwrap());

        store.delete("https://app.hey.com").unwrap();
        assert_eq!(None, store.load("https://app.hey.com").unwrap());
        assert_eq!(
            Some(Credentials::default()),
            store.load("https://staging.example.com").unwrap()
        );

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let file = fs::metadata(directory.join("credentials.json")).unwrap();
            assert_eq!(0o600, file.permissions().mode() & 0o777);
            assert_eq!(
                0o700,
                fs::metadata(&directory).unwrap().permissions().mode() & 0o777
            );
        }

        fs::remove_dir_all(&directory).unwrap();
    }

    #[test]
    fn debugging_credentials_shows_no_tokens() {
        let credentials = Credentials {
            access_token: "super-secret-access".into(),
            refresh_token: Some("super-secret-refresh".into()),
            expires_at: Some(1_800_000_000),
            token_endpoint: Some("https://app.hey.com/token".to_string()),
            user_id: Some("42".to_string()),
            ..Credentials::default()
        };

        let printed = format!("{credentials:?}");

        assert!(!printed.contains("super-secret-access"), "{printed}");
        assert!(!printed.contains("super-secret-refresh"), "{printed}");
        assert!(printed.contains("[REDACTED]"), "{printed}");
        assert!(printed.contains("https://app.hey.com/token"), "{printed}");
        assert_eq!(
            r#"{"access_token":"super-secret-access","refresh_token":"super-secret-refresh","expires_at":1800000000,"token_endpoint":"https://app.hey.com/token","user_id":"42"}"#,
            serde_json::to_string(&credentials).unwrap()
        );
    }

    #[test]
    fn a_missing_file_reads_as_no_credentials() {
        let store =
            FileCredentialStore::new(env::temp_dir().join("hey-sdk-nothing-here/credentials.json"));

        assert_eq!(None, store.load("https://app.hey.com").unwrap());
    }
}
