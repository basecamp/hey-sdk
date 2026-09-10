//! Small models put through the generator whole, so a wrong answer is caught here rather
//! than in the checked-in output. The drift check only proves `src/generated` matches what
//! the generator emits; these prove the generator emits the right thing.

use std::collections::BTreeMap;
use std::path::PathBuf;

use serde_json::{Value, json};

use crate::model::{Model, ResourceTypes};
use crate::naming::Naming;

const NAMES: &str =
    "[type_names]\nBox = \"Mailbox\"\n\n[resource_types]\nboxes = \"box\"\nstages = \"stage\"\n";

fn behavior(operations: &[&str]) -> Value {
    let mut entries = serde_json::Map::new();
    for id in operations {
        entries.insert(
            (*id).to_string(),
            json!({ "readonly": true, "retry": { "max": 3, "base_delay_ms": 1000, "retry_on": [429, 503] } }),
        );
    }
    json!({ "operations": entries })
}

fn openapi(paths: Value, schemas: Value) -> Value {
    json!({
        "openapi": "3.1.0",
        "info": { "version": "2026-01-01" },
        "paths": paths,
        "components": { "schemas": schemas },
    })
}

fn read(id: &str, tag: &str, path_params: &[&str], response: Value) -> Value {
    let parameters: Vec<Value> = path_params
        .iter()
        .map(|name| json!({ "name": name, "in": "path", "required": true, "schema": { "type": "integer", "format": "int64" } }))
        .collect();
    json!({ "get": {
        "operationId": id,
        "tags": [tag],
        "parameters": parameters,
        "responses": { "200": response },
    } })
}

fn json_body(schema: &str) -> Value {
    json!({ "content": { "application/json": { "schema": { "$ref": format!("#/components/schemas/{schema}") } } } })
}

fn build(paths: Value, schemas: Value, operations: &[&str]) -> Result<Model, String> {
    let naming = Naming::parse(NAMES).unwrap();
    let resource_types = ResourceTypes::parse(NAMES).unwrap();
    Model::build(
        &openapi(paths, schemas),
        &behavior(operations),
        &naming,
        &resource_types,
    )
}

fn generate(paths: Value, schemas: Value, operations: &[&str]) -> BTreeMap<PathBuf, String> {
    crate::render(&build(paths, schemas, operations).unwrap())
}

fn file<'a>(files: &'a BTreeMap<PathBuf, String>, name: &str) -> &'a str {
    &files[&PathBuf::from(name)]
}

/// The emitted `Route` static for one operation, so an assertion reads that route's own
/// fields and not another's.
fn route<'a>(routes: &'a str, constant: &str) -> &'a str {
    let start = routes
        .find(&format!("pub static {constant}: Route = Route {{"))
        .unwrap_or_else(|| panic!("{constant} is not in routes.rs"));
    let end = routes[start..].find("};\n").unwrap() + start;
    &routes[start..end]
}

fn box_schema() -> Value {
    json!({ "Box": { "type": "object", "properties": { "id": { "type": "integer", "format": "int64" } }, "required": ["id"] } })
}

#[test]
fn an_unsupported_representation_fails_generation() {
    let response = json!({ "content": { "image/png": { "schema": { "type": "string", "contentEncoding": "byte" } } } });
    let error = build(
        json!({ "/boxes/{id}/badge": read("GetBoxBadge", "Boxes", &["id"], response) }),
        box_schema(),
        &["GetBoxBadge"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "GetBoxBadge answers 200 as image/png, which the generator has no representation for; it emits application/json and text/html"
    );
}

#[test]
fn more_than_one_representation_fails_generation() {
    let response = json!({ "content": {
        "application/json": { "schema": { "$ref": "#/components/schemas/Box" } },
        "text/html": { "schema": { "$ref": "#/components/schemas/Page" } },
    } });
    let error = build(
        json!({ "/boxes/{id}": read("GetBox", "Boxes", &["id"], response) }),
        json!({ "Box": box_schema()["Box"], "Page": { "type": "string" } }),
        &["GetBox"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "GetBox answers 200 in 2 representations (application/json, text/html); the generator emits one"
    );
}

#[test]
fn an_inline_response_schema_fails_generation() {
    let response = json!({ "content": { "application/json": { "schema": { "type": "object" } } } });
    let error = build(
        json!({ "/boxes/{id}": read("GetBox", "Boxes", &["id"], response) }),
        box_schema(),
        &["GetBox"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "GetBox answers 200 as application/json with a schema that is not a $ref to components.schemas"
    );
}

#[test]
fn an_html_response_has_to_be_a_string_schema() {
    let response =
        json!({ "content": { "text/html": { "schema": { "$ref": "#/components/schemas/Box" } } } });
    let error = build(
        json!({ "/boxes/{id}": read("GetBox", "Boxes", &["id"], response) }),
        box_schema(),
        &["GetBox"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "GetBox answers text/html as Mailbox, which is not a string schema; an HTML page is handed back as a String"
    );
}

#[test]
fn a_second_success_status_answered_differently_fails_generation() {
    let mut operation = read("GetBox", "Boxes", &["id"], json_body("Box"));
    operation["get"]["responses"]["202"] =
        json!({ "content": { "image/png": { "schema": { "$ref": "#/components/schemas/Box" } } } });
    let error = build(
        json!({ "/boxes/{id}": operation }),
        box_schema(),
        &["GetBox"],
    )
    .err()
    .unwrap();
    assert_eq!(
        error,
        "GetBox answers 202 as image/png, which the generator has no representation for; it emits application/json and text/html"
    );

    let mut operation = read("GetBox", "Boxes", &["id"], json_body("Box"));
    operation["get"]["responses"]["204"] = json!({ "description": "nothing" });
    let error = build(
        json!({ "/boxes/{id}": operation }),
        box_schema(),
        &["GetBox"],
    )
    .err()
    .unwrap();
    assert_eq!(
        error,
        "GetBox answers 200 and 204 differently (Mailbox as JSON and no body); the generator emits one representation per operation"
    );
}

#[test]
fn a_referenced_response_fails_generation() {
    let response = json!({ "$ref": "#/components/responses/Stage" });
    let error = build(
        json!({ "/stages/{id}": read("GetStage", "Stages", &["id"], response) }),
        box_schema(),
        &["GetStage"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "GetStage answers 200 through a $ref response, which the generator does not resolve; write the response inline"
    );
}

#[test]
fn a_reference_outside_the_documents_schemas_fails_generation() {
    let response = json!({ "content": { "application/json": { "schema": { "$ref": "types.json#/components/schemas/Box" } } } });
    let error = build(
        json!({ "/boxes/{id}": read("GetBox", "Boxes", &["id"], response) }),
        box_schema(),
        &["GetBox"],
    )
    .err()
    .unwrap();
    assert_eq!(
        error,
        "$ref types.json#/components/schemas/Box does not point into #/components/schemas/; the generator resolves nothing else"
    );

    let error = build(
        json!({ "/boxes/{id}": read("GetBox", "Boxes", &["id"], json_body("Crate")) }),
        box_schema(),
        &["GetBox"],
    )
    .err()
    .unwrap();
    assert_eq!(
        error,
        "GetBox refers to Crate, which is not in components.schemas"
    );

    let error = build(
        json!({}),
        json!({ "Box": { "type": "object", "properties": { "owner": { "$ref": "#/components/schemas/Owner" } } } }),
        &[],
    )
    .err()
    .unwrap();
    assert_eq!(
        error,
        "Mailbox refers to Owner, which is not in components.schemas"
    );
}

#[test]
fn an_html_response_may_reach_its_string_through_an_alias() {
    let response = json!({ "content": { "text/html": { "schema": { "$ref": "#/components/schemas/StagePage" } } } });
    let files = generate(
        json!({ "/stages/{id}": read("GetStage", "Stages", &["id"], response) }),
        json!({
            "StagePage": { "$ref": "#/components/schemas/PageText" },
            "PageText": { "type": "string" },
        }),
        &["GetStage"],
    );

    assert!(file(&files, "types.rs").contains("pub type StagePage = PageText;\n"));
    assert!(file(&files, "services/stages.rs").contains("self.client.send_text(operation).await"));
}

#[test]
fn an_operation_the_generator_does_not_emit_fails_generation() {
    let mut item = read("GetBox", "Boxes", &["id"], json_body("Box"));
    item["head"] = item["get"].clone();
    item["head"]["operationId"] = json!("HeadBox");
    let error = build(
        json!({ "/boxes/{id}": item }),
        box_schema(),
        &["GetBox", "HeadBox"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "/boxes/{id} has a head operation, which the generator does not emit"
    );
}

#[test]
fn an_html_page_is_read_as_text_from_the_path_as_written() {
    let response = json!({ "content": { "text/html": { "schema": { "$ref": "#/components/schemas/StagePage" } } } });
    let files = generate(
        json!({ "/stages/{id}": read("GetStage", "Stages", &["id"], response) }),
        json!({ "StagePage": { "type": "string", "contentEncoding": "byte" } }),
        &["GetStage"],
    );

    let service = file(&files, "services/stages.rs");
    assert!(service.contains("pub async fn get(&self, id: i64) -> Result<StagePage, Error> {"));
    assert!(service.contains("self.client.send_text(operation).await"));
    let stage = route(file(&files, "routes.rs"), "GET_STAGE");
    assert!(stage.contains("    path: \"/stages/{id}\",\n"));
    assert!(stage.contains("    html: true,\n"));
    assert!(file(&files, "types.rs").contains("pub type StagePage = String;\n"));
}

#[test]
fn json_and_empty_responses_take_their_own_send() {
    let files = generate(
        json!({
            "/boxes/{id}.json": read("GetBox", "Boxes", &["id"], json_body("Box")),
            "/boxes/{id}/seen.json": read("GetBoxSeen", "Boxes", &["id"], json!({ "description": "nothing" })),
        }),
        box_schema(),
        &["GetBox", "GetBoxSeen"],
    );

    let service = file(&files, "services/boxes.rs");
    assert!(service.contains(
        "    pub async fn get(&self, id: i64) -> Result<Mailbox, Error> {\n        let mut operation = self.client.operation(&routes::GET_BOX, &[&id]);\n        operation.resource_id(id);\n        self.client.send(operation).await\n    }\n"
    ));
    assert!(service.contains(
        "    pub async fn get_seen(&self, id: i64) -> Result<(), Error> {\n        let mut operation = self.client.operation(&routes::GET_BOX_SEEN, &[&id]);\n        operation.resource_id(id);\n        self.client.send_unit(operation).await\n    }\n"
    ));
    let routes = file(&files, "routes.rs");
    assert!(route(routes, "GET_BOX").contains("    html: false,\n"));
    assert!(route(routes, "GET_BOX_SEEN").contains("    html: false,\n"));
}

#[test]
fn two_operations_collapsing_to_one_method_fail_generation() {
    let error = build(
        json!({
            "/boxes/{id}.json": read("GetBox", "Boxes", &["id"], json_body("Box")),
            "/boxes/{id}/box.json": read("GetBoxBox", "Boxes", &["id"], json_body("Box")),
        }),
        box_schema(),
        &["GetBox", "GetBoxBox"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "GetBox and GetBoxBox both become boxes::get; add an [operation_methods] override to names.toml"
    );
}

#[test]
fn a_method_named_after_a_keyword_fails_generation() {
    let error = build(
        json!({ "/boxes/{id}/match.json": read("MatchBox", "Boxes", &["id"], json_body("Box")) }),
        box_schema(),
        &["MatchBox"],
    )
    .err()
    .unwrap();

    assert_eq!(
        error,
        "MatchBox becomes `match` in boxes; add an [operation_methods] override to names.toml"
    );
}

#[test]
fn required_and_optional_fields_read_as_the_model_says() {
    let files = generate(
        json!({}),
        json!({ "Sticky": {
            "type": "object",
            "description": "A sticky note",
            "properties": {
                "id": { "type": "integer", "format": "int64" },
                "created_at": { "type": "string", "format": "date-time" },
                "due_on": { "type": "string", "format": "date" },
                "note": { "type": "string", "description": "What it says" },
                "box": { "$ref": "#/components/schemas/Box" },
                "type": { "type": "string" },
            },
            "required": ["id", "created_at", "box"],
        }, "Box": box_schema()["Box"] }),
        &[],
    );

    let expected = "/// A sticky note
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct Sticky {
    #[serde(default, deserialize_with = \"crate::types::null_as_default::deserialize\")]
    pub id: i64,
    pub created_at: DateTime,
    #[serde(default, with = \"crate::types::optional_date\", skip_serializing_if = \"Option::is_none\")]
    pub due_on: Option<Date>,
    /// What it says
    #[serde(default, skip_serializing_if = \"Option::is_none\")]
    pub note: Option<String>,
    #[serde(default)]
    pub r#box: Mailbox,
    #[serde(default, skip_serializing_if = \"Option::is_none\")]
    pub r#type: Option<String>,
}

";
    assert!(
        file(&files, "types.rs").contains(expected),
        "{}",
        file(&files, "types.rs")
    );
}

#[test]
fn a_shape_that_mentions_itself_is_boxed_and_a_renamed_one_reaches_every_field() {
    let files = generate(
        json!({}),
        json!({
            "Box": { "type": "object", "properties": { "parent": { "$ref": "#/components/schemas/Box" }, "children": { "type": "array", "items": { "$ref": "#/components/schemas/Box" } } } },
            "Folder": { "type": "object", "properties": { "box": { "$ref": "#/components/schemas/Box" }, "boxes_by_name": { "type": "object", "additionalProperties": { "$ref": "#/components/schemas/Box" } } } },
            "Boxes": { "type": "array", "items": { "$ref": "#/components/schemas/Box" } },
        }),
        &[],
    );

    let types = file(&files, "types.rs");
    assert!(types.contains("pub struct Mailbox {"));
    assert!(types.contains("    pub parent: Option<::std::boxed::Box<Mailbox>>,"));
    assert!(types.contains("    pub children: Option<Vec<Mailbox>>,"));
    assert!(types.contains("    pub r#box: Option<Mailbox>,"));
    assert!(types.contains("    pub boxes_by_name: Option<BTreeMap<String, Mailbox>>,"));
    assert!(types.contains("pub type Boxes = Vec<Mailbox>;"));
    assert!(!types.contains("struct Box "));
}
