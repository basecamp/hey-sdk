//! The HTTP layer the client sends on, and the seam for replacing it.
//!
//! Everything the SDK sends goes out through one [`HttpClient`]. The one it ships,
//! [`ReqwestClient`], is what [`crate::Client::new`] builds; an application that already
//! has an HTTP stack — a mobile shell on the platform's own, a test on a canned answer —
//! implements the trait and hands it to [`crate::ClientBuilder::http_client`].
//!
//! The request and response types are the `http` crate's, re-exported here so a caller
//! needs no dependency of its own to name a [`Method`] or read a [`StatusCode`].

use std::pin::Pin;

use async_trait::async_trait;
use bytes::{Bytes, BytesMut};
use futures_util::{Stream, StreamExt, stream};

use crate::error::Error;

pub use ::http::header::{self, HeaderMap, HeaderName, HeaderValue};
pub use ::http::{Method, Request, Response, StatusCode, Version};

#[cfg(feature = "reqwest")]
mod reqwest;
#[cfg(feature = "reqwest")]
#[cfg_attr(docsrs, doc(cfg(feature = "reqwest")))]
pub use self::reqwest::ReqwestClient;

/// Sends one HTTP request and answers with the response, its body still unread.
///
/// The Go SDK takes an `*http.Client` and leaves the transport to it. Rust takes a trait
/// instead: the shells this SDK exists for bring the platform's own HTTP stack, and a
/// second one in the binary is the wrong price for a client library. The SDK does the rest
/// above this seam — credentials, retries, redirects, the response cache, the body caps —
/// so an implementation is only a transport.
///
/// An implementation **must not follow redirects**. The SDK follows them itself, so it can
/// keep credentials on the HEY origin and read the `Location` a form request was made for.
/// One that follows them anyway loses that `Location`, and the SDK cannot tell that it did.
///
/// Timeouts belong to the implementation, since the SDK has no way to interrupt a transport
/// it does not know. A failure to get an answer at all — no connection, a timeout, a broken
/// stream — is [`Error::network`]; a response with any status is `Ok`.
#[async_trait]
pub trait HttpClient: Send + Sync {
    async fn send(&self, request: Request<Bytes>) -> Result<Response<Body>, Error>;
}

/// A response body as it arrives, read once.
///
/// An implementation builds one with [`Body::from_stream`], passing along the length the
/// transport knows so a body declared past the caller's cap is refused before a byte of it
/// is read. The SDK reads it with [`Body::chunk`] or [`Body::collect`].
pub struct Body {
    stream: Pin<Box<dyn Stream<Item = Result<Bytes, Error>> + Send>>,
    content_length: Option<u64>,
}

impl Body {
    pub fn from_stream(
        stream: impl Stream<Item = Result<Bytes, Error>> + Send + 'static,
        content_length: Option<u64>,
    ) -> Body {
        Body {
            stream: Box::pin(stream),
            content_length,
        }
    }

    pub fn empty() -> Body {
        Body::from(Bytes::new())
    }

    /// What the transport declared the body's length to be, when it declared one.
    pub fn content_length(&self) -> Option<u64> {
        self.content_length
    }

    /// The next piece of the body, or `None` once it has all arrived.
    pub async fn chunk(&mut self) -> Result<Option<Bytes>, Error> {
        self.stream.next().await.transpose()
    }

    /// Reads the body whole, up to `limit` bytes, and answers `too_large` on the first byte
    /// past. A body exactly at the limit reads whole; one declared past it never starts.
    pub async fn collect(
        mut self,
        limit: usize,
        too_large: impl Fn() -> Error,
    ) -> Result<Bytes, Error> {
        if self
            .content_length
            .is_some_and(|length| length > limit as u64)
        {
            return Err(too_large());
        }
        let mut body = BytesMut::new();
        while let Some(chunk) = self.chunk().await? {
            if body.len() + chunk.len() > limit {
                return Err(too_large());
            }
            body.extend_from_slice(&chunk);
        }
        Ok(body.freeze())
    }
}

impl From<Bytes> for Body {
    fn from(bytes: Bytes) -> Body {
        let content_length = Some(bytes.len() as u64);
        Body::from_stream(stream::once(async move { Ok(bytes) }), content_length)
    }
}

impl From<Vec<u8>> for Body {
    fn from(bytes: Vec<u8>) -> Body {
        Body::from(Bytes::from(bytes))
    }
}

impl From<&'static str> for Body {
    fn from(text: &'static str) -> Body {
        Body::from(Bytes::from_static(text.as_bytes()))
    }
}

impl std::fmt::Debug for Body {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Body")
            .field("content_length", &self.content_length)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn too_large() -> Error {
        Error::api(0, "too large")
    }

    #[tokio::test]
    async fn collects_a_body_up_to_the_limit() {
        let body = Body::from(Bytes::from_static(b"hello"));
        assert_eq!(body.content_length(), Some(5));
        assert_eq!(body.collect(5, too_large).await.unwrap(), "hello");
    }

    #[tokio::test]
    async fn refuses_a_body_declared_past_the_limit_before_reading_it() {
        let body = Body::from_stream(
            stream::once(async { panic!("the body should never be read") }),
            Some(6),
        );
        assert_eq!(
            body.collect(5, too_large).await.unwrap_err().to_string(),
            "too large"
        );
    }

    #[tokio::test]
    async fn refuses_an_undeclared_body_on_the_first_byte_past_the_limit() {
        let chunks = stream::iter([
            Ok(Bytes::from_static(b"hel")),
            Ok(Bytes::from_static(b"lo!")),
        ]);
        let body = Body::from_stream(chunks, None);
        assert_eq!(
            body.collect(5, too_large).await.unwrap_err().to_string(),
            "too large"
        );
    }

    #[tokio::test]
    async fn a_failing_stream_fails_the_read() {
        let chunks = stream::iter([
            Ok(Bytes::from_static(b"hel")),
            Err(Error::api(0, "cut off")),
        ]);
        let body = Body::from_stream(chunks, None);
        assert_eq!(
            body.collect(100, too_large).await.unwrap_err().to_string(),
            "cut off"
        );
    }
}
