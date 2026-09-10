use std::collections::HashSet;

use hey_sdk::routes::{self, ROUTES};
use hey_sdk::url::router;

#[test]
fn filling_a_route_substitutes_and_encodes_its_parameters() {
    assert_eq!(routes::GET_BOX.fill(&[&123]), "/boxes/123");
    assert_eq!(
        routes::GET_BOX_GROUP.fill(&[&24090, &9]),
        "/boxes/24090/groups/9"
    );
    assert_eq!(
        routes::GET_JOURNAL_ENTRY.fill(&[&"2026-03-04"]),
        "/calendar/days/2026-03-04/journal_entry"
    );
    assert_eq!(
        routes::GET_JOURNAL_ENTRY.fill(&[&"a day off"]),
        "/calendar/days/a%20day%20off/journal_entry"
    );
}

#[test]
fn the_router_names_the_operation_a_pasted_url_refers_to() {
    let matched = router()
        .recognize("https://app.hey.com/boxes/24090/groups/9")
        .unwrap();

    assert_eq!(matched.operation(), "GetBoxGroup");
    assert_eq!(matched.resource, "Boxes");
    assert_eq!(matched.pattern, "/boxes/{boxId}/groups/{groupId}");
    assert_eq!(
        matched.params,
        [("boxId", "24090".to_string()), ("groupId", "9".to_string())]
    );
    assert_eq!(matched.resource_id(), Some("9"));
}

#[test]
fn every_modelled_route_is_listed_once() {
    let ids: HashSet<&str> = ROUTES.iter().map(|route| route.id).collect();

    assert_eq!(ids.len(), ROUTES.len());
    match modelled_operations() {
        Some(modelled) => assert_eq!(ROUTES.len(), modelled),
        None => eprintln!(
            "openapi.json is not in the package; the route count is checked in the repository"
        ),
    }
}

/// How many operations `openapi.json` models: one per HTTP method under each path. `None`
/// from the published crate, which carries the tests but not the model they were generated
/// from; in the repository the file is always there.
fn modelled_operations() -> Option<usize> {
    let path = concat!(env!("CARGO_MANIFEST_DIR"), "/../../openapi.json");
    let openapi: serde_json::Value =
        serde_json::from_str(&std::fs::read_to_string(path).ok()?).unwrap();
    Some(
        openapi["paths"]
            .as_object()
            .unwrap()
            .values()
            .flat_map(|item| item.as_object().unwrap().keys())
            .filter(|method| matches!(method.as_str(), "get" | "post" | "put" | "patch" | "delete"))
            .count(),
    )
}
