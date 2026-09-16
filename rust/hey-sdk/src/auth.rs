use async_trait::async_trait;
use bytes::Bytes;

use crate::error::Error;
use crate::http::header::AUTHORIZATION;
use crate::http::{HeaderValue, Request};
use crate::types::SensitiveString;

/// Supplies the access token each request goes out with.
#[async_trait]
pub trait TokenProvider: Send + Sync {
    /// The token to send, asked for on every request. A provider that renews of its own
    /// accord — handing over a new token ahead of the old one's expiry, as OAuth libraries
    /// do — need do nothing more: the client takes a token other than the one it last
    /// signed with for the renewal it is, so a 401 on the old token is answered by resending
    /// with the new one, and [`refresh`](TokenProvider::refresh) is not asked.
    async fn access_token(&self) -> Result<String, Error>;

    /// Asked once when a request is answered with 401. Answer `true` when the next
    /// `access_token` will hand out renewed credentials, and the request is sent again.
    /// Either answer is for every request signed with the credentials that earned the 401,
    /// not only the one that asked: a `true` resends them all on the new credentials, and
    /// a `false` fails them all, so an outage at the token's issuer costs one call per set
    /// of credentials. A request signed after a `false` asks again. Not asked for a 401 on
    /// a token this provider has already replaced — one `access_token` no longer hands out,
    /// whether or not the replacement has signed anything yet — since that is a renewal
    /// already made: the request is resent with the replacement, and a rotating refresh
    /// token the provider was just issued is not spent again over the top of it.
    async fn refresh(&self) -> bool {
        false
    }
}

/// A fixed token, from an environment variable say. It prints as `[REDACTED]`, so a `{:?}`
/// of the provider — or of anything holding one — cannot put the token in a log.
#[derive(Debug, Clone)]
pub struct StaticTokenProvider {
    /// The token every request goes out with.
    pub token: SensitiveString,
}

impl StaticTokenProvider {
    /// A provider that hands out `token` and nothing else.
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

/// A provider behind an `Arc` is a provider, so one can be shared with the client and
/// kept by the application — to watch its refreshes, say.
#[async_trait]
impl<P: TokenProvider + ?Sized> TokenProvider for std::sync::Arc<P> {
    async fn access_token(&self) -> Result<String, Error> {
        (**self).access_token().await
    }

    async fn refresh(&self) -> bool {
        (**self).refresh().await
    }
}

/// Puts credentials on a request. The default, [`BearerAuth`], sets an `Authorization`
/// header from a [`TokenProvider`]; anything else can plug in here.
#[async_trait]
pub trait AuthStrategy: Send + Sync {
    /// Puts the credentials on a request about to be sent.
    async fn authenticate(&self, request: &mut Request<Bytes>) -> Result<(), Error>;

    /// Asked once when a request is answered with 401; see [`TokenProvider::refresh`].
    async fn refresh(&self) -> bool {
        false
    }
}

/// Marks a request the SDK's own [`BearerAuth`] signed, so the client can read the bearer
/// it put on for the token it is — and count one other than the last as a renewal the
/// provider made on its own. A strategy of the caller's leaves no mark, since its headers
/// may legitimately differ from one request to the next; they are never read this way.
#[derive(Clone, Copy)]
pub(crate) struct BearerSigned;

/// Sends the token as `Authorization: Bearer`, which is how HEY takes one.
pub struct BearerAuth<P: TokenProvider> {
    provider: P,
}

impl<P: TokenProvider> BearerAuth<P> {
    /// Bearer authentication over the given provider's tokens.
    pub fn new(provider: P) -> BearerAuth<P> {
        BearerAuth { provider }
    }
}

#[async_trait]
impl<P: TokenProvider> AuthStrategy for BearerAuth<P> {
    async fn authenticate(&self, request: &mut Request<Bytes>) -> Result<(), Error> {
        let token = self.provider.access_token().await?;
        let value = HeaderValue::from_str(&format!("Bearer {token}"))
            .map_err(|_| Error::auth("access token is not a valid header value"))?;
        request.headers_mut().insert(AUTHORIZATION, value);
        request.extensions_mut().insert(BearerSigned);
        Ok(())
    }

    async fn refresh(&self) -> bool {
        self.provider.refresh().await
    }
}
