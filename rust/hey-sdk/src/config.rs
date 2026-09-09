use std::env;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

use crate::error::Error;

pub const DEFAULT_BASE_URL: &str = "https://app.hey.com";
pub const DEFAULT_OAUTH_CLIENT_ID: &str = "khMWSVDVSq78oyKA3KtxmYRv";

/// What a client needs to know before it can talk to HEY.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct Config {
    pub base_url: String,
    pub oauth_client_id: String,
    pub cache_dir: PathBuf,
    pub cache_enabled: bool,
}

impl Default for Config {
    fn default() -> Config {
        Config {
            base_url: DEFAULT_BASE_URL.to_string(),
            oauth_client_id: DEFAULT_OAUTH_CLIENT_ID.to_string(),
            cache_dir: default_cache_dir(),
            cache_enabled: false,
        }
    }
}

impl Config {
    /// Reads a JSON config file over the defaults. A missing file answers the defaults.
    pub fn load(path: &Path) -> Result<Config, Error> {
        match std::fs::read(path) {
            Ok(bytes) => serde_json::from_slice(&bytes)
                .map_err(|error| Error::usage(format!("{}: {error}", path.display()))),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(Config::default()),
            Err(error) => Err(Error::usage(format!("{}: {error}", path.display()))),
        }
    }

    /// Lets `HEY_BASE_URL`, `HEY_OAUTH_CLIENT_ID`, `HEY_CACHE_DIR` and `HEY_CACHE_ENABLED`
    /// override whatever is set. An empty variable counts as unset.
    pub fn with_env(mut self) -> Config {
        if let Some(value) = env_value("HEY_BASE_URL") {
            self.base_url = value;
        }
        if let Some(value) = env_value("HEY_OAUTH_CLIENT_ID") {
            self.oauth_client_id = value;
        }
        if let Some(value) = env_value("HEY_CACHE_DIR") {
            self.cache_dir = PathBuf::from(value);
        }
        if let Some(value) = env_value("HEY_CACHE_ENABLED") {
            self.cache_enabled = value.eq_ignore_ascii_case("true") || value == "1";
        }
        self
    }

    pub fn with_base_url(mut self, base_url: impl Into<String>) -> Config {
        self.base_url = base_url.into();
        self
    }

    pub fn with_cache_enabled(mut self, enabled: bool) -> Config {
        self.cache_enabled = enabled;
        self
    }

    /// The base URL without a trailing slash: the key credentials are stored under.
    pub fn origin(&self) -> &str {
        self.base_url.trim_end_matches('/')
    }
}

fn env_value(name: &str) -> Option<String> {
    env::var(name).ok().filter(|value| !value.is_empty())
}

/// Where the response cache goes when nothing says otherwise: `$XDG_CACHE_HOME/hey`,
/// falling back to `~/.cache/hey`.
pub fn default_cache_dir() -> PathBuf {
    xdg_dir("XDG_CACHE_HOME", ".cache").join("hey")
}

/// Where credentials and settings go when nothing says otherwise: `$XDG_CONFIG_HOME/hey`,
/// falling back to `~/.config/hey`. Shared with hey-cli.
pub fn default_config_dir() -> PathBuf {
    xdg_dir("XDG_CONFIG_HOME", ".config").join("hey")
}

fn xdg_dir(variable: &str, fallback: &str) -> PathBuf {
    match env_value(variable) {
        Some(value) => PathBuf::from(value),
        None => env::home_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join(fallback),
    }
}
