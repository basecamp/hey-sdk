use std::collections::BTreeMap;
use std::sync::OnceLock;

use reqwest::Method;
use url::Url;

use crate::generated::routes::ROUTES;
use crate::route::Route;

/// Recognizes HEY paths and URLs, as pasted from the web app, and names the operation,
/// resource and ids they refer to. Works offline.
pub struct Router {
    patterns: Vec<Pattern>,
}

struct Pattern {
    pattern: &'static str,
    routes: Vec<&'static Route>,
}

/// What a recognized path refers to.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Match {
    pub pattern: &'static str,
    pub resource: &'static str,
    /// The operations served on this path, by method.
    pub operations: BTreeMap<String, &'static str>,
    /// The path parameters in the order they appear.
    pub params: Vec<(&'static str, String)>,
}

impl Match {
    /// The read on this path, or the alphabetically first operation when there is none.
    pub fn operation(&self) -> &'static str {
        self.operations
            .get(Method::GET.as_str())
            .or_else(|| self.operations.values().min())
            .copied()
            .expect("a matched pattern serves at least one operation")
    }

    /// The last path parameter: the id of the record the path names, if any.
    pub fn resource_id(&self) -> Option<&str> {
        self.params.last().map(|(_, value)| value.as_str())
    }
}

impl Router {
    /// A router over every modelled route.
    pub fn new() -> Router {
        Router::over(ROUTES)
    }

    /// A router over a chosen set of routes, for a caller that recognizes only part of HEY
    /// — one service's paths, say.
    pub fn over(routes: &[&'static Route]) -> Router {
        let mut by_pattern: BTreeMap<&'static str, Vec<&'static Route>> = BTreeMap::new();
        for route in routes {
            by_pattern.entry(route.pattern).or_default().push(route);
        }
        let mut patterns: Vec<Pattern> = by_pattern
            .into_iter()
            .map(|(pattern, routes)| Pattern { pattern, routes })
            .collect();
        patterns.sort_by(|a, b| {
            let depth = |pattern: &str| pattern.matches('/').count();
            depth(b.pattern)
                .cmp(&depth(a.pattern))
                .then_with(|| a.pattern.cmp(b.pattern))
        });
        Router { patterns }
    }

    /// Recognizes a path such as `/topics/456` or a full URL such as
    /// `https://app.hey.com/topics/456.json?foo=bar`. Trailing slashes, a `.json` suffix,
    /// the query and the fragment are ignored.
    pub fn recognize(&self, path_or_url: &str) -> Option<Match> {
        let path = match Url::parse(path_or_url) {
            Ok(url) if !url.cannot_be_a_base() => url.path().to_string(),
            _ => path_or_url
                .split(['?', '#'])
                .next()
                .unwrap_or_default()
                .to_string(),
        };
        let path = path.trim_end_matches('/');
        let path = path.strip_suffix(".json").unwrap_or(path);
        self.patterns
            .iter()
            .find_map(|pattern| pattern.recognize(path))
    }
}

impl Default for Router {
    fn default() -> Router {
        Router::new()
    }
}

impl Pattern {
    fn recognize(&self, path: &str) -> Option<Match> {
        let params = self.routes[0].recognize(path)?;
        let operations = self
            .routes
            .iter()
            .map(|route| (route.method.to_string(), route.id))
            .collect();
        Some(Match {
            pattern: self.pattern,
            resource: self.routes[0].resource,
            operations,
            params,
        })
    }
}

/// The process-wide router.
pub fn router() -> &'static Router {
    static ROUTER: OnceLock<Router> = OnceLock::new();
    ROUTER.get_or_init(Router::new)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn recognizes_a_pasted_topic_url() {
        let matched = router()
            .recognize("https://app.hey.com/topics/456?x=1")
            .unwrap();
        assert_eq!(matched.operation(), "GetTopic");
        assert_eq!(matched.resource, "Topics");
        assert_eq!(matched.resource_id(), Some("456"));
    }

    #[test]
    fn deeper_patterns_win_and_methods_are_listed() {
        let matched = router().recognize("/boxes/24090/groups/9.json").unwrap();
        assert_eq!(matched.pattern, "/boxes/{boxId}/groups/{groupId}");
        assert_eq!(matched.operation(), "GetBoxGroup");
        assert_eq!(matched.operations["DELETE"], "DeleteBoxGroup");
        assert_eq!(
            matched.params,
            vec![("boxId", "24090".to_string()), ("groupId", "9".to_string())]
        );
    }

    #[test]
    fn a_router_over_a_chosen_set_recognizes_only_those() {
        let router = Router::over(&[&crate::generated::routes::GET_TOPIC]);

        assert_eq!(
            router.recognize("/topics/456").unwrap().operation(),
            "GetTopic"
        );
        assert!(router.recognize("/imbox").is_none());
    }

    #[test]
    fn prefers_the_read_and_falls_back_alphabetically() {
        assert_eq!(
            router().recognize("/postings/seen").unwrap().operation(),
            "MarkPostingsSeen"
        );
        assert_eq!(
            router().recognize("/postings/mutings").unwrap().operation(),
            "MutePostings"
        );
        assert_eq!(
            router().recognize("/imbox/").unwrap().operation(),
            "GetImbox"
        );
        assert!(router().recognize("/nothing/here").is_none());
        assert!(router().recognize("/boxes//groups").is_none());
    }
}
