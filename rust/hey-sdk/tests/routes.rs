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

    assert_eq!(ROUTES.len(), 130);
    assert_eq!(ids.len(), ROUTES.len());
}
