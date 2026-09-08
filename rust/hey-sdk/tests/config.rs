use std::env;
use std::fs;
use std::path::PathBuf;
use std::sync::{Mutex, MutexGuard};

use hey_sdk::Config;
use hey_sdk::config::{
    DEFAULT_BASE_URL, DEFAULT_OAUTH_CLIENT_ID, default_cache_dir, default_config_dir,
};

/// The environment belongs to the whole process, so every test that reads or writes it
/// takes a turn.
static ENVIRONMENT: Mutex<()> = Mutex::new(());

const VARIABLES: [&str; 6] = [
    "HEY_BASE_URL",
    "HEY_OAUTH_CLIENT_ID",
    "HEY_CACHE_DIR",
    "HEY_CACHE_ENABLED",
    "XDG_CACHE_HOME",
    "XDG_CONFIG_HOME",
];

#[test]
fn the_defaults_point_at_hey_with_the_cache_off() {
    let _environment = environment();

    let config = Config::default();

    assert_eq!(DEFAULT_BASE_URL, config.base_url);
    assert_eq!("https://app.hey.com", config.base_url);
    assert_eq!(DEFAULT_OAUTH_CLIENT_ID, config.oauth_client_id);
    assert!(!config.oauth_client_id.is_empty());
    assert!(!config.cache_enabled);
    assert!(config.cache_dir.ends_with("hey"));
}

#[test]
fn a_missing_file_reads_as_the_defaults() {
    let _environment = environment();
    let directory = scratch_dir();

    let config = Config::load(&directory.join("missing.json")).unwrap();

    assert_eq!(Config::default(), config);
}

#[test]
fn a_config_file_only_overrides_the_keys_it_sets() {
    let _environment = environment();
    let directory = scratch_dir();
    let path = directory.join("config.json");
    fs::create_dir_all(&directory).unwrap();
    fs::write(
        &path,
        r#"{"base_url":"https://custom.hey.com","cache_enabled":true}"#,
    )
    .unwrap();

    let config = Config::load(&path).unwrap();

    assert_eq!("https://custom.hey.com", config.base_url);
    assert!(config.cache_enabled);
    assert_eq!(DEFAULT_OAUTH_CLIENT_ID, config.oauth_client_id);
    assert_eq!(default_cache_dir(), config.cache_dir);

    fs::remove_dir_all(&directory).unwrap();
}

#[test]
fn a_malformed_config_file_is_a_usage_error_naming_the_path() {
    let _environment = environment();
    let directory = scratch_dir();
    let path = directory.join("config.json");
    fs::create_dir_all(&directory).unwrap();
    fs::write(&path, "{bad json").unwrap();

    let error = Config::load(&path).unwrap_err();

    assert_eq!(hey_sdk::ErrorCode::Usage, error.code());
    assert!(
        error.message().contains(&path.display().to_string()),
        "{error}"
    );

    fs::remove_dir_all(&directory).unwrap();
}

#[test]
fn the_environment_overrides_everything_the_file_set() {
    let _environment = environment();
    set("HEY_BASE_URL", "https://env.hey.com");
    set("HEY_OAUTH_CLIENT_ID", "env-client-id");
    set("HEY_CACHE_DIR", "/tmp/hey-cache");
    set("HEY_CACHE_ENABLED", "true");

    let config = Config::default()
        .with_base_url("https://from-the-file.hey.com")
        .with_env();

    assert_eq!("https://env.hey.com", config.base_url);
    assert_eq!("env-client-id", config.oauth_client_id);
    assert_eq!(PathBuf::from("/tmp/hey-cache"), config.cache_dir);
    assert!(config.cache_enabled);

    clear();
}

#[test]
fn the_cache_is_switched_on_by_true_or_one_and_off_by_anything_else() {
    let _environment = environment();

    for value in ["true", "TRUE", "True", "1"] {
        set("HEY_CACHE_ENABLED", value);
        assert!(Config::default().with_env().cache_enabled, "{value}");
    }

    for value in ["false", "FALSE", "0", "yes"] {
        set("HEY_CACHE_ENABLED", value);
        assert!(
            !Config::default()
                .with_cache_enabled(true)
                .with_env()
                .cache_enabled,
            "{value}"
        );
    }

    clear();
}

#[test]
fn an_empty_variable_counts_as_unset() {
    let _environment = environment();
    set("HEY_BASE_URL", "");
    set("HEY_OAUTH_CLIENT_ID", "");
    set("HEY_CACHE_DIR", "");
    set("HEY_CACHE_ENABLED", "");

    let config = Config::default().with_cache_enabled(true).with_env();

    assert_eq!(DEFAULT_BASE_URL, config.base_url);
    assert_eq!(DEFAULT_OAUTH_CLIENT_ID, config.oauth_client_id);
    assert_eq!(default_cache_dir(), config.cache_dir);
    assert!(config.cache_enabled);

    clear();
}

#[test]
fn the_xdg_directories_decide_where_the_cache_and_the_credentials_live() {
    let _environment = environment();
    set("XDG_CACHE_HOME", "/tmp/xdg-cache");
    set("XDG_CONFIG_HOME", "/tmp/xdg-config");

    assert_eq!(PathBuf::from("/tmp/xdg-cache/hey"), default_cache_dir());
    assert_eq!(
        PathBuf::from("/tmp/xdg-cache/hey"),
        Config::default().cache_dir
    );
    assert_eq!(PathBuf::from("/tmp/xdg-config/hey"), default_config_dir());

    clear();

    let home = env::home_dir().unwrap();
    assert_eq!(home.join(".cache/hey"), default_cache_dir());
    assert_eq!(home.join(".config/hey"), default_config_dir());
}

#[test]
fn the_origin_is_the_base_url_without_its_trailing_slash() {
    let _environment = environment();

    assert_eq!(
        "https://example.com",
        Config::default()
            .with_base_url("https://example.com/")
            .origin()
    );
    assert_eq!(
        "https://example.com",
        Config::default()
            .with_base_url("https://example.com")
            .origin()
    );
}

fn environment() -> MutexGuard<'static, ()> {
    let guard = ENVIRONMENT
        .lock()
        .unwrap_or_else(|poison| poison.into_inner());
    clear();
    guard
}

fn set(name: &str, value: &str) {
    unsafe { env::set_var(name, value) };
}

fn clear() {
    for name in VARIABLES {
        unsafe { env::remove_var(name) };
    }
}

fn scratch_dir() -> PathBuf {
    let suffix = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    env::temp_dir().join(format!("hey-sdk-config-{suffix}"))
}
