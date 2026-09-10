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

mod account_scope;
pub mod auth;
pub mod cache;
pub mod client;
pub mod config;
pub mod error;
pub mod form;
mod generated;
pub mod http;
pub mod oauth;
pub mod observability;
pub mod operation;
pub mod pagination;
mod raw;
pub mod resilience;
pub mod route;
pub mod security;
pub mod services;
mod trace;
pub mod types;
pub mod url;
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
