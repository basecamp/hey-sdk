use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use bytes::Bytes;
use sha2::{Digest, Sha256};

/// A response the cache holds for a URL: the validator HEY sent with it, and the body it
/// validates.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CachedResponse {
    pub etag: String,
    pub body: Bytes,
}

/// Stores JSON responses by ETag so a repeated read can be answered from a 304.
pub trait ResponseCache: Send + Sync {
    fn get(&self, key: &str) -> Option<CachedResponse>;
    fn set(&self, key: &str, response: CachedResponse);
    fn invalidate(&self, key: &str);
    fn clear(&self);
}

/// Keeps cached responses for the life of the process.
#[derive(Debug, Default)]
pub struct InMemoryCache {
    entries: Mutex<HashMap<String, CachedResponse>>,
}

impl InMemoryCache {
    pub fn new() -> InMemoryCache {
        InMemoryCache::default()
    }
}

impl ResponseCache for InMemoryCache {
    fn get(&self, key: &str) -> Option<CachedResponse> {
        self.entries.lock().unwrap().get(key).cloned()
    }

    fn set(&self, key: &str, response: CachedResponse) {
        self.entries
            .lock()
            .unwrap()
            .insert(key.to_string(), response);
    }

    fn invalidate(&self, key: &str) {
        self.entries.lock().unwrap().remove(key);
    }

    fn clear(&self) {
        self.entries.lock().unwrap().clear();
    }
}

/// Keeps cached responses on disk, the same layout the Go SDK uses: `etags.json` maps
/// keys to validators and `responses/<key>.body` holds the bodies. Anything that goes
/// wrong on disk is treated as a miss.
#[derive(Debug)]
pub struct FileCache {
    directory: PathBuf,
    lock: Mutex<()>,
}

impl FileCache {
    pub fn new(directory: impl Into<PathBuf>) -> FileCache {
        FileCache {
            directory: directory.into(),
            lock: Mutex::new(()),
        }
    }

    fn etags(&self) -> HashMap<String, String> {
        fs::read(self.directory.join("etags.json"))
            .ok()
            .and_then(|bytes| serde_json::from_slice(&bytes).ok())
            .unwrap_or_default()
    }

    fn write_etags(&self, etags: &HashMap<String, String>) {
        if let Ok(json) = serde_json::to_vec_pretty(etags) {
            write_private(&self.directory.join("etags.json"), &json);
        }
    }

    fn body_path(&self, key: &str) -> PathBuf {
        self.directory.join("responses").join(format!("{key}.body"))
    }
}

impl ResponseCache for FileCache {
    fn get(&self, key: &str) -> Option<CachedResponse> {
        let _guard = self.lock.lock().unwrap();
        let etag = self.etags().get(key).cloned()?;
        let body = fs::read(self.body_path(key)).ok()?;
        Some(CachedResponse {
            etag,
            body: Bytes::from(body),
        })
    }

    fn set(&self, key: &str, response: CachedResponse) {
        let _guard = self.lock.lock().unwrap();
        write_private(&self.body_path(key), &response.body);
        let mut etags = self.etags();
        etags.insert(key.to_string(), response.etag);
        self.write_etags(&etags);
    }

    fn invalidate(&self, key: &str) {
        let _guard = self.lock.lock().unwrap();
        let _ = fs::remove_file(self.body_path(key));
        let mut etags = self.etags();
        etags.remove(key);
        self.write_etags(&etags);
    }

    /// Throws away what the cache put in the directory and nothing else. The directory is
    /// shared with hey-cli, which keeps its credentials and its own state alongside, so
    /// only `responses/` and `etags.json` go.
    fn clear(&self) {
        let _guard = self.lock.lock().unwrap();
        let _ = fs::remove_dir_all(self.directory.join("responses"));
        let _ = fs::remove_file(self.directory.join("etags.json"));
    }
}

fn write_private(path: &Path, bytes: &[u8]) {
    let Some(parent) = path.parent() else { return };
    if fs::create_dir_all(parent).is_err() {
        return;
    }
    let temporary = path.with_extension("tmp");
    if fs::write(&temporary, bytes).is_ok() {
        restrict_permissions(parent, &temporary);
        let _ = fs::rename(&temporary, path);
    }
}

#[cfg(unix)]
fn restrict_permissions(directory: &Path, file: &Path) {
    use std::os::unix::fs::PermissionsExt;
    let _ = fs::set_permissions(directory, fs::Permissions::from_mode(0o700));
    let _ = fs::set_permissions(file, fs::Permissions::from_mode(0o600));
}

#[cfg(not(unix))]
fn restrict_permissions(_directory: &Path, _file: &Path) {}

/// The key a URL is cached under. The credentials are folded in so one person's cached
/// reads are never answered to another.
pub fn cache_key(url: &str, credential: &str) -> String {
    let mut credential_hash = String::new();
    if !credential.is_empty() {
        credential_hash = hex(&Sha256::digest(credential.as_bytes())[..8]);
    }
    hex(&Sha256::digest(
        format!("{url}:{credential_hash}").as_bytes(),
    ))
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}
