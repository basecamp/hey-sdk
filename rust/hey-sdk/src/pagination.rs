use std::ops::Deref;

use reqwest::Method;
use serde_json::Value;
use url::Url;

use crate::client::{Client, Response};
use crate::error::Error;
use crate::observability::OperationInfo;
use crate::operation::Operation;
use crate::security::is_same_origin;

/// One page of a paginated read, with the cursor HEY handed out for the next one.
///
/// The page derefs to its value, so `page.postings` reads the same as it would on the
/// response itself.
#[derive(Debug, Clone)]
pub struct Page<T> {
    value: T,
    next_url: Option<Url>,
    next_page: Option<String>,
    total_count: Option<u64>,
    /// What the read that produced this page announced itself as, so the reads that walk
    /// on from it can say the same.
    info: OperationInfo,
}

impl<T> Page<T> {
    pub(crate) fn new(value: T, response: &Response, info: OperationInfo) -> Page<T> {
        let next_url = response
            .headers
            .get("link")
            .and_then(|value| value.to_str().ok())
            .and_then(next_link)
            .and_then(|target| response.url.join(&target).ok());
        let next_page = next_url.as_ref().and_then(|url| {
            url.query_pairs()
                .find(|(name, _)| name == "page")
                .map(|(_, value)| value.into_owned())
        });
        let total_count = response
            .headers
            .get("x-total-count")
            .and_then(|value| value.to_str().ok())
            .and_then(|value| value.trim().parse().ok());
        Page {
            value,
            next_url,
            next_page,
            total_count,
            info,
        }
    }

    pub(crate) fn info(&self) -> &OperationInfo {
        &self.info
    }

    pub fn into_inner(self) -> T {
        self.value
    }

    pub fn value(&self) -> &T {
        &self.value
    }

    /// The opaque cursor for the page after this one, to pass as `page` on the same read.
    pub fn next_page(&self) -> Option<&str> {
        self.next_page.as_deref()
    }

    /// The URL of the page after this one, as HEY's `Link` header named it.
    pub fn next_url(&self) -> Option<&Url> {
        self.next_url.as_ref()
    }

    pub fn has_next(&self) -> bool {
        self.next_url.is_some()
    }

    /// The `X-Total-Count` header, when the read carried one.
    pub fn total_count(&self) -> Option<u64> {
        self.total_count
    }

    pub fn map<U>(self, f: impl FnOnce(T) -> U) -> Page<U> {
        Page {
            value: f(self.value),
            next_url: self.next_url,
            next_page: self.next_page,
            total_count: self.total_count,
            info: self.info,
        }
    }
}

impl<T> Deref for Page<T> {
    type Target = T;

    fn deref(&self) -> &T {
        &self.value
    }
}

impl Client {
    /// Reads a paginated path to its end and hands back the items of every page as one
    /// list. Each page has to decode as a JSON array. Use this for the paths the model
    /// does not cover; a modelled read walks with [`Client::each_page`], which keeps the
    /// records typed.
    pub async fn get_all(&self, path: &str) -> Result<Vec<Value>, Error> {
        self.get_all_with_limit(path, 0).await
    }

    /// Reads a paginated path until `limit` items are in hand, or to its end when `limit`
    /// is zero. The last page is trimmed to land on exactly `limit`.
    pub async fn get_all_with_limit(&self, path: &str, limit: usize) -> Result<Vec<Value>, Error> {
        let mut operation = self.raw(Method::GET, path)?;
        let started_at = self.url_for(&operation)?;
        let mut collected: Vec<Value> = Vec::new();
        let mut pages = 0;

        loop {
            let response = self.execute(operation).await?;
            collected.extend(response.json::<Vec<Value>>()?);
            pages += 1;

            if limit > 0 && collected.len() >= limit {
                collected.truncate(limit);
                break;
            }
            match self.next_page_url(&response, &started_at)? {
                Some(next) if pages < self.max_pages() => {
                    operation = Operation::at(Method::GET, next);
                }
                Some(_) => {
                    tracing::warn!(max_pages = self.max_pages(), "pagination capped");
                    break;
                }
                None => break,
            }
        }
        Ok(collected)
    }

    /// Reads the pages after one already in hand, and hands back their items. Say how many
    /// the first page held as `first_page_count`, so a `limit` counts the whole walk;
    /// `limit` of zero reads to the end.
    pub async fn follow_pagination(
        &self,
        first: &Response,
        first_page_count: usize,
        limit: usize,
    ) -> Result<Vec<Value>, Error> {
        if limit > 0 && first_page_count >= limit {
            return Ok(Vec::new());
        }

        let started_at = first.url.clone();
        let mut next = self.next_page_url(first, &started_at)?;
        let mut collected: Vec<Value> = Vec::new();
        let mut count = first_page_count;
        let mut pages = 1;

        while let Some(url) = next {
            let response = self.execute(Operation::at(Method::GET, url)).await?;
            let items: Vec<Value> = response.json()?;
            count += items.len();
            collected.extend(items);
            pages += 1;

            if limit > 0 && count >= limit {
                collected.truncate(collected.len().saturating_sub(count - limit));
                break;
            }
            next = match self.next_page_url(&response, &started_at)? {
                Some(url) if pages < self.max_pages() => Some(url),
                Some(_) => {
                    tracing::warn!(max_pages = self.max_pages(), "pagination capped");
                    None
                }
                None => None,
            };
        }
        Ok(collected)
    }

    /// The page after this one, as the `Link` header named it, resolved against the answer
    /// it came in. A target off the origin the walk started on is refused rather than
    /// followed: the header is the server's to write, and following it would carry the
    /// credentials somewhere they were never meant to go.
    fn next_page_url(&self, response: &Response, started_at: &Url) -> Result<Option<Url>, Error> {
        match response.header("link").and_then(next_link) {
            None => Ok(None),
            Some(target) => {
                let next = response.url.join(&target)?;
                if is_same_origin(&next, started_at) {
                    Ok(Some(next))
                } else {
                    Err(Error::usage(format!(
                        "pagination Link header points to a different origin: {next}"
                    )))
                }
            }
        }
    }
}

/// The target of the `rel="next"` link in an RFC 8288 `Link` header. Targets are read
/// between angle brackets, so commas inside a URL do not split it, and `rel` is a
/// space-separated set matched case-insensitively.
pub fn next_link(header: &str) -> Option<String> {
    let mut remaining = header;
    while let Some(start) = remaining.find('<') {
        let after_start = &remaining[start + 1..];
        let end = after_start.find('>')?;
        let target = &after_start[..end];
        let rest = &after_start[end + 1..];
        let params_end = rest.find('<').unwrap_or(rest.len());
        if link_is_next(&rest[..params_end]) {
            return Some(target.to_string());
        }
        remaining = &rest[params_end..];
    }
    None
}

fn link_is_next(params: &str) -> bool {
    params.split(';').any(|param| {
        let mut parts = param.splitn(2, '=');
        let name = parts.next().unwrap_or_default().trim();
        let value = parts.next().unwrap_or_default().trim().trim_matches('"');
        name.eq_ignore_ascii_case("rel")
            && value
                .split_whitespace()
                .any(|rel| rel.eq_ignore_ascii_case("next"))
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn finds_the_next_link_among_others() {
        let header = r#"<https://app.hey.com/imbox.json?page=a,b>; rel="prev", <https://app.hey.com/imbox.json?page=c>; rel="next""#;
        assert_eq!(
            next_link(header).as_deref(),
            Some("https://app.hey.com/imbox.json?page=c")
        );
    }

    #[test]
    fn matches_rel_sets_and_case() {
        assert_eq!(
            next_link(r#"</x?page=2>; REL="prev next""#).as_deref(),
            Some("/x?page=2")
        );
        assert_eq!(next_link(r#"</x?page=2>; rel="last""#), None);
        assert_eq!(next_link(""), None);
    }
}
