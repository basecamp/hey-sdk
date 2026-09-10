use std::fmt::Display;
use std::sync::Arc;
use std::time::{Duration, Instant};

use bytes::Bytes;
use serde::de::DeserializeOwned;
use tokio::sync::Mutex;
use url::Url;

use crate::auth::{AuthStrategy, BearerAuth, TokenProvider};
use crate::cache::{CachedResponse, FileCache, ResponseCache, cache_key};
use crate::config::Config;
use crate::error::{Error, ErrorCode, retry_after_seconds};
use crate::http::header::{
    ACCEPT, AUTHORIZATION, CONTENT_LENGTH, CONTENT_TYPE, COOKIE, IF_NONE_MATCH,
    PROXY_AUTHORIZATION, USER_AGENT,
};
use crate::http::{
    Body, HeaderMap, HeaderValue, HttpClient, Method, Request, Response as HttpResponse, StatusCode,
};
use crate::observability::{
    Hooks, NoopHooks, OperationInfo, OperationState, RequestInfo, RequestResult,
};
use crate::operation::Operation;
use crate::pagination::Page;
use crate::route::Route;
use crate::security::{is_same_origin, require_secure_endpoint};
use crate::services::boxes::BoxKinds;
use crate::trace::{AttemptSpan, OperationSpan, label};
use crate::version::default_user_agent;

pub const DEFAULT_TIMEOUT: Duration = Duration::from_secs(30);
pub const DEFAULT_MAX_RETRIES: u32 = 3;
pub const DEFAULT_BASE_DELAY: Duration = Duration::from_secs(1);
/// The longest the client waits between attempts, however many it has made.
///
/// This is a deliberate divergence from Go, whose backoff doubles without a ceiling: by the
/// fourth attempt there it is already eight seconds, and a caller who raised
/// [`ClientBuilder::max_retries`] would be waiting minutes on a scope that is never coming
/// back. The circuit breaker is what should give up on that scope; the backoff's job is to
/// stop hammering, and thirty seconds does it. Move it with [`ClientBuilder::max_delay`].
pub const DEFAULT_MAX_DELAY: Duration = Duration::from_secs(30);
pub const DEFAULT_MAX_JITTER: Duration = Duration::from_millis(100);
pub const DEFAULT_MAX_PAGES: usize = 10_000;
pub const DEFAULT_MAX_RESPONSE_BODY_BYTES: usize = 16 << 20;

/// The most the client buffers of an answer the configurable cap leaves alone: a blob, an
/// export, whatever a form request answered. Only [`Client::download_blob`], which writes
/// to the caller's destination as the bytes arrive, reads without a bound.
pub const MAX_RESPONSE_BODY_BYTES: usize = 50 << 20;

/// The statuses a request the model says nothing about — a path the caller wrote — is
/// resent on. A modelled route is resent on the statuses its own policy names.
const RETRYABLE_STATUSES: &[u16] = &[429, 500, 502, 503, 504];
const ACCOUNT_FILTER_PARAMETER: &str = "filtered_account_id";
/// How many redirects one request may go through before the client gives up on it, which
/// is what reqwest allowed when it was the one following them.
const MAX_REDIRECTS: usize = 10;

/// A HEY client: one authenticated identity, presenting mail from All Accounts unless
/// derived for one linked account with [`Client::for_account`].
///
/// Clients are cheap to clone and share their connection pool, credentials and cache.
#[derive(Clone)]
pub struct Client {
    pub(crate) shared: Arc<Shared>,
    pub(crate) account_id: Option<i64>,
    pub(crate) scope: Arc<ScopeState>,
}

pub(crate) struct Shared {
    pub(crate) config: Config,
    pub(crate) base_url: Url,
    pub(crate) http: Arc<dyn HttpClient>,
    pub(crate) auth: Arc<dyn AuthStrategy>,
    pub(crate) user_agent: String,
    pub(crate) max_retries: u32,
    pub(crate) base_delay: Option<Duration>,
    pub(crate) max_delay: Duration,
    pub(crate) max_jitter: Duration,
    pub(crate) max_pages: usize,
    pub(crate) max_response_body_bytes: usize,
    pub(crate) cache: Option<Arc<dyn ResponseCache>>,
    pub(crate) hooks: Arc<dyn Hooks>,
}

/// What a client works out about the identity it presents and keeps for as long as it
/// lives. A client derived with [`Client::for_account`] starts an empty one of its own,
/// since none of it means the same thing under another account.
#[derive(Default)]
pub(crate) struct ScopeState {
    pub(crate) default_sender_id: Mutex<Option<i64>>,
    pub(crate) account_user_id: Mutex<Option<i64>>,
    pub(crate) box_kinds: Mutex<Option<BoxKinds>>,
}

/// What came back from HEY, before it is decoded.
#[derive(Debug, Clone)]
pub struct Response {
    pub status: StatusCode,
    pub headers: HeaderMap,
    pub body: Bytes,
    pub url: Url,
    pub from_cache: bool,
    /// The operation takes this status for an answer rather than a failure: a 404 that
    /// means "nothing there", or the redirect a form request went out to collect.
    pub empty: bool,
}

impl Response {
    pub fn json<T: DeserializeOwned>(&self) -> Result<T, Error> {
        if self.body.is_empty() {
            Err(Error::api(self.status.as_u16(), "empty response body"))
        } else {
            Ok(serde_json::from_slice(&self.body)?)
        }
    }

    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers.get(name).and_then(|value| value.to_str().ok())
    }
}

pub struct ClientBuilder {
    config: Config,
    auth: Option<Arc<dyn AuthStrategy>>,
    http: Option<Arc<dyn HttpClient>>,
    user_agent: String,
    timeout: Duration,
    max_retries: u32,
    base_delay: Option<Duration>,
    max_delay: Duration,
    max_jitter: Duration,
    max_pages: usize,
    max_response_body_bytes: usize,
    cache: Option<Arc<dyn ResponseCache>>,
    pub(crate) hooks: Arc<dyn Hooks>,
}

impl ClientBuilder {
    pub fn new(config: Config) -> ClientBuilder {
        ClientBuilder {
            config,
            auth: None,
            http: None,
            user_agent: default_user_agent(),
            timeout: DEFAULT_TIMEOUT,
            max_retries: DEFAULT_MAX_RETRIES,
            base_delay: None,
            max_delay: DEFAULT_MAX_DELAY,
            max_jitter: DEFAULT_MAX_JITTER,
            max_pages: DEFAULT_MAX_PAGES,
            max_response_body_bytes: DEFAULT_MAX_RESPONSE_BODY_BYTES,
            cache: None,
            hooks: Arc::new(NoopHooks),
        }
    }

    pub fn token_provider(self, provider: impl TokenProvider + 'static) -> ClientBuilder {
        self.auth_strategy(BearerAuth::new(provider))
    }

    pub fn auth_strategy(mut self, strategy: impl AuthStrategy + 'static) -> ClientBuilder {
        self.auth = Some(Arc::new(strategy));
        self
    }

    /// Replaces the HTTP client every request goes out on, including the attachment bytes
    /// that go to the storage service. The one supplied must not follow redirects; see
    /// [`HttpClient`]. The timeout set on the builder is then ignored — a timeout belongs to
    /// the client that can enforce it.
    pub fn http_client(mut self, http: impl HttpClient + 'static) -> ClientBuilder {
        self.http = Some(Arc::new(http));
        self
    }

    pub fn user_agent(mut self, user_agent: impl Into<String>) -> ClientBuilder {
        self.user_agent = user_agent.into();
        self
    }

    /// How long the HTTP client the SDK ships gives an answer to arrive. It has no effect on
    /// one supplied with [`ClientBuilder::http_client`].
    pub fn timeout(mut self, timeout: Duration) -> ClientBuilder {
        self.timeout = timeout;
        self
    }

    /// The most times any operation is resent after a transient failure. A modelled
    /// route is resent as many times as its own policy allows and no more; this only
    /// lowers that. A path the caller wrote, which no policy covers, is resent this many
    /// times when its method is idempotent.
    pub fn max_retries(mut self, max_retries: u32) -> ClientBuilder {
        self.max_retries = max_retries;
        self
    }

    /// The least the client waits before the first resend. A modelled route starts from
    /// the delay its own policy names when that is longer; a path the caller wrote starts
    /// from this, or from [`DEFAULT_BASE_DELAY`] when it is not set. Each wait after the
    /// first is double the one before. [`ClientBuilder::max_delay`] holds every wait down,
    /// this one included.
    pub fn base_delay(mut self, base_delay: Duration) -> ClientBuilder {
        self.base_delay = Some(base_delay);
        self
    }

    /// The most the client waits between attempts, jitter included, whatever the policy,
    /// the backoff or [`ClientBuilder::base_delay`] asks for. The wait a `Retry-After`
    /// names is honoured as given.
    pub fn max_delay(mut self, max_delay: Duration) -> ClientBuilder {
        self.max_delay = max_delay;
        self
    }

    pub fn max_jitter(mut self, max_jitter: Duration) -> ClientBuilder {
        self.max_jitter = max_jitter;
        self
    }

    pub fn max_pages(mut self, max_pages: usize) -> ClientBuilder {
        self.max_pages = max_pages;
        self
    }

    /// The most a JSON or HTML answer may deliver before the client refuses to hold it.
    /// Zero asks for the default: the cap cannot be lifted, only moved.
    pub fn max_response_body_bytes(mut self, bytes: usize) -> ClientBuilder {
        self.max_response_body_bytes = bytes;
        self
    }

    /// Caches JSON reads by ETag. Without this, `config.cache_enabled` decides whether a
    /// [`FileCache`] in `config.cache_dir` is used.
    pub fn cache(mut self, cache: impl ResponseCache + 'static) -> ClientBuilder {
        self.cache = Some(Arc::new(cache));
        self
    }

    /// Reports every operation and every request the client makes. Several sets of hooks
    /// go on as one with [`crate::observability::ChainHooks`].
    pub fn hooks(mut self, hooks: impl Hooks + 'static) -> ClientBuilder {
        self.hooks = Arc::new(hooks);
        self
    }

    pub fn build(self) -> Result<Client, Error> {
        let base_url = parse_base_url(&self.config.base_url)?;
        let auth = self
            .auth
            .ok_or_else(|| Error::usage("a token provider or auth strategy is required"))?;
        if self.timeout.is_zero() {
            return Err(Error::usage("timeout must be greater than zero"));
        }
        if self.max_pages == 0 {
            return Err(Error::usage("max pages must be greater than zero"));
        }
        let http = match self.http {
            Some(http) => http,
            None => shipped_http_client(self.timeout)?,
        };
        let cache =
            match (self.cache, self.config.cache_enabled) {
                (Some(cache), _) => Some(cache),
                (None, true) => Some(Arc::new(FileCache::new(self.config.cache_dir.clone()))
                    as Arc<dyn ResponseCache>),
                (None, false) => None,
            };
        let max_response_body_bytes = match self.max_response_body_bytes {
            0 => DEFAULT_MAX_RESPONSE_BODY_BYTES,
            bytes => bytes,
        };
        let shared = Shared {
            config: self.config,
            base_url,
            http,
            auth,
            user_agent: self.user_agent,
            max_retries: self.max_retries,
            base_delay: self.base_delay,
            max_delay: self.max_delay,
            max_jitter: self.max_jitter,
            max_pages: self.max_pages,
            max_response_body_bytes,
            cache,
            hooks: self.hooks,
        };
        Ok(Client {
            shared: Arc::new(shared),
            account_id: None,
            scope: Arc::default(),
        })
    }
}

impl Client {
    pub fn builder(config: Config) -> ClientBuilder {
        ClientBuilder::new(config)
    }

    /// A client with the default settings and a bearer token, on the HTTP client the SDK
    /// ships. Without the `reqwest` feature there is no such client, and a
    /// [`ClientBuilder`] with an [`HttpClient`] of the application's own is the way in.
    #[cfg(feature = "reqwest")]
    #[cfg_attr(docsrs, doc(cfg(feature = "reqwest")))]
    pub fn new(config: Config, provider: impl TokenProvider + 'static) -> Result<Client, Error> {
        Client::builder(config).token_provider(provider).build()
    }

    pub fn config(&self) -> &Config {
        &self.shared.config
    }

    pub fn base_url(&self) -> &Url {
        &self.shared.base_url
    }

    /// The linked account this client presents, or `None` for All Accounts.
    pub fn account_id(&self) -> Option<i64> {
        self.account_id
    }

    pub fn max_pages(&self) -> usize {
        self.shared.max_pages
    }

    /// The HTTP client every request goes out on, for the one request the SDK makes outside
    /// HEY: the attachment blob that goes to the storage service the direct upload named. It
    /// shares the connection pool and the settings the caller configured, and carries no
    /// credentials of its own — those go on per request.
    pub(crate) fn http(&self) -> &dyn HttpClient {
        self.shared.http.as_ref()
    }

    /// Starts a request for one of the modelled routes. Generated service methods call
    /// this; reach for it directly only to add headers or query parameters they do not
    /// expose.
    pub fn operation(&self, route: &'static Route, params: &[&dyn Display]) -> Operation {
        Operation::for_route(route, params)
    }

    /// Starts a request for a path the model does not cover. The path is relative to the
    /// base URL and gets the same credentials, `.json` suffix, account scope and retry
    /// treatment as a modelled one.
    pub fn request(&self, method: Method, path: impl Into<String>) -> Operation {
        Operation::raw(method, path.into())
    }

    /// Sends an operation and decodes its JSON body.
    pub async fn send<T: DeserializeOwned>(&self, operation: Operation) -> Result<T, Error> {
        self.execute(operation).await?.json()
    }

    /// Sends an operation whose answer carries no body worth reading.
    pub async fn send_unit(&self, operation: Operation) -> Result<(), Error> {
        self.execute(operation).await.map(|_| ())
    }

    /// Sends an operation and reads its body as text: the HTML page a route serves no
    /// JSON for.
    pub async fn send_text(&self, operation: Operation) -> Result<String, Error> {
        let response = self.execute(operation).await?;
        Ok(String::from_utf8_lossy(&response.body).into_owned())
    }

    /// Sends an operation that answers a status meaning "nothing there" with `None`.
    pub async fn send_optional<T: DeserializeOwned>(
        &self,
        operation: Operation,
    ) -> Result<Option<T>, Error> {
        let response = self.execute(operation).await?;
        if response.empty {
            Ok(None)
        } else {
            response.json().map(Some)
        }
    }

    /// Sends a paginated read and keeps the cursor HEY answered with.
    pub async fn send_page<T: DeserializeOwned>(
        &self,
        operation: Operation,
    ) -> Result<Page<T>, Error> {
        let info = operation.info.clone();
        let route = operation.route;
        let response = self.execute(operation).await?;
        Ok(Page::new(response.json()?, &response, info, route))
    }

    /// Reads the page after the given one, or `None` when HEY named no next page. A
    /// `Link` header pointing off the HEY origin is refused rather than followed. The read
    /// announces itself as the operation the first page came from, so a whole walk shows
    /// up as one thing rather than as a list read followed by anonymous requests, and it
    /// is resent under that operation's retry policy.
    pub async fn next_page<T: DeserializeOwned>(
        &self,
        page: &Page<T>,
    ) -> Result<Option<Page<T>>, Error> {
        match page.next_url() {
            None => Ok(None),
            Some(next) if !is_same_origin(next, &self.shared.base_url) => Err(Error::usage(
                format!("pagination Link header points to a different origin: {next}"),
            )),
            Some(next) => {
                let mut operation = Operation::at(Method::GET, next.clone());
                operation.info(page.info().clone());
                operation.route = page.route();
                self.send_page(operation).await.map(Some)
            }
        }
    }

    /// Reads every page after the first, up to the client's page limit, calling `visit`
    /// with each one. Stops early when `visit` answers `false`.
    pub async fn each_page<T: DeserializeOwned>(
        &self,
        first: Page<T>,
        mut visit: impl FnMut(&Page<T>) -> bool,
    ) -> Result<(), Error> {
        let mut page = first;
        let mut count = 1;
        while visit(&page) && count < self.shared.max_pages {
            match self.next_page(&page).await? {
                Some(next) => page = next,
                None => break,
            }
            count += 1;
        }
        Ok(())
    }

    /// Sends an operation: asks the hooks whether it may run, applies credentials and
    /// account scope, retries transient failures when the operation is idempotent,
    /// resends once after a refreshed 401, and answers a cached body on 304. Non-2xx
    /// statuses become errors unless the operation treats them as empty.
    pub async fn execute(&self, operation: Operation) -> Result<Response, Error> {
        let span = span_for(&operation);
        span.wrap(self.instrument(&operation, self.dispatch(&operation, &span)))
            .await
    }

    /// Sends an operation and hands back the answer with its body unread, for a caller
    /// that writes it somewhere rather than holding it. Everything up to the answer is
    /// [`Client::execute`]'s doing — the gate, the credentials, the account scope, the
    /// retries, the resend after a refreshed 401 — and nothing is resent once the answer
    /// is in hand, since its bytes may already be on their way out.
    pub(crate) async fn stream(&self, operation: Operation) -> Result<HttpResponse<Body>, Error> {
        let span = span_for(&operation);
        span.wrap(self.instrument(&operation, self.streamed(&operation, &span)))
            .await
    }

    /// Runs one operation inside the hook lifecycle every call shares. A quiet operation is
    /// one request inside another and skips that lifecycle — see [`Operation::quiet`].
    /// The `tracing` span around all of this is the caller's to put on, so that the gate and
    /// the end hook are inside it too.
    ///
    /// The end is reported from a drop guard rather than after the await, because the await
    /// may never return: a caller's `tokio::time::timeout` or `select!` can drop the future
    /// mid-flight, and a start with no end leaves the bulkhead a permit short and the
    /// circuit breaker a call short for the life of the client. Dropped that way, the
    /// operation ends as [`Error::cancelled`].
    async fn instrument<T>(
        &self,
        operation: &Operation,
        work: impl Future<Output = Result<T, Error>>,
    ) -> Result<T, Error> {
        if operation.quiet {
            work.await
        } else {
            let hooks = &self.shared.hooks;
            hooks.on_operation_gate(&operation.info).await?;

            let mut running = Running {
                hooks,
                info: &operation.info,
                state: Some(hooks.on_operation_start(&operation.info)),
                started: Instant::now(),
            };
            let outcome = work.await;
            running.finished(outcome.as_ref().map(|_| ()));
            outcome
        }
    }

    /// Reads the answer the retry loop settled on, and tells the hooks how it turned out
    /// once its body has been dealt with.
    async fn dispatch(
        &self,
        operation: &Operation,
        span: &OperationSpan,
    ) -> Result<Response, Error> {
        let url = self.url_for(operation)?;
        let answered = self.attempt(operation, &url).await?;
        let status = answered.response.status();
        span.answered(status, request_id(answered.response.headers()));
        let finished = self
            .finish(
                operation,
                &url,
                answered.url,
                answered.response,
                answered.cached,
            )
            .await;
        self.shared.hooks.on_request_end(
            &answered.info,
            &RequestResult {
                status: Some(status),
                duration: answered.duration,
                error: finished.as_ref().err(),
                from_cache: finished.as_ref().is_ok_and(|response| response.from_cache),
                retryable: answered.retryable,
                retry_after: answered.retry_after,
            },
        );
        finished
    }

    /// Hands the answer over unread, once its status says there is a body worth reading.
    async fn streamed(
        &self,
        operation: &Operation,
        span: &OperationSpan,
    ) -> Result<HttpResponse<Body>, Error> {
        let url = self.url_for(operation)?;
        let answered = self.attempt(operation, &url).await?;
        let status = answered.response.status();
        span.answered(status, request_id(answered.response.headers()));
        let failure = (!status.is_success()).then(|| {
            Error::from_response(status, &operation.method, answered.response.headers(), &[])
        });
        self.shared.hooks.on_request_end(
            &answered.info,
            &RequestResult {
                status: Some(status),
                duration: answered.duration,
                error: failure.as_ref(),
                from_cache: false,
                retryable: answered.retryable,
                retry_after: answered.retry_after,
            },
        );
        match failure {
            Some(error) => Err(error),
            None => Ok(answered.response),
        }
    }

    /// What the retry loop may spend on one operation. A modelled route brings its own
    /// policy from the model — how many sends it gets in all, which statuses earn another,
    /// and how long the first wait is — and the client's settings only make that gentler:
    /// [`ClientBuilder::max_retries`] caps the sends, [`ClientBuilder::base_delay`] holds
    /// the wait up and [`ClientBuilder::max_delay`] holds it down. A route the model gives
    /// no policy is sent once. A path the caller wrote has no policy to bring, so it runs
    /// on the client's settings alone. Whatever the policy, an operation that is not
    /// idempotent is sent once.
    fn budget(&self, operation: &Operation) -> Budget {
        let shared = &self.shared;
        let ceiling = shared.max_retries.saturating_add(1);
        let (attempts, retry_on, delay) = match operation.route.map(|route| &route.retry) {
            Some(policy) if policy.max > 0 => (
                policy.max.min(ceiling),
                policy.retry_on,
                Duration::from_millis(policy.base_delay_ms)
                    .max(shared.base_delay.unwrap_or(Duration::ZERO)),
            ),
            Some(_) => (1, &[][..], DEFAULT_BASE_DELAY),
            None => (
                ceiling,
                RETRYABLE_STATUSES,
                shared.base_delay.unwrap_or(DEFAULT_BASE_DELAY),
            ),
        };
        Budget {
            attempts: if operation.idempotent { attempts } else { 1 },
            retry_on,
            delay: delay.min(shared.max_delay),
        }
    }

    /// Sends the operation as many times as its retry budget and HEY's answers call for,
    /// and hands back the answer it stopped on with the body still unread.
    async fn attempt(&self, operation: &Operation, url: &Url) -> Result<Answered, Error> {
        let hooks = &self.shared.hooks;
        let budget = self.budget(operation);
        let mut attempts = budget.attempts;
        let mut attempt = 1;
        let mut delay = budget.delay;
        let mut refreshed = false;
        // Looked up once and carried across the attempts: a resend would find the same
        // entry, and the cache the SDK ships reads it off disk.
        let mut cached = None;

        loop {
            let request = self.prepare(operation, url, &mut cached).await?;
            let info = RequestInfo {
                method: operation.method.clone(),
                url: url.clone(),
                attempt,
            };
            hooks.on_request_start(&info);
            let started = Instant::now();
            // The attempt span closes here, before any refresh or backoff: it is the send.
            let sent = {
                let span = AttemptSpan::new(attempt);
                let sent = span
                    .wrap(self.transmit(operation, url.clone(), request))
                    .await;
                if let Ok((_, response)) = &sent {
                    span.answered(response.status());
                }
                sent
            };
            let duration = started.elapsed();

            match sent {
                Err(error) => {
                    hooks.on_request_end(
                        &info,
                        &RequestResult {
                            status: None,
                            duration,
                            error: Some(&error),
                            from_cache: false,
                            retryable: true,
                            retry_after: None,
                        },
                    );
                    if attempt < attempts {
                        crate::trace::debug!(operation = label(operation), attempt, error = %error.code(), "request failed, retrying");
                        hooks.on_retry(&info, attempt + 1, &error);
                        self.wait(delay).await;
                        delay = self.next_delay(delay);
                        attempt += 1;
                    } else {
                        return Err(error);
                    }
                }
                Ok((final_url, response)) => {
                    let status = response.status();
                    let retryable = budget.retry_on.contains(&status.as_u16());
                    let retry_after = retry_after_asked(status, response.headers());
                    if status == StatusCode::UNAUTHORIZED
                        && !refreshed
                        && self.shared.auth.refresh().await
                    {
                        let cause = Error::auth("Token refreshed").retryable();
                        hooks.on_request_end(
                            &info,
                            &RequestResult {
                                status: Some(status),
                                duration,
                                error: Some(&cause),
                                from_cache: false,
                                retryable,
                                retry_after,
                            },
                        );
                        crate::trace::debug!(
                            operation = label(operation),
                            "credentials refreshed, resending"
                        );
                        hooks.on_retry(&info, attempt + 1, &cause);
                        refreshed = true;
                        attempt += 1;
                        attempts = attempts.max(attempt);
                    } else if retryable && attempt < attempts {
                        let cause = Error::from_response(
                            status,
                            &operation.method,
                            response.headers(),
                            &[],
                        );
                        hooks.on_request_end(
                            &info,
                            &RequestResult {
                                status: Some(status),
                                duration,
                                error: Some(&cause),
                                from_cache: false,
                                retryable,
                                retry_after,
                            },
                        );
                        crate::trace::debug!(operation = label(operation), attempt, %status, "retryable status, retrying");
                        hooks.on_retry(&info, attempt + 1, &cause);
                        match retry_after {
                            Some(seconds)
                                if status == StatusCode::TOO_MANY_REQUESTS && seconds > 0 =>
                            {
                                self.wait_as_asked(Duration::from_secs(seconds)).await;
                            }
                            _ => self.wait(delay).await,
                        }
                        delay = self.next_delay(delay);
                        attempt += 1;
                    } else {
                        return Ok(Answered {
                            url: final_url,
                            response,
                            cached: cached.take(),
                            info,
                            duration,
                            retryable,
                            retry_after,
                        });
                    }
                }
            }
        }
    }

    pub(crate) fn url_for(&self, operation: &Operation) -> Result<Url, Error> {
        let mut url = match &operation.url {
            Some(url) => url.clone(),
            None => {
                let mut path = operation.path.clone();
                if operation.json_suffix {
                    path = with_json_extension(&path);
                }
                self.shared.base_url.join(path.trim_start_matches('/'))?
            }
        };
        if !operation.query.is_empty() {
            url.query_pairs_mut().extend_pairs(&operation.query);
        }
        if let Some(account_id) = self.account_id
            && is_same_origin(&url, &self.shared.base_url)
        {
            let others: Vec<(String, String)> = url
                .query_pairs()
                .filter(|(name, _)| name != ACCOUNT_FILTER_PARAMETER)
                .map(|(name, value)| (name.into_owned(), value.into_owned()))
                .collect();
            url.query_pairs_mut()
                .clear()
                .extend_pairs(others)
                .append_pair(ACCOUNT_FILTER_PARAMETER, &account_id.to_string());
        }
        Ok(url)
    }

    /// Builds the request for one attempt, and looks the response cache up the first time
    /// it is asked for a key. `cached` carries the entry — or the empty stand-in that says
    /// "cacheable, nothing stored" — from one attempt to the next.
    async fn prepare(
        &self,
        operation: &Operation,
        url: &Url,
        cached: &mut Option<(String, CachedResponse)>,
    ) -> Result<Request<Bytes>, Error> {
        let mut request = Request::builder()
            .method(operation.method.clone())
            .uri(url.as_str())
            .body(Bytes::new())
            .map_err(Error::from_std)?;
        let headers = request.headers_mut();
        headers.insert(USER_AGENT, header_value(&self.shared.user_agent)?);
        headers.insert(ACCEPT, HeaderValue::from_static(operation.accept));
        if let Some(body) = &operation.body {
            headers.insert(CONTENT_TYPE, header_value(&body.content_type)?);
            *request.body_mut() = body.bytes.clone();
        }
        self.shared.auth.authenticate(&mut request).await?;

        let key = match self.cacheable(operation) {
            None => None,
            Some(cache) => match request
                .headers()
                .get(AUTHORIZATION)
                .and_then(|value| value.to_str().ok())
            {
                None => None,
                Some(credential) => {
                    let key = cache_key(url.as_str(), credential);
                    if cached.as_ref().is_none_or(|(held, _)| *held != key) {
                        *cached = self.look_up(cache, &key).await;
                    }
                    Some(key)
                }
            },
        };
        // A refreshed credential gives the read a new key, and whatever was held under the
        // old one belongs to somebody else's reading.
        if key.is_none() {
            *cached = None;
        }
        if let Some((_, entry)) = cached.as_ref()
            && !entry.etag.is_empty()
        {
            let validator = header_value(&entry.etag)?;
            request.headers_mut().insert(IF_NONE_MATCH, validator);
        }
        Ok(request)
    }

    /// What the cache holds for a key, as the attempt should carry it: the stored entry, an
    /// empty stand-in when there is nothing stored, and nothing at all when what is stored
    /// is longer than the client would hold — which is thrown away on the way past.
    async fn look_up(
        &self,
        cache: &Arc<dyn ResponseCache>,
        key: &str,
    ) -> Option<(String, CachedResponse)> {
        match cache_get(cache, key).await {
            Some(entry) if entry.body.len() <= self.shared.max_response_body_bytes => {
                Some((key.to_string(), entry))
            }
            Some(_) => {
                cache_invalidate(cache, key).await;
                None
            }
            None => Some((
                key.to_string(),
                CachedResponse {
                    etag: String::new(),
                    body: Bytes::new(),
                },
            )),
        }
    }

    /// The cache the operation reads and writes, when there is one to use. Cached bodies
    /// are held per identity, so a request that goes out without credentials — an
    /// [`AuthStrategy`] that signs some other way, or none at all — is not cached: there
    /// would be nothing to tell one caller's copy from another's.
    fn cacheable(&self, operation: &Operation) -> Option<&Arc<dyn ResponseCache>> {
        if !operation.no_cache
            && operation.method == Method::GET
            && operation.accept == "application/json"
        {
            self.shared.cache.as_ref()
        } else {
            None
        }
    }

    /// Sends one request and follows the redirects it is answered with, up to
    /// [`MAX_REDIRECTS`] hops, unless the operation is one that takes the redirect for its
    /// answer. Hands back the URL the answer came from along with the answer.
    ///
    /// Credentials stay on the origin they were meant for: a hop to another origin goes out
    /// without the `Authorization`, the way a browser would send it, which is how a blob
    /// request ends up at the storage service without HEY's token. A 301, 302 or 303 turns
    /// anything but a GET or HEAD into a GET without its body; a 307 or 308 keeps both.
    async fn transmit(
        &self,
        operation: &Operation,
        mut url: Url,
        mut request: Request<Bytes>,
    ) -> Result<(Url, HttpResponse<Body>), Error> {
        let mut hops = 0;
        loop {
            let outgoing = (
                request.method().clone(),
                request.headers().clone(),
                request.body().clone(),
            );
            let response = self.shared.http.send(request).await?;
            let next = if operation.capture_redirects {
                None
            } else {
                redirect_target(&url, &response)
            };
            match next {
                None => return Ok((url, response)),
                Some(_) if hops == MAX_REDIRECTS => {
                    return Err(Error::new(
                        ErrorCode::Network,
                        format!(
                            "{} redirected more than {MAX_REDIRECTS} times",
                            operation.id
                        ),
                    )
                    .retryable());
                }
                Some(next) => {
                    require_secure_endpoint(&next)?;
                    request = redirected(outgoing, response.status(), &url, &next)?;
                    url = next;
                    hops += 1;
                }
            }
        }
    }

    /// The most of an answer to this operation the client will hold. An answer it asked
    /// for as a document it goes on to parse is held to the configured cap; anything else
    /// — a blob, an export, whatever a form request answered — to the fixed
    /// [`MAX_RESPONSE_BODY_BYTES`]. What the server labels the answer does not come into
    /// it: a JSON body sent as a PNG is still capped, and an attachment that turns out to
    /// be text is still not.
    fn buffer_bound(&self, operation: &Operation) -> usize {
        if is_parsed(operation.accept) {
            self.shared.max_response_body_bytes
        } else {
            MAX_RESPONSE_BODY_BYTES
        }
    }

    async fn finish(
        &self,
        operation: &Operation,
        url: &Url,
        final_url: Url,
        response: HttpResponse<Body>,
        cached: Option<(String, CachedResponse)>,
    ) -> Result<Response, Error> {
        let status = response.status();
        let headers = response.headers().clone();

        if status == StatusCode::NOT_MODIFIED {
            return match cached {
                Some((_, entry)) if !entry.etag.is_empty() => Ok(Response {
                    status: StatusCode::OK,
                    headers,
                    body: entry.body,
                    url: final_url,
                    from_cache: true,
                    empty: false,
                }),
                _ => Err(Error::api(
                    304,
                    "304 received but no cached response available",
                )),
            };
        }

        let bound = self.buffer_bound(operation);
        let body = match read_body(response.into_body(), bound, &operation.method, url.path()).await
        {
            Ok(body) => body,
            Err(refusal) if status.is_success() => return Err(refusal),
            // The status is what matters about a failure, and a body the client would not
            // read is no reason to lose it.
            Err(refusal) => {
                return Err(
                    Error::from_response(status, &operation.method, &headers, &[])
                        .refusing(refusal),
                );
            }
        };

        if status.is_success() {
            if let (Some((key, _)), Some(cache)) = (cached, self.cacheable(operation))
                && let Some(etag) = headers.get("etag").and_then(|value| value.to_str().ok())
            {
                cache_set(
                    cache,
                    &key,
                    CachedResponse {
                        etag: etag.to_string(),
                        body: body.clone(),
                    },
                )
                .await;
            }
            Ok(Response {
                status,
                headers,
                body,
                url: final_url,
                from_cache: false,
                empty: false,
            })
        } else if operation.empty_on.contains(&status.as_u16()) {
            Ok(Response {
                status,
                headers,
                body,
                url: final_url,
                from_cache: false,
                empty: true,
            })
        } else {
            Err(Error::from_response(
                status,
                &operation.method,
                &headers,
                &body,
            ))
        }
    }

    /// Sleeps the backoff's wait plus a little jitter, held under the longest wait the
    /// client allows.
    async fn wait(&self, delay: Duration) {
        tokio::time::sleep((delay + self.jitter()).min(self.shared.max_delay)).await;
    }

    /// Sleeps the wait HEY asked for, which the client's ceiling does not shorten: the
    /// server said when it will answer again, and resending sooner only earns another
    /// refusal.
    async fn wait_as_asked(&self, delay: Duration) {
        tokio::time::sleep(delay + self.jitter()).await;
    }

    fn jitter(&self) -> Duration {
        match self.shared.max_jitter.as_millis() {
            0 => Duration::ZERO,
            millis => Duration::from_millis(rand::random_range(0..millis as u64)),
        }
    }

    fn next_delay(&self, delay: Duration) -> Duration {
        (delay * 2).min(self.shared.max_delay)
    }
}

/// An operation the hooks have been told the start of and are still owed the end of. It
/// reports the end whichever way the operation leaves: [`Running::finished`] with the
/// outcome, or the drop that comes instead when the caller abandons the future.
struct Running<'a> {
    hooks: &'a Arc<dyn Hooks>,
    info: &'a OperationInfo,
    state: Option<OperationState>,
    started: Instant,
}

impl Running<'_> {
    fn finished(&mut self, outcome: Result<(), &Error>) {
        if let Some(state) = self.state.take() {
            self.hooks
                .on_operation_end(self.info, state, outcome, self.started.elapsed());
        }
    }
}

impl Drop for Running<'_> {
    fn drop(&mut self) {
        // A state still here is one `finished` never took, which means the future was
        // dropped before the work returned.
        if self.state.is_some() {
            self.finished(Err(&Error::cancelled()));
        }
    }
}

/// What one operation may spend on being resent: the sends it gets in all, the statuses
/// that earn another, and the wait before the first resend.
struct Budget {
    attempts: u32,
    retry_on: &'static [u16],
    delay: Duration,
}

/// One answer from HEY with its body unread: what the retry loop settled on, the URL it
/// came from once any redirects were followed, and what the hooks still have to be told
/// about it once the body has been dealt with.
struct Answered {
    url: Url,
    response: HttpResponse<Body>,
    cached: Option<(String, CachedResponse)>,
    info: RequestInfo,
    duration: Duration,
    retryable: bool,
    retry_after: Option<u64>,
}

/// The cache is whatever the caller supplied, and the one the SDK ships keeps its entries in
/// files. So every read and write of it goes to the blocking pool: a file read on the
/// runtime's own thread stalls every other task sharing that thread. A cache that cannot be
/// reached — the pool shutting down under it — is a miss, which is what any other failure to
/// read it is too.
async fn cache_get(cache: &Arc<dyn ResponseCache>, key: &str) -> Option<CachedResponse> {
    let cache = cache.clone();
    let key = key.to_string();
    tokio::task::spawn_blocking(move || cache.get(&key))
        .await
        .ok()
        .flatten()
}

async fn cache_set(cache: &Arc<dyn ResponseCache>, key: &str, response: CachedResponse) {
    let cache = cache.clone();
    let key = key.to_string();
    let _ = tokio::task::spawn_blocking(move || cache.set(&key, response)).await;
}

async fn cache_invalidate(cache: &Arc<dyn ResponseCache>, key: &str) {
    let cache = cache.clone();
    let key = key.to_string();
    let _ = tokio::task::spawn_blocking(move || cache.invalidate(&key)).await;
}

#[cfg(feature = "reqwest")]
fn shipped_http_client(timeout: Duration) -> Result<Arc<dyn HttpClient>, Error> {
    Ok(Arc::new(crate::http::ReqwestClient::with_timeout(timeout)?))
}

#[cfg(not(feature = "reqwest"))]
fn shipped_http_client(_timeout: Duration) -> Result<Arc<dyn HttpClient>, Error> {
    Err(Error::usage(
        "no HTTP client: supply one with ClientBuilder::http_client, or enable the reqwest feature",
    ))
}

/// Where a redirect points, when the answer is one and says where. A 3xx without a
/// `Location`, or with one that is not a URL, is handed back as the answer it is.
fn redirect_target(url: &Url, response: &HttpResponse<Body>) -> Option<Url> {
    let status = response.status();
    if status.is_redirection() && status != StatusCode::NOT_MODIFIED {
        response
            .headers()
            .get("location")
            .and_then(|value| value.to_str().ok())
            .and_then(|location| url.join(location).ok())
    } else {
        None
    }
}

/// The request to send to `next` on the way there from `from`: the same one, less the
/// credentials when the origin changes, and reduced to a GET when the status asks for it.
fn redirected(
    (method, mut headers, body): (Method, HeaderMap, Bytes),
    status: StatusCode,
    from: &Url,
    next: &Url,
) -> Result<Request<Bytes>, Error> {
    let keeps_method = method == Method::GET
        || method == Method::HEAD
        || status == StatusCode::TEMPORARY_REDIRECT
        || status == StatusCode::PERMANENT_REDIRECT;
    let (method, body) = if keeps_method {
        (method, body)
    } else {
        headers.remove(CONTENT_TYPE);
        headers.remove(CONTENT_LENGTH);
        (Method::GET, Bytes::new())
    };
    if !is_same_origin(next, from) {
        headers.remove(AUTHORIZATION);
        headers.remove(COOKIE);
        headers.remove(PROXY_AUTHORIZATION);
    }
    let mut request = Request::builder()
        .method(method)
        .uri(next.as_str())
        .body(body)
        .map_err(Error::from_std)?;
    *request.headers_mut() = headers;
    Ok(request)
}

fn parse_base_url(base_url: &str) -> Result<Url, Error> {
    let mut url = Url::parse(base_url)
        .map_err(|error| Error::usage(format!("base URL {base_url}: {error}")))?;
    require_secure_endpoint(&url)?;
    if !url.path().ends_with('/') {
        url.set_path(&format!("{}/", url.path()));
    }
    Ok(url)
}

/// HEY answers JSON to paths that end in `.json`. The model leaves the extension off
/// paths that end in a parameter, since Smithy cannot express `{id}.json`, so it is put
/// back here unless the last segment already carries an extension.
pub(crate) fn with_json_extension(path: &str) -> String {
    let last_segment = path.rsplit('/').next().unwrap_or_default();
    if path.is_empty() || path.ends_with('/') || last_segment.contains('.') {
        path.to_string()
    } else {
        format!("{path}.json")
    }
}

/// The span an operation runs in: one of its own, or none for a quiet send, which is one
/// request inside another operation and runs in that operation's span.
fn span_for(operation: &Operation) -> OperationSpan {
    if operation.quiet {
        OperationSpan::none()
    } else {
        OperationSpan::new(operation)
    }
}

/// HEY's own id for the request, when the answer names one.
fn request_id(headers: &HeaderMap) -> Option<&str> {
    headers
        .get("x-request-id")
        .and_then(|value| value.to_str().ok())
}

/// The wait HEY asked for, on the two statuses that carry one.
fn retry_after_asked(status: StatusCode, headers: &HeaderMap) -> Option<u64> {
    if status == StatusCode::TOO_MANY_REQUESTS || status == StatusCode::SERVICE_UNAVAILABLE {
        retry_after_seconds(headers)
    } else {
        None
    }
}

fn header_value(value: &str) -> Result<HeaderValue, Error> {
    HeaderValue::from_str(value)
        .map_err(|_| Error::usage(format!("{value:?} is not a valid header value")))
}

/// Whether the answer to a request that asked for this is a document the SDK buffers and
/// parses. Anything it did not ask for as JSON or HTML — a blob's `*/*`, an export's
/// `text/csv` — it streams or holds under its own bound instead.
fn is_parsed(accept: &str) -> bool {
    accept.is_empty()
        || accept.split(',').any(|part| {
            let media_type = part.split(';').next().unwrap_or_default().trim();
            media_type == "application/json"
                || media_type.ends_with("+json")
                || media_type == "text/html"
        })
}

/// Reads a body up to the bound and refuses it on the first byte past. A body exactly at
/// the bound reads whole; one declared past it never starts.
pub(crate) async fn read_body(
    body: Body,
    limit: usize,
    method: &Method,
    path: &str,
) -> Result<Bytes, Error> {
    body.collect(limit, || Error::response_too_large(limit, method, path))
        .await
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use async_trait::async_trait;
    use serde_json::Value;

    use super::*;
    use crate::auth::StaticTokenProvider;

    /// An [`HttpClient`] with no network behind it: it answers each request from a closure
    /// and keeps what it was sent. This is the second implementation the trait exists for,
    /// so the client is exercised here with no `reqwest` in the picture.
    struct Canned {
        answer: Box<Answer>,
        sent: Mutex<Vec<(Method, String, HeaderMap)>>,
    }

    type Answer = dyn Fn(&Request<Bytes>) -> HttpResponse<Body> + Send + Sync;

    impl Canned {
        fn new(
            answer: impl Fn(&Request<Bytes>) -> HttpResponse<Body> + Send + Sync + 'static,
        ) -> Arc<Canned> {
            Arc::new(Canned {
                answer: Box::new(answer),
                sent: Mutex::new(Vec::new()),
            })
        }

        fn sent(&self) -> Vec<(Method, String, HeaderMap)> {
            self.sent.lock().unwrap().clone()
        }
    }

    #[async_trait]
    impl HttpClient for Arc<Canned> {
        async fn send(&self, request: Request<Bytes>) -> Result<HttpResponse<Body>, Error> {
            self.sent.lock().unwrap().push((
                request.method().clone(),
                request.uri().to_string(),
                request.headers().clone(),
            ));
            Ok((self.answer)(&request))
        }
    }

    fn answer(status: u16, body: &'static str) -> HttpResponse<Body> {
        let mut response = HttpResponse::new(Body::from(body));
        *response.status_mut() = StatusCode::from_u16(status).unwrap();
        response
    }

    fn redirect(location: &str) -> HttpResponse<Body> {
        let mut response = answer(302, "");
        response
            .headers_mut()
            .insert("location", HeaderValue::from_str(location).unwrap());
        response
    }

    fn client_over(http: Arc<Canned>) -> Client {
        Client::builder(Config::default().with_base_url("https://hey.test"))
            .token_provider(StaticTokenProvider::new("secret"))
            .http_client(http)
            .max_retries(0)
            .build()
            .unwrap()
    }

    #[tokio::test]
    async fn a_request_goes_out_on_the_supplied_http_client_with_credentials() {
        let http = Canned::new(|_| answer(200, r#"{"ok":true}"#));
        let client = client_over(http.clone());

        let body: Value = client
            .send(client.request(Method::GET, "/boxes"))
            .await
            .unwrap();

        assert_eq!(body, serde_json::json!({ "ok": true }));
        let sent = http.sent();
        assert_eq!(sent.len(), 1);
        assert_eq!(sent[0].0, Method::GET);
        assert_eq!(sent[0].1, "https://hey.test/boxes.json");
        assert_eq!(sent[0].2[AUTHORIZATION], "Bearer secret");
    }

    #[tokio::test]
    async fn a_redirect_on_the_same_origin_is_followed_with_credentials() {
        let http = Canned::new(|request| {
            if request.uri().path() == "/old.json" {
                redirect("/new.json")
            } else {
                answer(200, r#"{"moved":true}"#)
            }
        });
        let client = client_over(http.clone());

        let response = client
            .execute(client.request(Method::GET, "/old"))
            .await
            .unwrap();

        assert_eq!(response.url.as_str(), "https://hey.test/new.json");
        assert_eq!(response.body, r#"{"moved":true}"#);
        let sent = http.sent();
        assert_eq!(sent.len(), 2);
        assert_eq!(sent[1].1, "https://hey.test/new.json");
        assert_eq!(sent[1].2[AUTHORIZATION], "Bearer secret");
    }

    #[tokio::test]
    async fn an_html_read_asks_for_the_page_as_hey_serves_it() {
        let http = Canned::new(|_| {
            answer(
                200,
                r#"<section id="container_workflow_stage_5512"></section>"#,
            )
        });
        let client = client_over(http.clone());

        let page = client.workflows().get_stage(8801, 5512).await.unwrap();

        assert_eq!(
            page,
            r#"<section id="container_workflow_stage_5512"></section>"#
        );
        let sent = http.sent();
        assert_eq!(sent.len(), 1);
        assert_eq!(sent[0].1, "https://hey.test/workflows/8801/stages/5512");
        assert_eq!(sent[0].2[ACCEPT], "text/html");
        assert_eq!(sent[0].2[AUTHORIZATION], "Bearer secret");
    }

    #[tokio::test]
    async fn a_redirect_off_the_origin_is_followed_without_credentials() {
        let http = Canned::new(|request| {
            if request.uri().host() == Some("hey.test") {
                redirect("https://storage.test/blobs/1")
            } else {
                answer(200, "the bytes")
            }
        });
        let client = client_over(http.clone());

        let response = client.get_blob("/blobs/1").await.unwrap();

        assert_eq!(response.body, "the bytes");
        let sent = http.sent();
        assert_eq!(sent.len(), 2);
        assert_eq!(sent[1].1, "https://storage.test/blobs/1");
        assert!(sent[1].2.get(AUTHORIZATION).is_none());
    }

    #[tokio::test]
    async fn a_redirect_to_plain_http_elsewhere_is_refused() {
        let http = Canned::new(|_| redirect("http://evil.test/"));
        let client = client_over(http.clone());

        let error = client.get("/anything").await.unwrap_err();

        assert_eq!(error.code(), ErrorCode::Usage);
        assert_eq!(http.sent().len(), 1);
    }

    #[tokio::test]
    async fn a_redirect_loop_is_given_up_on() {
        let http = Canned::new(|_| redirect("/again"));
        let client = client_over(http.clone());

        let error = client.get("/again").await.unwrap_err();

        assert_eq!(error.code(), ErrorCode::Network);
        assert_eq!(http.sent().len(), MAX_REDIRECTS + 1);
    }

    #[tokio::test]
    async fn a_form_request_keeps_its_redirect_rather_than_following_it() {
        let http = Canned::new(|_| redirect("/workflows/8801"));
        let client = client_over(http.clone());

        let created = client
            .post_form("/workflows", &[("workflow[name]", "Launch")])
            .await
            .unwrap();

        assert_eq!(created.location.as_deref(), Some("/workflows/8801"));
        assert_eq!(http.sent().len(), 1);
    }

    #[test]
    fn json_extension_is_added_only_where_missing() {
        assert_eq!(with_json_extension("/boxes/123"), "/boxes/123.json");
        assert_eq!(with_json_extension("/boxes.json"), "/boxes.json");
        assert_eq!(
            with_json_extension("/calendar/days/2026-03-04/journal_entry"),
            "/calendar/days/2026-03-04/journal_entry.json"
        );
        assert_eq!(
            with_json_extension("/rails/active_storage/direct_uploads.json"),
            "/rails/active_storage/direct_uploads.json"
        );
        assert_eq!(with_json_extension("/boxes/"), "/boxes/");
    }

    #[test]
    fn base_url_must_be_https_or_local() {
        assert!(parse_base_url("https://app.hey.com").is_ok());
        assert!(parse_base_url("http://127.0.0.1:3000").is_ok());
        assert_eq!(
            parse_base_url("http://evil.example.com")
                .unwrap_err()
                .code(),
            crate::ErrorCode::Usage
        );
    }
}
