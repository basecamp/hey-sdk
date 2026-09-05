use async_trait::async_trait;
use reqwest::Request;
use reqwest::header::{AUTHORIZATION, HeaderValue};

use crate::error::Error;
use crate::types::SensitiveString;

/// Supplies the access token each request goes out with.
#[async_trait]
pub trait TokenProvider: Send + Sync {
    async fn access_token(&self) -> Result<String, Error>;

    /// Asked once when a request is answered with 401. Answer `true` when the next
    /// `access_token` will hand out renewed credentials, and the request is sent again.
    async fn refresh(&self) -> bool {
        false
    }
}

/// A fixed token, from an environment variable say. It prints as `[REDACTED]`, so a `{:?}`
/// of the provider — or of anything holding one — cannot put the token in a log.
#[derive(Debug, Clone)]
pub struct StaticTokenProvider {
    pub token: SensitiveString,
}

impl StaticTokenProvider {
    pub fn new(token: impl Into<SensitiveString>) -> StaticTokenProvider {
        StaticTokenProvider {
            token: token.into(),
        }
    }
}

#[async_trait]
impl TokenProvider for StaticTokenProvider {
    async fn access_token(&self) -> Result<String, Error> {
        if self.token.is_empty() {
            Err(Error::auth("no token configured"))
        } else {
            Ok(self.token.expose().to_string())
        }
    }
}

/// Puts credentials on a request. The default, [`BearerAuth`], sets an `Authorization`
/// header from a [`TokenProvider`]; anything else can plug in here.
#[async_trait]
pub trait AuthStrategy: Send + Sync {
    async fn authenticate(&self, request: &mut Request) -> Result<(), Error>;

    /// Asked once when a request is answered with 401; see [`TokenProvider::refresh`].
    async fn refresh(&self) -> bool {
        false
    }
}

pub struct BearerAuth<P: TokenProvider> {
    provider: P,
}

impl<P: TokenProvider> BearerAuth<P> {
    pub fn new(provider: P) -> BearerAuth<P> {
        BearerAuth { provider }
    }
}

#[async_trait]
impl<P: TokenProvider> AuthStrategy for BearerAuth<P> {
    async fn authenticate(&self, request: &mut Request) -> Result<(), Error> {
        let token = self.provider.access_token().await?;
        let value = HeaderValue::from_str(&format!("Bearer {token}"))
            .map_err(|_| Error::auth("access token is not a valid header value"))?;
        request.headers_mut().insert(AUTHORIZATION, value);
        Ok(())
    }

    async fn refresh(&self) -> bool {
        self.provider.refresh().await
    }
}
