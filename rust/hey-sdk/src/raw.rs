//! The verbs for the parts of HEY the model does not describe. They take a path rather
//! than a route and hand back the answer undecoded, but everything else is what a modelled
//! call gets: the credentials, the account scope, the hooks, the retries and the resend
//! after a refreshed 401.
//!
//! A path here goes out as it was written — no `.json` suffix is added — since a caller
//! reaching past the model is naming a path HEY serves, not one Smithy had to spell
//! around.

use bytes::Bytes;
use reqwest::Method;
use reqwest::header::HeaderMap;
use serde::Serialize;
use tokio::io::{AsyncWrite, AsyncWriteExt};
use url::Url;

use crate::client::{Client, Response};
use crate::error::Error;
use crate::form::FormResponse;
use crate::operation::Operation;
use crate::security::is_same_origin;

impl Client {
    pub async fn get(&self, path: &str) -> Result<Response, Error> {
        self.execute(self.raw(Method::GET, path)?).await
    }

    /// Reads the HTML representation, for the pages HEY serves no JSON for.
    pub async fn get_html(&self, path: &str) -> Result<Response, Error> {
        let mut operation = self.raw(Method::GET, path)?;
        operation.accept("text/html");
        self.execute(operation).await
    }

    /// Reads an export, which HEY streams as a file rather than a document.
    pub async fn get_csv(&self, path: &str) -> Result<Response, Error> {
        self.execute(self.csv(path)?).await
    }

    /// A request for one of the exports HEY streams as a file. This and [`Client::get_csv`]
    /// stand to each other as [`Client::form`] does to [`Client::post_form`]: a call that
    /// has something to say about itself builds the operation here and gives it an
    /// [`Operation::info`] before sending it.
    pub fn csv(&self, path: &str) -> Result<Operation, Error> {
        let mut operation = self.raw(Method::GET, path)?;
        operation.accept("text/csv");
        Ok(operation)
    }

    /// Reads a file whole. [`Client::download_blob`] writes one out as it arrives instead,
    /// for a file too large to want in memory.
    pub async fn get_blob(&self, path: &str) -> Result<Response, Error> {
        self.execute(self.blob(path)?).await
    }

    /// Writes a file to `destination` as it arrives, and answers how many bytes went and
    /// what headers came with them. Nothing is read into memory, and nothing is resent:
    /// once bytes are on their way to the destination a second attempt would double them.
    pub async fn download_blob(
        &self,
        path: &str,
        destination: &mut (impl AsyncWrite + Unpin),
    ) -> Result<(u64, HeaderMap), Error> {
        let mut response = self.stream(self.blob(path)?).await?;
        let headers = response.headers().clone();
        let mut written = 0;
        while let Some(chunk) = response.chunk().await.map_err(Error::network)? {
            destination
                .write_all(&chunk)
                .await
                .map_err(Error::from_std)?;
            written += chunk.len() as u64;
        }
        Ok((written, headers))
    }

    pub async fn post(&self, path: &str, body: &impl Serialize) -> Result<Response, Error> {
        self.send_json(Method::POST, path, body, "application/json")
            .await
    }

    /// Posts to an endpoint that may answer with something other than JSON.
    pub async fn post_mutation(
        &self,
        path: &str,
        body: &impl Serialize,
    ) -> Result<Response, Error> {
        self.send_json(Method::POST, path, body, "*/*").await
    }

    pub async fn put(&self, path: &str, body: &impl Serialize) -> Result<Response, Error> {
        self.send_json(Method::PUT, path, body, "application/json")
            .await
    }

    pub async fn patch(&self, path: &str, body: &impl Serialize) -> Result<Response, Error> {
        self.send_json(Method::PATCH, path, body, "application/json")
            .await
    }

    /// Patches an endpoint that may answer with something other than JSON.
    pub async fn patch_mutation(
        &self,
        path: &str,
        body: &impl Serialize,
    ) -> Result<Response, Error> {
        self.send_json(Method::PATCH, path, body, "*/*").await
    }

    pub async fn delete(&self, path: &str) -> Result<Response, Error> {
        self.execute(self.raw(Method::DELETE, path)?).await
    }

    /// Posts a form the way a browser would, and captures the redirect HEY answers with
    /// rather than following it. [`FormResponse::extract_id`] reads the created record's
    /// id out of that redirect.
    ///
    /// This and the three below are [`Client::form`] and [`Client::send_form`] together. A
    /// call that has something to say about itself builds the operation with those two
    /// instead, and gives it an [`Operation::info`] on the way.
    pub async fn post_form(
        &self,
        path: &str,
        fields: &[(&str, &str)],
    ) -> Result<FormResponse, Error> {
        let mut operation = self.form(Method::POST, path)?;
        operation.form(fields);
        self.send_form(operation).await
    }

    pub async fn patch_form(
        &self,
        path: &str,
        fields: &[(&str, &str)],
    ) -> Result<FormResponse, Error> {
        let mut operation = self.form(Method::PATCH, path)?;
        operation.form(fields);
        self.send_form(operation).await
    }

    /// Deletes through the form endpoint, which answers a redirect. The request carries no
    /// body, and so no content type either.
    pub async fn delete_form(&self, path: &str) -> Result<FormResponse, Error> {
        self.send_form(self.form(Method::DELETE, path)?).await
    }

    /// Posts a multipart body the caller assembled, for the endpoints that take a file.
    pub async fn post_multipart(
        &self,
        path: &str,
        content_type: String,
        body: Bytes,
    ) -> Result<FormResponse, Error> {
        let mut operation = self.form(Method::POST, path)?;
        operation.multipart(content_type, body);
        self.send_form(operation).await
    }

    /// An operation for a path the model does not cover. An absolute URL is sent as it
    /// stands, provided the credentials may travel to it; anything else is resolved
    /// against the base URL.
    pub(crate) fn raw(&self, method: Method, path: &str) -> Result<Operation, Error> {
        let mut operation = match self.absolute(path)? {
            Some(url) => Operation::at(method, url),
            None => Operation::raw(method, path.to_string()),
        };
        operation.without_json_suffix();
        Ok(operation)
    }

    /// The URL a path names when it already is one. HTTPS goes anywhere; plain HTTP only
    /// back to the base URL's own host, which is how a HEY running on this machine is
    /// reached, and nowhere else — the credentials would go with it.
    fn absolute(&self, path: &str) -> Result<Option<Url>, Error> {
        if path.starts_with("https://") || path.starts_with("http://") {
            let url = Url::parse(path)?;
            if url.scheme() == "https" || is_same_origin(&url, self.base_url()) {
                Ok(Some(url))
            } else {
                Err(Error::usage(format!("URL must use HTTPS, got: {path}")))
            }
        } else {
            Ok(None)
        }
    }

    /// A blob is read from the HEY origin and nowhere else: the request carries the
    /// credentials, and only a redirect — which the HTTP client strips them before
    /// following — may lead off it. The response cache is for JSON documents, so a blob
    /// goes past it.
    fn blob(&self, path: &str) -> Result<Operation, Error> {
        let mut operation = self.raw(Method::GET, path)?;
        if let Some(url) = &operation.url
            && !is_same_origin(url, self.base_url())
        {
            return Err(Error::usage("a blob URL must start on the HEY origin"));
        }
        operation.accept("*/*").no_cache();
        Ok(operation)
    }

    async fn send_json(
        &self,
        method: Method,
        path: &str,
        body: &impl Serialize,
        accept: &'static str,
    ) -> Result<Response, Error> {
        let mut operation = self.raw(method, path)?;
        operation.json(body)?.accept(accept);
        self.execute(operation).await
    }
}
