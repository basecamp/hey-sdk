//! The Rust client for the [HEY](https://www.hey.com) API.
//!
//! Types, routes and service methods are generated from the Smithy model in the
//! repository's `spec/` directory; everything else in this crate is the plumbing they
//! share: authentication, retries, redirects, the response cache, pagination and account
//! scope. All of it sends through one [`http::HttpClient`], which an application may
//! replace with its own; the `reqwest` feature, on by default, ships one.
//!
//! ```no_run
//! use hey_sdk::{Client, Config, StaticTokenProvider};
//!
//! # async fn run() -> Result<(), hey_sdk::Error> {
//! let client = Client::new(Config::default(), StaticTokenProvider::new(std::env::var("HEY_TOKEN").unwrap()))?;
//! for mailbox in client.boxes().list().await?.iter() {
//!     println!("{} ({})", mailbox.name, mailbox.kind);
//! }
//! # Ok(())
//! # }
//! ```

// docs.rs builds with `--cfg docsrs` on nightly, where `doc_cfg` badges the items that
// exist only with a feature on.
#![cfg_attr(docsrs, feature(doc_cfg))]
#![cfg_attr(not(test), deny(clippy::unwrap_used, clippy::expect_used))]

mod account_scope;
/// Who a request goes out as: the token it carries and the strategy that puts it on.
pub mod auth;
/// The response cache: JSON reads kept by `ETag`, so a repeated one is answered from a 304.
pub mod cache;
/// The client, how it is built, and the send every call goes through.
pub mod client;
/// What a client needs to know before it can talk to HEY, and where that is read from.
pub mod config;
/// The error every call can answer with, its categories, and the exit status each maps to.
pub mod error;
pub mod form;
mod generated;
pub mod http;
/// Signing in to HEY over OAuth 2.0: discovery, the authorization code flow with PKCE, and
/// refresh.
pub mod oauth;
pub mod observability;
/// A request the client has not sent yet, and everything a caller may say about it first.
pub mod operation;
/// Paginated reads: one page with the cursor for the next, and the walks that follow it.
pub mod pagination;
mod raw;
pub mod resilience;
/// What the model says about an operation: its method, its path template and how it behaves.
pub mod route;
/// The checks that keep credentials on the HEY origin, and the redaction that keeps them out
/// of logs.
pub mod security;
pub mod services;
mod trace;
/// The scalars HEY's records are made of: dates, instants, and strings that must not be
/// logged.
pub mod types;
/// Recognizing HEY paths and URLs as pasted from the web app, offline.
pub mod url;
/// The SDK's version, the API contract it was built against, and the `User-Agent` naming
/// both.
pub mod version;

pub use auth::{AuthStrategy, BearerAuth, StaticTokenProvider, TokenProvider};
pub use client::{Client, ClientBuilder, Response};
pub use config::Config;
pub use error::{Error, ErrorCode};
pub use form::FormResponse;
pub use http::HttpClient;
pub use operation::Operation;
pub use pagination::Page;
pub use types::{Date, DateTime, SensitiveString};
pub use version::{API_VERSION, VERSION};

/// The request and response types HEY speaks, generated from the model.
pub mod models {
    pub use crate::generated::types::*;
}

/// Every modelled route, generated from the model.
pub mod routes {
    pub use crate::generated::routes::*;
    pub use crate::route::{Pagination, ParamKind, ParamRole, Retry, Route, RouteParam};
}
