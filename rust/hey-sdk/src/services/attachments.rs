//! Uploading an outgoing attachment, which takes two requests: HEY reserves an Active
//! Storage blob and names a storage URL, and the bytes then go to that URL rather than to
//! HEY.

use base64::Engine;
use base64::engine::general_purpose::STANDARD;
use bytes::Bytes;
use reqwest::Method;
use reqwest::header::{AUTHORIZATION, HeaderMap, HeaderName, HeaderValue};
use url::Url;

use crate::client::{Client, MAX_RESPONSE_BODY_BYTES, read_body};
use crate::error::Error;
use crate::generated::types::{
    CreateDirectUploadRequestContent, DirectUpload, DirectUploadBlob, DirectUploadTarget,
};
use crate::security::require_secure_endpoint;

pub use crate::generated::services::attachments::*;

/// What an attachment is taken to be when the caller names no content type.
const DEFAULT_CONTENT_TYPE: &str = "application/octet-stream";

impl<'a> Attachments<'a> {
    /// Reserves an Active Storage blob and uploads the bytes to the storage URL HEY named.
    /// The answer's `attachable_sgid` is what embeds the attachment in Trix rich text.
    ///
    /// Empty content is an empty attachment rather than a mistake, so only a missing
    /// filename is refused.
    pub async fn upload(
        &self,
        filename: &str,
        content_type: Option<&str>,
        content: impl Into<Bytes>,
    ) -> Result<DirectUpload, Error> {
        if filename.is_empty() {
            return Err(Error::usage("an attachment needs a filename"));
        }

        let content = content.into();
        let body = CreateDirectUploadRequestContent {
            blob: DirectUploadBlob {
                filename: filename.to_string(),
                byte_size: content.len() as i64,
                checksum: STANDARD.encode(md5::compute(&content).0),
                content_type: content_type.unwrap_or(DEFAULT_CONTENT_TYPE).to_string(),
            },
        };
        let upload = reserved(self.create_direct_upload(&body).await?)?;
        store(self.client(), &upload.direct_upload, content).await?;
        Ok(upload)
    }
}

/// The blob HEY reserved, once it carries everything the upload needs. HEY answers 200 with
/// a payload rather than a status when it has nothing to give, so the fields are what say
/// whether there is an upload to make.
fn reserved(upload: DirectUpload) -> Result<DirectUpload, Error> {
    if upload.signed_id.is_empty()
        || upload.attachable_sgid.is_empty()
        || upload.direct_upload.url.is_empty()
    {
        Err(Error::api(
            0,
            "HEY returned an empty attachment upload response",
        ))
    } else {
        Ok(upload)
    }
}

/// Puts the bytes to the storage service.
///
/// This is the one request the SDK makes outside the HEY API. The storage URL
/// authenticates itself and takes exactly the headers HEY named — including any
/// `Authorization` the storage service wants, which is why the HEY credentials must not
/// ride along. Going through [`crate::Client::execute`] would attach them, so the request
/// is built here and sent on the client's own `reqwest::Client`, which carries the
/// connection pool, the timeout and whatever else the caller configured.
///
/// A storage service answers a failure with a document of its own, and that is all this
/// reads: the answer is held to [`MAX_RESPONSE_BODY_BYTES`] so a service saying too much
/// cannot be what runs the caller out of memory.
async fn store(client: &Client, target: &DirectUploadTarget, content: Bytes) -> Result<(), Error> {
    let url = Url::parse(&target.url)?;
    require_secure_endpoint(&url)
        .map_err(|error| Error::usage(format!("unsafe attachment upload target: {error}")))?;

    let path = url.path().to_string();
    let answered = client
        .http()
        .put(url)
        .headers(storage_headers(target)?)
        .body(content)
        .send()
        .await
        .map_err(Error::network)?;

    let status = answered.status();
    let headers = answered.headers().clone();
    let body = read_body(answered, MAX_RESPONSE_BODY_BYTES, &Method::PUT, &path).await?;
    if status.is_success() {
        Ok(())
    } else {
        Err(Error::from_response(status, &Method::PUT, &headers, &body))
    }
}

/// The headers HEY named, with any `Authorization` among them dropped: the SDK's own
/// credentials never reach the storage service, and neither does a stale one HEY echoed.
fn storage_headers(target: &DirectUploadTarget) -> Result<HeaderMap, Error> {
    let mut headers = HeaderMap::new();
    for (name, value) in target.headers.iter().flatten() {
        let name = HeaderName::from_bytes(name.as_bytes())
            .map_err(|_| Error::api(0, format!("{name:?} is not a valid header name")))?;
        let value = HeaderValue::from_str(value)
            .map_err(|_| Error::api(0, format!("{name} carries an unsendable value")))?;
        headers.insert(name, value);
    }
    headers.remove(AUTHORIZATION);
    Ok(headers)
}
