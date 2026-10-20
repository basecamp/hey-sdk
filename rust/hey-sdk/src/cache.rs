use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, PoisonError};

use bytes::Bytes;
use sha2::{Digest, Sha256};

use crate::http::header::{CONNECTION, CONTENT_LENGTH};
use crate::http::{HeaderMap, HeaderName, HeaderValue};

/// A response the cache holds for a URL: the validator HEY sent with it, the body it
/// validates, and the headers that came with the body, so a 304 answers the page as it was
/// first read — `Link` and `X-Total-Count` included, which a 304 need not repeat.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CachedResponse {
    /// The `ETag` HEY sent with the body, sent back as `If-None-Match` on the next read.
    pub etag: String,
    /// The body as HEY answered it.
    pub body: Bytes,
    /// The headers HEY answered the body with, less any credential and the connection's own
    /// fields.
    pub headers: HeaderMap,
}

/// The fields a cache entry never keeps: credentials, which have no place on disk, and the
/// connection's own fields, which RFC 9111 §3.1 keeps out of a cache.
const UNSTORED_HEADERS: &[&str] = &[
    "authorization",
    "proxy-authorization",
    "cookie",
    "set-cookie",
    "x-csrf-token",
    "connection",
    "keep-alive",
    "proxy-connection",
    "te",
    "transfer-encoding",
    "upgrade",
];

/// What of a response's headers a cache entry keeps beside its body.
pub(crate) fn storable_headers(headers: &HeaderMap) -> HeaderMap {
    let mut stored = headers.clone();
    for listed in headers.get_all(CONNECTION) {
        for name in listed.to_str().unwrap_or_default().split(',') {
            stored.remove(name.trim());
        }
    }
    for name in UNSTORED_HEADERS {
        stored.remove(*name);
    }
    stored
}

/// What a 304 answers with: the headers stored with the body, each field the 304 carried
/// replacing the stored one of that name, as RFC 9111 §4.3.4 has a cache update its entry.
/// `Content-Length` describes the 304's own empty body, not the stored one, so it is not
/// taken (RFC 9111 §3.2). Everything else the 304 carried reaches the caller, a fresh
/// `Set-Cookie` included; what the entry keeps of it is [`storable_headers`]' to decide.
pub(crate) fn revalidated_headers(stored: &HeaderMap, not_modified: &HeaderMap) -> HeaderMap {
    let mut merged = stored.clone();
    for name in not_modified.keys() {
        if name == CONTENT_LENGTH {
            continue;
        }
        merged.remove(name);
        for value in not_modified.get_all(name) {
            merged.append(name.clone(), value.clone());
        }
    }
    merged
}

/// Stores JSON responses by `ETag` so a repeated read can be answered from a 304.
pub trait ResponseCache: Send + Sync {
    /// What is held under `key`, if anything.
    fn get(&self, key: &str) -> Option<CachedResponse>;
    /// Holds a response under `key`, replacing whatever was there.
    fn set(&self, key: &str, response: CachedResponse);
    /// Forgets what is held under `key`.
    fn invalidate(&self, key: &str);
    /// Forgets everything.
    fn clear(&self);
    /// Keeps what a 304 said about the entry under `key`: the validator it carried, which
    /// is `validated` unless the 304 named another, and the headers it updated. The body
    /// is the one already held, so a store need not write it again; and the update is
    /// made only while the entry's validator is still `validated`, since a body stored
    /// meanwhile is newer than anything the 304 confirmed. The default reads the entry and
    /// sets it again whole. Between the two, another read can store a newer entry, which the
    /// set then replaces with the older one. That entry is still consistent — body,
    /// validator and headers of one version — so the next read revalidates it and draws the
    /// newer one in full. A store that can check and update in one step, as both of the
    /// crate's do, overrides this.
    fn revalidate(&self, key: &str, validated: &str, etag: String, headers: HeaderMap) {
        if let Some(held) = self.get(key)
            && held.etag == validated
        {
            self.set(
                key,
                CachedResponse {
                    etag,
                    body: held.body,
                    headers,
                },
            );
        }
    }
}

/// Keeps cached responses for the life of the process.
#[derive(Debug, Default)]
pub struct InMemoryCache {
    entries: Mutex<HashMap<String, CachedResponse>>,
}

impl InMemoryCache {
    /// An empty cache.
    pub fn new() -> InMemoryCache {
        InMemoryCache::default()
    }
}

impl ResponseCache for InMemoryCache {
    fn get(&self, key: &str) -> Option<CachedResponse> {
        self.entries
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .get(key)
            .cloned()
    }

    fn set(&self, key: &str, response: CachedResponse) {
        self.entries
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .insert(key.to_string(), response);
    }

    fn invalidate(&self, key: &str) {
        self.entries
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .remove(key);
    }

    fn clear(&self) {
        self.entries
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clear();
    }

    fn revalidate(&self, key: &str, validated: &str, etag: String, headers: HeaderMap) {
        let mut entries = self.entries.lock().unwrap_or_else(PoisonError::into_inner);
        if let Some(held) = entries.get_mut(key)
            && held.etag == validated
        {
            held.etag = etag;
            held.headers = headers;
        }
    }
}

/// Keeps cached responses on disk, the same layout the Go SDK uses: `etags.json` maps
/// keys to validators, `responses/<key>.body` holds the bodies and `responses/<key>.headers`
/// the headers each came with, as JSON mapping a name to its values. Anything that goes
/// wrong on disk is treated as a miss, and so is an entry an older SDK wrote without its
/// headers: it could answer a 304 with the body but not with the `Link` it came with.
#[derive(Debug)]
pub struct FileCache {
    directory: PathBuf,
    lock: Mutex<()>,
}

impl FileCache {
    /// A cache in `directory`, which is created on the first write.
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

    fn headers_path(&self, key: &str) -> PathBuf {
        self.directory
            .join("responses")
            .join(format!("{key}.headers"))
    }

    fn headers(&self, key: &str) -> Option<HeaderMap> {
        let bytes = fs::read(self.headers_path(key)).ok()?;
        let stored: HashMap<String, Vec<String>> = serde_json::from_slice(&bytes).ok()?;
        let mut headers = HeaderMap::new();
        for (name, values) in stored {
            // A name or value the client could not send is one it never stored either.
            let Ok(name) = HeaderName::from_bytes(name.as_bytes()) else {
                continue;
            };
            for value in values {
                if let Ok(value) = HeaderValue::from_str(&value) {
                    headers.append(name.clone(), value);
                }
            }
        }
        Some(headers)
    }

    fn write_headers(&self, key: &str, headers: &HeaderMap) {
        let storable = storable_headers(headers);
        let mut stored: HashMap<&str, Vec<&str>> = HashMap::new();
        for (name, value) in &storable {
            // A value that is not text cannot be written down as JSON, and is not one a
            // page's pagination is read from.
            if let Ok(value) = value.to_str() {
                stored.entry(name.as_str()).or_default().push(value);
            }
        }
        if let Ok(json) = serde_json::to_vec_pretty(&stored) {
            write_private(&self.headers_path(key), &json);
        }
    }
}

impl ResponseCache for FileCache {
    fn get(&self, key: &str) -> Option<CachedResponse> {
        let _guard = self.lock.lock().unwrap_or_else(PoisonError::into_inner);
        let etag = self.etags().get(key).cloned()?;
        let body = fs::read(self.body_path(key)).ok()?;
        let headers = self.headers(key)?;
        Some(CachedResponse {
            etag,
            body: Bytes::from(body),
            headers,
        })
    }

    fn set(&self, key: &str, response: CachedResponse) {
        let _guard = self.lock.lock().unwrap_or_else(PoisonError::into_inner);
        write_private(&self.body_path(key), &response.body);
        self.write_headers(key, &response.headers);
        let mut etags = self.etags();
        etags.insert(key.to_string(), response.etag);
        self.write_etags(&etags);
    }

    fn invalidate(&self, key: &str) {
        let _guard = self.lock.lock().unwrap_or_else(PoisonError::into_inner);
        let _ = fs::remove_file(self.body_path(key));
        let _ = fs::remove_file(self.headers_path(key));
        let mut etags = self.etags();
        etags.remove(key);
        self.write_etags(&etags);
    }

    /// Writes the headers and, should the 304 carry a new one, the validator; the body on
    /// disk is the one the 304 confirmed and stays as it is.
    fn revalidate(&self, key: &str, validated: &str, etag: String, headers: HeaderMap) {
        let _guard = self.lock.lock().unwrap_or_else(PoisonError::into_inner);
        let mut etags = self.etags();
        if etags.get(key).is_none_or(|held| held != validated) {
            return;
        }
        self.write_headers(key, &headers);
        if etag != validated {
            etags.insert(key.to_string(), etag);
            self.write_etags(&etags);
        }
    }

    /// Throws away what the cache put in the directory and nothing else. The directory is
    /// shared with hey-cli, which keeps its credentials and its own state alongside, so
    /// only `responses/` and `etags.json` go.
    fn clear(&self) {
        let _guard = self.lock.lock().unwrap_or_else(PoisonError::into_inner);
        let _ = fs::remove_dir_all(self.directory.join("responses"));
        let _ = fs::remove_file(self.directory.join("etags.json"));
    }
}

fn write_private(path: &Path, bytes: &[u8]) {
    let Some(parent) = path.parent() else { return };
    if fs::create_dir_all(parent).is_err() {
        return;
    }
    let temporary = temporary_path(path);
    if fs::write(&temporary, bytes).is_ok() {
        restrict_permissions(parent, &temporary);
        let _ = fs::rename(&temporary, path);
    }
}

/// Where a file is written before it is renamed into place: its whole name with `.tmp`
/// added, as Go names it, so a body and the headers beside it — `<key>.body` and
/// `<key>.headers` — never share one, even when another process writes the same key.
fn temporary_path(path: &Path) -> PathBuf {
    let mut name = path.as_os_str().to_owned();
    name.push(".tmp");
    PathBuf::from(name)
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
    use std::fmt::Write;
    bytes.iter().fold(String::new(), |mut hex, byte| {
        let _ = write!(hex, "{byte:02x}");
        hex
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_body_and_its_headers_are_written_through_different_temporary_files() {
        let responses = Path::new("cache").join("responses");
        assert_eq!(
            temporary_path(&responses.join("abc.body")),
            responses.join("abc.body.tmp")
        );
        assert_ne!(
            temporary_path(&responses.join("abc.body")),
            temporary_path(&responses.join("abc.headers"))
        );
    }
}
