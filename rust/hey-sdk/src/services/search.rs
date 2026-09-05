//! Searching mail.
//!
//! HEY has no JSON search endpoint — `/search` and `/advanced_search` render HTML — so
//! results are read off the advanced search page. Only the refine options are JSON, through
//! [`Search::get_advanced_filters`].

use crate::error::Error;
use crate::generated::types::AdvancedSearchResult;

pub use crate::generated::services::search::*;

/// An advanced search. `query` is the free-text part; the rest map onto the `refine[...]`
/// parameters the advanced search form submits. An empty refinement is left off the wire.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct SearchParams {
    /// The words to search for.
    pub query: String,
    /// The 1-based results page. Zero and one both ask for the first.
    pub page: u32,
    /// Words that must all appear.
    pub required: String,
    /// Words of which at least one must appear.
    pub any: String,
    /// Words that must not appear.
    pub none: String,
    /// Text that must appear verbatim.
    pub exact_phrase: String,
    /// Narrows by sender.
    pub from: String,
    /// Narrows by recipient.
    pub to: String,
    /// Narrows by subject line.
    pub subject: String,
    /// `last_7_days`, `last_30_days`, `last_90_days` or a four-digit year.
    pub date: String,
    /// Narrows to a box: `imbox`, `feed`, `papertrail` or `trash`.
    pub r#in: String,
    /// Narrows to a folder name.
    pub label: String,
    /// Narrows by attachment kind, or `any`.
    pub attachment: String,
}

/// One page of matches and the number of the page after it.
///
/// Search numbers its pages rather than cursoring them, so the next page is read by passing
/// that number back as [`SearchParams::page`]. It is `None` on the last page: HEY only sends
/// the `Link` header while there is more to read, which is how a caller walking the results
/// is told to stop asking rather than having to ask for a page that turns out empty.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct SearchResults {
    pub result: AdvancedSearchResult,
    pub next_page: Option<u32>,
}

impl<'a> Search<'a> {
    /// Runs an advanced search and answers the matching threads, grouped by topic as the
    /// search page shows them: the topic, your posting of it, and the entries that matched
    /// as summaries — read a message with
    /// [`Messages::get`](crate::services::messages::Messages::get).
    pub async fn search(&self, params: &SearchParams) -> Result<AdvancedSearchResult, Error> {
        Ok(self.search_page(params).await?.result)
    }

    /// Runs the same search as [`Search::search`] and also answers which page comes next.
    pub async fn search_page(&self, params: &SearchParams) -> Result<SearchResults, Error> {
        let page = self.advanced(&refinements(params)).await?;
        let next_page = page.next_page().and_then(|page| page.parse().ok());
        Ok(SearchResults {
            result: page.into_inner(),
            next_page,
        })
    }
}

fn refinements(params: &SearchParams) -> AdvancedSearchParams {
    let mut advanced = AdvancedSearchParams {
        q: optional(&params.query),
        page: None,
        refine_from: optional(&params.from),
        refine_to: optional(&params.to),
        refine_subject: optional(&params.subject),
        refine_exact_phrase: optional(&params.exact_phrase),
        refine_required: optional(&params.required),
        refine_any: optional(&params.any),
        refine_none: optional(&params.none),
        refine_date: optional(&params.date),
        refine_in: optional(&params.r#in),
        refine_label: optional(&params.label),
        refine_attachment: optional(&params.attachment),
    };
    if params.page > 1 {
        advanced.page = Some(params.page.to_string());
    }
    advanced
}

fn optional(value: &str) -> Option<String> {
    if value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}
