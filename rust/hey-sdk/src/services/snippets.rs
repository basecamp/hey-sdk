//! Keeping snippets — the reusable bits of text the composer offers — on top of the
//! generated snippet route.
//!
//! Snippets have no JSON surface for writes: every one of them redirects, so they are
//! browser form posts.

use reqwest::Method;

use crate::error::Error;
use crate::services::write_info;

pub use crate::generated::services::snippets::*;

impl<'a> Snippets<'a> {
    /// Saves a snippet.
    pub async fn create(&self, name: &str, content: &str) -> Result<(), Error> {
        let mut operation = self.client().form(Method::POST, "/snippets")?;
        operation.info(write_info("Snippets", "CreateSnippet", "snippet", None));
        operation.form(&snippet_fields(name, content));
        self.client().send_unit(operation).await
    }

    /// Edits a snippet. An empty field is left out of the form, and so left as it was.
    pub async fn update(&self, snippet_id: i64, name: &str, content: &str) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::PATCH, &format!("/snippets/{snippet_id}"))?;
        operation.info(write_info(
            "Snippets",
            "UpdateSnippet",
            "snippet",
            Some(snippet_id),
        ));
        operation.form(&snippet_fields(name, content));
        self.client().send_unit(operation).await
    }

    /// Throws a snippet away.
    pub async fn delete(&self, snippet_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::DELETE, &format!("/snippets/{snippet_id}"))?;
        operation.info(write_info(
            "Snippets",
            "DeleteSnippet",
            "snippet",
            Some(snippet_id),
        ));
        self.client().send_unit(operation).await
    }
}

fn snippet_fields<'a>(name: &'a str, content: &'a str) -> Vec<(&'a str, &'a str)> {
    let mut fields = Vec::new();
    if !name.is_empty() {
        fields.push(("snippet[name]", name));
    }
    if !content.is_empty() {
        fields.push(("snippet[content]", content));
    }
    fields
}
