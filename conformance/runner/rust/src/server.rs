use std::io;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use axum::Router;
use axum::body::{Body, Bytes};
use axum::extract::{Request, State};
use axum::http::header::CONTENT_TYPE;
use axum::http::{HeaderMap, Response, StatusCode};
use axum::response::IntoResponse;
use tokio::net::TcpListener;
use tokio::task::JoinHandle;

use crate::fixtures::MockResponse;

const NO_MORE_RESPONSES: &str = r#"{"error": "No more mock responses"}"#;

/// What the mock server saw and answered, for the assertions to read afterwards.
#[derive(Debug, Default, Clone)]
pub struct Recorded {
    pub count: usize,
    pub times: Vec<Instant>,
    pub paths: Vec<String>,
    pub methods: Vec<String>,
    pub queries: Vec<Vec<(String, String)>>,
    pub bodies: Vec<Bytes>,
    pub headers: Vec<HeaderMap>,
    pub statuses: Vec<u16>,
    pub links: Vec<Option<String>>,
    served: usize,
}

/// A loopback server that answers each request with the case's next mock response, in
/// order. A request to `/` is refused and not recorded: the conformance server serves API
/// operation routes, and HEY has no root operation.
pub struct MockServer {
    base_url: String,
    recorded: Arc<Mutex<Recorded>>,
    server: JoinHandle<()>,
}

struct Mocks {
    responses: Vec<MockResponse>,
    recorded: Arc<Mutex<Recorded>>,
}

impl MockServer {
    pub async fn start(responses: Vec<MockResponse>) -> Result<MockServer, io::Error> {
        let recorded = Arc::new(Mutex::new(Recorded::default()));
        let mocks = Arc::new(Mocks {
            responses,
            recorded: recorded.clone(),
        });
        let listener = TcpListener::bind("127.0.0.1:0").await?;
        let base_url = format!("http://{}", listener.local_addr()?);
        let router = Router::new().fallback(answer).with_state(mocks);
        let server = tokio::spawn(async move {
            let _ = axum::serve(listener, router).await;
        });
        Ok(MockServer {
            base_url,
            recorded,
            server,
        })
    }

    pub fn base_url(&self) -> &str {
        &self.base_url
    }

    pub fn shutdown(self) -> Recorded {
        self.server.abort();
        let recorded = self.recorded.lock().unwrap();
        recorded.clone()
    }
}

async fn answer(State(mocks): State<Arc<Mocks>>, request: Request) -> Response<Body> {
    let (parts, body) = request.into_parts();
    if parts.uri.path() == "/" {
        return (StatusCode::NOT_FOUND, "404 page not found").into_response();
    }

    let bytes = axum::body::to_bytes(body, usize::MAX)
        .await
        .unwrap_or_default();
    let index = record_request(&mocks.recorded, &parts, bytes);

    match mocks.responses.get(index) {
        None => (StatusCode::INTERNAL_SERVER_ERROR, NO_MORE_RESPONSES).into_response(),
        Some(mock) => {
            if mock.delay > 0 {
                tokio::time::sleep(Duration::from_millis(mock.delay)).await;
            }
            record_response(&mocks.recorded, mock);
            serve(mock)
        }
    }
}

fn record_request(
    recorded: &Mutex<Recorded>,
    parts: &axum::http::request::Parts,
    body: Bytes,
) -> usize {
    let mut recorded = recorded.lock().unwrap();
    recorded.count += 1;
    recorded.times.push(Instant::now());
    recorded.paths.push(parts.uri.path().to_string());
    recorded.methods.push(parts.method.to_string());
    recorded
        .queries
        .push(query_pairs(parts.uri.query().unwrap_or_default()));
    recorded.bodies.push(body);
    recorded.headers.push(parts.headers.clone());
    let index = recorded.served;
    recorded.served += 1;
    index
}

fn query_pairs(query: &str) -> Vec<(String, String)> {
    url::form_urlencoded::parse(query.as_bytes())
        .map(|(name, value)| (name.into_owned(), value.into_owned()))
        .collect()
}

fn record_response(recorded: &Mutex<Recorded>, mock: &MockResponse) {
    let mut recorded = recorded.lock().unwrap();
    recorded.statuses.push(mock.status);
    recorded.links.push(header(mock, "link"));
}

fn header(mock: &MockResponse, name: &str) -> Option<String> {
    mock.headers
        .iter()
        .find(|(header, _)| header.eq_ignore_ascii_case(name))
        .map(|(_, value)| value.clone())
}

fn serve(mock: &MockResponse) -> Response<Body> {
    let mut response = Response::builder().status(mock.status);
    for (name, value) in &mock.headers {
        response = response.header(name, value);
    }
    let typed = response
        .headers_ref()
        .is_some_and(|headers| headers.contains_key(CONTENT_TYPE));
    if !typed {
        response = response.header(CONTENT_TYPE, "application/json");
    }
    let body = match &mock.body {
        Some(body) => Body::from(serde_json::to_vec(body).unwrap_or_default()),
        None => Body::empty(),
    };
    response.body(body).expect("mock response is well formed")
}
