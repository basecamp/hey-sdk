//! The response cache: conditional reads, 304 revalidation, and the file and memory stores.

mod support;

use std::fs;
use std::sync::{Arc, Mutex};

use bytes::Bytes;
use hey_sdk::cache::{CachedResponse, FileCache, InMemoryCache, ResponseCache};
use hey_sdk::http::HeaderMap;
use hey_sdk::models::{CreateMessageRequestContent, MessagePayload};
use serde_json::json;
use wiremock::matchers::{header, method, path, query_param};
use wiremock::{Mock, MockServer, Request, Respond, ResponseTemplate};

use support::{builder, client};

/// The cache directory is the one hey-cli keeps its own state in, so clearing the cache
/// takes only what the cache put there.
#[test]
fn clearing_the_file_cache_leaves_the_rest_of_the_directory_alone() {
    let directory = std::env::temp_dir().join(format!("hey-sdk-cache-{}", rand::random::<u64>()));
    fs::create_dir_all(&directory).unwrap();
    fs::write(directory.join("credentials.json"), b"{}").unwrap();
    let cache = FileCache::new(&directory);
    cache.set(
        "abc",
        CachedResponse {
            etag: "\"v1\"".to_string(),
            body: Bytes::from_static(b"[]"),
            headers: HeaderMap::new(),
        },
    );

    cache.clear();

    assert_eq!(None, cache.get("abc"));
    assert!(!directory.join("responses").exists());
    assert!(!directory.join("etags.json").exists());
    assert!(directory.join("credentials.json").exists());

    fs::remove_dir_all(&directory).unwrap();
}

#[tokio::test]
async fn a_second_read_revalidates_and_is_answered_from_the_cache() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1\""))
        .respond_with(ResponseTemplate::new(304).insert_header("ETag", "\"v1\""))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    let first = client.boxes().list().await.unwrap();
    let second = client.boxes().list().await.unwrap();

    assert_eq!(first[0].name, "Imbox");
    assert_eq!(second.len(), 1);
    assert_eq!(second[0].id, 7);
    assert_eq!(second[0].name, "Imbox");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert!(!requests[0].headers.contains_key("if-none-match"));
    assert_eq!(requests[1].headers["if-none-match"], "\"v1\"");
}

#[tokio::test]
async fn a_new_etag_replaces_what_the_cache_holds() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v2\""))
        .respond_with(ResponseTemplate::new(304).insert_header("ETag", "\"v2\""))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1\""))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v2\"")
                .set_body_json(json!([
                    { "id": 7, "kind": "imbox", "name": "Imbox" },
                    { "id": 8, "kind": "feedbox", "name": "The Feed" }
                ])),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    assert_eq!(client.boxes().list().await.unwrap().len(), 1);
    assert_eq!(client.boxes().list().await.unwrap().len(), 2);
    let third = client.boxes().list().await.unwrap();

    assert_eq!(third.len(), 2);
    assert_eq!(third[1].name, "The Feed");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 3);
    assert_eq!(requests[1].headers["if-none-match"], "\"v1\"");
    assert_eq!(requests[2].headers["if-none-match"], "\"v2\"");
}

#[tokio::test]
async fn a_client_without_a_cache_never_revalidates() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let client = client(&server);
    client.boxes().list().await.unwrap();
    client.boxes().list().await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert!(
        requests
            .iter()
            .all(|request| !request.headers.contains_key("if-none-match"))
    );
}

#[tokio::test]
async fn a_write_is_never_cached() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/messages.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .set_body_json(json!({
                    "id": 2201, "topic_id": 880, "subject": "Quarterly planning", "delayed": false
                })),
        )
        .mount(&server)
        .await;

    let body = CreateMessageRequestContent {
        acting_sender_id: 314,
        message: MessagePayload {
            subject: "Quarterly planning".to_string(),
            content: "<div>Agenda.</div>".to_string(),
        },
        entry: None,
    };
    let client = builder(&server)
        .cache(InMemoryCache::new())
        .build()
        .unwrap();
    client.messages().create(&body).await.unwrap();
    client.messages().create(&body).await.unwrap();

    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 2);
    assert!(
        requests
            .iter()
            .all(|request| !request.headers.contains_key("if-none-match"))
    );
}

/// Answers `/boxes.json` the way `geared_pagination`'s 200 does — `Link` and `X-Total-Count`
/// beside the `ETag` — and a request conditional on that `ETag` with a 304 carrying the
/// `ETag` and whatever `not_modified` holds, which is nothing unless a test puts news in it.
struct PagedBoxes {
    not_modified: Arc<Mutex<Vec<(&'static str, &'static str)>>>,
}

impl Respond for PagedBoxes {
    fn respond(&self, request: &Request) -> ResponseTemplate {
        if request
            .headers
            .get("if-none-match")
            .is_some_and(|etag| etag == "\"v1\"")
        {
            let mut answer = ResponseTemplate::new(304).insert_header("ETag", "\"v1\"");
            for (name, value) in self.not_modified.lock().unwrap().iter() {
                answer = answer.insert_header(*name, *value);
            }
            return answer;
        }
        ResponseTemplate::new(200)
            .insert_header("ETag", "\"v1\"")
            .insert_header("Link", r#"</boxes.json?page=cursor-2>; rel="next""#)
            .insert_header("X-Total-Count", "75")
            .insert_header("Set-Cookie", "session=abc")
            .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }]))
    }
}

/// Mounts [`PagedBoxes`] with a second page behind its cursor, and hands back what its
/// 304s carry, for a test to put news in.
async fn paged_boxes(server: &MockServer) -> Arc<Mutex<Vec<(&'static str, &'static str)>>> {
    let not_modified = Arc::new(Mutex::new(Vec::new()));
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(query_param("page", "cursor-2"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(json!([{ "id": 8, "kind": "feedbox", "name": "The Feed" }])),
        )
        .mount(server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(PagedBoxes {
            not_modified: not_modified.clone(),
        })
        .mount(server)
        .await;
    not_modified
}

/// A 304 need not repeat the `Link` and `X-Total-Count` of the page it confirms, so the
/// cache answers it with the headers stored beside the body: the revalidated page still
/// counts the collection and still leads to the next page.
async fn a_bare_304_keeps_the_page_it_confirms(cache: impl ResponseCache + 'static) {
    let server = MockServer::start().await;
    let not_modified = paged_boxes(&server).await;
    let client = builder(&server).cache(cache).build().unwrap();

    let first = client.boxes().list().await.unwrap();
    let revalidated = client.boxes().list().await.unwrap();

    assert_eq!(first.total_count(), Some(75));
    assert_eq!(
        revalidated.total_count(),
        Some(75),
        "the count the 304 left out"
    );
    assert_eq!(
        revalidated.next_page(),
        Some("cursor-2"),
        "the Link the 304 left out"
    );
    let second = client.next_page(&revalidated).await.unwrap().unwrap();
    assert_eq!(second[0].name, "The Feed");

    // A header the 304 carries replaces the stored one, in the answer and in the entry.
    not_modified.lock().unwrap().push(("X-Total-Count", "76"));
    let updated = client.boxes().list().await.unwrap();
    not_modified.lock().unwrap().clear();
    let after = client.boxes().list().await.unwrap();

    assert_eq!(updated.total_count(), Some(76));
    assert_eq!(
        after.total_count(),
        Some(76),
        "the entry kept the 304's update"
    );
    assert_eq!(after.next_page(), Some("cursor-2"));
    let requests = server.received_requests().await.unwrap();
    let conditional = requests
        .iter()
        .filter(|request| request.headers.contains_key("if-none-match"))
        .count();
    assert_eq!(conditional, 3, "every read after the first revalidated");
}

#[tokio::test]
async fn a_bare_304_keeps_the_page_it_confirms_in_memory() {
    a_bare_304_keeps_the_page_it_confirms(InMemoryCache::new()).await;
}

#[tokio::test]
async fn a_bare_304_keeps_the_page_it_confirms_on_disk() {
    let directory = std::env::temp_dir().join(format!("hey-sdk-cache-{}", rand::random::<u64>()));
    a_bare_304_keeps_the_page_it_confirms(FileCache::new(&directory)).await;
    fs::remove_dir_all(&directory).unwrap();
}

/// The file cache keeps the headers in the layout the Go SDK reads, leaves credentials and
/// the connection's own fields out, and takes an entry an older SDK wrote without its
/// headers for a miss: it could answer a 304 with the body, but not with the `Link`.
#[test]
fn the_file_cache_keeps_headers_beside_the_body() {
    let directory = std::env::temp_dir().join(format!("hey-sdk-cache-{}", rand::random::<u64>()));
    let cache = FileCache::new(&directory);
    let mut headers = HeaderMap::new();
    headers.insert(
        "link",
        r#"</boxes.json?page=2>; rel="next""#.parse().unwrap(),
    );
    headers.insert("x-total-count", "2".parse().unwrap());
    headers.insert("set-cookie", "session=abc".parse().unwrap());
    headers.insert("connection", "keep-alive, x-hop".parse().unwrap());
    headers.insert("x-hop", "1".parse().unwrap());
    cache.set(
        "abc",
        CachedResponse {
            etag: "\"v1\"".to_string(),
            body: Bytes::from_static(b"[]"),
            headers,
        },
    );

    let held = cache.get("abc").unwrap();
    let mut names: Vec<_> = held
        .headers
        .keys()
        .map(hey_sdk::http::HeaderName::as_str)
        .collect();
    names.sort_unstable();
    assert_eq!(names, ["link", "x-total-count"]);

    // Go writes the names canonically; they read back all the same.
    fs::write(
        directory.join("responses").join("abc.headers"),
        br#"{"Link": ["</boxes.json?page=3>; rel=\"next\""], "X-Total-Count": ["3"]}"#,
    )
    .unwrap();
    assert_eq!(cache.get("abc").unwrap().headers["x-total-count"], "3");

    fs::remove_file(directory.join("responses").join("abc.headers")).unwrap();
    assert_eq!(None, cache.get("abc"));

    fs::remove_dir_all(&directory).unwrap();
}

/// What a 304 carries reaches the caller whole, and the entry keeps what it may: a new
/// validator is the one the next read goes out with, a fresh `Set-Cookie` is the caller's
/// but never the cache's, and the body on disk is left as it was rather than written again.
#[tokio::test]
async fn a_304_updates_the_entry_without_rewriting_its_body() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1\""))
        .respond_with(
            ResponseTemplate::new(304)
                .insert_header("ETag", "\"v1-gzip\"")
                .insert_header("Set-Cookie", "session=fresh"),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .and(header("if-none-match", "\"v1-gzip\""))
        .respond_with(ResponseTemplate::new(304).insert_header("ETag", "\"v1-gzip\""))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/boxes.json"))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("ETag", "\"v1\"")
                .insert_header("X-Total-Count", "75")
                .set_body_json(json!([{ "id": 7, "kind": "imbox", "name": "Imbox" }])),
        )
        .mount(&server)
        .await;

    let directory = std::env::temp_dir().join(format!("hey-sdk-cache-{}", rand::random::<u64>()));
    let client = builder(&server)
        .cache(FileCache::new(&directory))
        .build()
        .unwrap();
    client.get("/boxes.json").await.unwrap();
    let body_file = fs::read_dir(directory.join("responses"))
        .unwrap()
        .map(|entry| entry.unwrap().path())
        .find(|path| {
            path.extension()
                .is_some_and(|extension| extension == "body")
        })
        .unwrap();
    let before = fs::metadata(&body_file).unwrap();
    let headers_file = body_file.with_extension("headers");

    let response = client.get("/boxes.json").await.unwrap();
    assert!(response.from_cache);
    assert_eq!(response.headers["set-cookie"], "session=fresh");
    assert_eq!(response.headers["x-total-count"], "75");
    let stored = fs::read_to_string(&headers_file).unwrap().to_lowercase();
    assert!(!stored.contains("set-cookie"), "{stored}");
    assert!(stored.contains("v1-gzip"), "{stored}");
    // A rewrite lands as a new file renamed into place.
    #[cfg(unix)]
    {
        use std::os::unix::fs::MetadataExt;
        assert_eq!(before.ino(), fs::metadata(&body_file).unwrap().ino());
    }
    #[cfg(not(unix))]
    assert_eq!(
        before.modified().unwrap(),
        fs::metadata(&body_file).unwrap().modified().unwrap()
    );

    let again = client.get("/boxes.json").await.unwrap();
    assert_eq!(again.headers["x-total-count"], "75");
    let requests = server.received_requests().await.unwrap();
    assert_eq!(requests[2].headers["if-none-match"], "\"v1-gzip\"");
    fs::remove_dir_all(&directory).unwrap();
}

/// A 304 confirms the entry the request went out conditional on, so an entry another read
/// replaced meanwhile is newer than anything it says, and both stores leave it alone.
#[test]
fn a_revalidation_leaves_an_entry_replaced_meanwhile_alone() {
    let directory = std::env::temp_dir().join(format!("hey-sdk-cache-{}", rand::random::<u64>()));
    let stores: [Box<dyn ResponseCache>; 2] = [
        Box::new(InMemoryCache::new()),
        Box::new(FileCache::new(&directory)),
    ];
    for cache in stores {
        let mut newer = HeaderMap::new();
        newer.insert("x-total-count", "99".parse().unwrap());
        cache.set(
            "abc",
            CachedResponse {
                etag: "\"v2\"".to_string(),
                body: Bytes::from_static(b"[2]"),
                headers: newer.clone(),
            },
        );
        let mut stale = HeaderMap::new();
        stale.insert("x-total-count", "2".parse().unwrap());
        cache.revalidate("abc", "\"v1\"", "\"v1\"".to_string(), stale.clone());
        let held = cache.get("abc").unwrap();
        assert_eq!(held.etag, "\"v2\"");
        assert_eq!(held.headers, newer);

        cache.revalidate("abc", "\"v2\"", "\"v3\"".to_string(), stale.clone());
        let held = cache.get("abc").unwrap();
        assert_eq!(
            (held.etag.as_str(), &held.body[..]),
            ("\"v3\"", &b"[2]"[..])
        );
        assert_eq!(held.headers, stale);
    }
    fs::remove_dir_all(&directory).unwrap();
}
