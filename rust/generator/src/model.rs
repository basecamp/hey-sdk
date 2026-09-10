use std::collections::{BTreeMap, BTreeSet};

use serde::Deserialize;
use serde_json::Value;

use crate::naming::{Naming, struct_name};

/// Which noun each operation acts on, read from the same `names.toml` the naming
/// overrides come from. Nothing in `openapi.json` says it, and the SDKs have to agree on
/// it, so it is written down per service and overridden per operation where the two
/// differ.
#[derive(Deserialize, Default)]
pub(crate) struct ResourceTypes {
    #[serde(default)]
    resource_types: BTreeMap<String, String>,
    #[serde(default)]
    operation_resource_types: BTreeMap<String, String>,
}

impl ResourceTypes {
    pub(crate) fn parse(source: &str) -> Result<ResourceTypes, String> {
        toml::from_str(source).map_err(|error| format!("names.toml: {error}"))
    }

    fn for_operation(&self, operation_id: &str, service: &str) -> Result<String, String> {
        if let Some(resource_type) = self.operation_resource_types.get(operation_id) {
            Ok(resource_type.clone())
        } else if let Some(resource_type) = self.resource_types.get(service) {
            Ok(resource_type.clone())
        } else {
            Err(format!(
                "{service} has no resource type; add one to the [resource_types] table in names.toml"
            ))
        }
    }
}

pub(crate) struct Model {
    pub api_version: String,
    pub schemas: Vec<Schema>,
    pub services: Vec<Service>,
}

pub(crate) struct Schema {
    pub name: String,
    pub description: Option<String>,
    pub shape: Shape,
}

pub(crate) enum Shape {
    Struct(Struct),
    Alias(FieldType),
}

pub(crate) struct Struct {
    pub fields: Vec<Field>,
    pub polymorphic: Option<Polymorphic>,
}

pub(crate) struct Polymorphic {
    pub discriminator: String,
    pub variants: Vec<String>,
}

pub(crate) struct Field {
    pub wire_name: String,
    pub description: Option<String>,
    pub kind: FieldType,
    pub required: bool,
    pub recursive: bool,
}

#[derive(Clone, PartialEq)]
pub(crate) enum FieldType {
    String,
    SensitiveString,
    DateTime,
    Date,
    Bool,
    Int32,
    Int64,
    Json,
    Named(String),
    List(Box<FieldType>),
    Map(Box<FieldType>),
}

pub(crate) struct Service {
    pub name: String,
    pub operations: Vec<Operation>,
}

pub(crate) struct Operation {
    pub id: String,
    /// The service handle that sends it, as every HEY SDK names it: `Boxes`, `TimeTracks`.
    pub service: String,
    pub method_name: String,
    pub description: Option<String>,
    pub http_method: String,
    pub path: String,
    pub resource: String,
    pub resource_type: String,
    pub path_params: Vec<PathParam>,
    pub query_params: Vec<QueryParam>,
    pub body: Option<String>,
    pub response: Response,
    pub idempotent: bool,
    pub readonly: bool,
    pub empty_on: Vec<u16>,
    pub pagination: Pagination,
    pub retry: Retry,
}

pub(crate) struct PathParam {
    pub wire_name: String,
    pub kind: ParamKind,
    pub role: ParamRole,
}

/// Where a path parameter sits: the last segment names the record itself, anything before
/// it names a parent.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum ParamRole {
    Parent,
    Recording,
}

pub(crate) struct QueryParam {
    pub wire_name: String,
    pub kind: ParamKind,
    pub required: bool,
}

#[derive(Clone, Copy, PartialEq)]
pub(crate) enum ParamKind {
    String,
    Bool,
    Int32,
    Int64,
}

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum Response {
    Empty,
    Json(String),
    /// A page HEY serves as HTML, with the name of the `String` alias the schema became.
    Html(String),
}

impl Response {
    fn describe(&self) -> String {
        match self {
            Response::Empty => "no body".to_string(),
            Response::Json(name) => format!("{name} as JSON"),
            Response::Html(name) => format!("{name} as HTML"),
        }
    }
}

#[derive(Clone, Copy, PartialEq)]
pub(crate) enum Pagination {
    None,
    Link,
    Window,
}

pub(crate) struct Retry {
    pub max: u32,
    pub base_delay_ms: u64,
    pub on: Vec<u16>,
}

impl Model {
    pub(crate) fn build(
        openapi: &Value,
        behavior: &Value,
        naming: &Naming,
        resource_types: &ResourceTypes,
    ) -> Result<Model, String> {
        let api_version = openapi["info"]["version"]
            .as_str()
            .ok_or("openapi.json has no info.version")?
            .to_string();
        let schemas = build_schemas(openapi, naming)?;
        let services = build_services(openapi, behavior, naming, resource_types)?;
        check_references(&schemas, &services)?;
        check_html_responses(&schemas, &services)?;
        Ok(Model {
            api_version,
            schemas,
            services,
        })
    }
}

/// A page is handed back as the `String` it arrived as, so the schema an HTML response
/// names has to be a string alias; a struct there would be a document the crate had no
/// parser for.
fn check_html_responses(schemas: &[Schema], services: &[Service]) -> Result<(), String> {
    for operation in services.iter().flat_map(|service| &service.operations) {
        if let Response::Html(name) = &operation.response
            && !resolves_to_string(schemas, name)
        {
            return Err(format!(
                "{} answers text/html as {name}, which is not a string schema; an HTML page is handed back as a String",
                operation.id
            ));
        }
    }
    Ok(())
}

/// Whether a schema is a string, following an alias of an alias as far as there are schemas
/// to follow it through.
fn resolves_to_string(schemas: &[Schema], name: &str) -> bool {
    let mut current = name.to_string();
    for _ in 0..=schemas.len() {
        match schemas
            .iter()
            .find(|schema| schema.name == current)
            .map(|schema| &schema.shape)
        {
            Some(Shape::Alias(FieldType::String)) => return true,
            Some(Shape::Alias(FieldType::Named(next))) => current = next.clone(),
            _ => return false,
        }
    }
    false
}

fn build_schemas(openapi: &Value, naming: &Naming) -> Result<Vec<Schema>, String> {
    let components = openapi["components"]["schemas"]
        .as_object()
        .ok_or("openapi.json has no components.schemas")?;
    let mut schemas = Vec::new();
    for (schema_name, schema) in components {
        let name = naming.type_for(schema_name);
        let shape = if let Some(properties) = schema.get("properties") {
            let required: BTreeSet<&str> = schema["required"]
                .as_array()
                .map(|list| list.iter().filter_map(Value::as_str).collect())
                .unwrap_or_default();
            let properties = properties
                .as_object()
                .ok_or(format!("{name}.properties is not an object"))?;
            let mut fields = Vec::new();
            for (wire_name, property) in properties {
                let kind = field_type(wire_name, property, naming)?;
                fields.push(Field {
                    wire_name: wire_name.clone(),
                    description: description_of(property),
                    recursive: kind.mentions(&name),
                    kind,
                    required: required.contains(wire_name.as_str()),
                });
            }
            Shape::Struct(Struct {
                fields,
                polymorphic: polymorphic_of(schema),
            })
        } else {
            Shape::Alias(field_type(schema_name, schema, naming)?)
        };
        schemas.push(Schema {
            name,
            description: description_of(schema),
            shape,
        });
    }
    Ok(schemas)
}

fn polymorphic_of(schema: &Value) -> Option<Polymorphic> {
    let extension = schema.get("x-hey-polymorphic")?;
    Some(Polymorphic {
        discriminator: extension["discriminator"].as_str()?.to_string(),
        variants: extension["variants"].as_object()?.keys().cloned().collect(),
    })
}

fn field_type(name: &str, property: &Value, naming: &Naming) -> Result<FieldType, String> {
    if let Some(reference) = property.get("$ref").and_then(Value::as_str) {
        return Ok(FieldType::Named(
            naming.type_for(&reference_name(reference)?),
        ));
    }
    let format = property["format"].as_str();
    match property["type"].as_str() {
        Some("string") => Ok(string_type(name, property, format)),
        Some("boolean") => Ok(FieldType::Bool),
        Some("integer") => match format {
            Some("int32") => Ok(FieldType::Int32),
            _ => Ok(FieldType::Int64),
        },
        Some("array") => Ok(FieldType::List(Box::new(field_type(
            name,
            &property["items"],
            naming,
        )?))),
        Some("object") => {
            if let Some(values) = property.get("additionalProperties") {
                Ok(FieldType::Map(Box::new(field_type(name, values, naming)?)))
            } else {
                Ok(FieldType::Json)
            }
        }
        other => Err(format!("{name}: unsupported schema type {other:?}")),
    }
}

fn string_type(name: &str, property: &Value, format: Option<&str>) -> FieldType {
    if property.get("x-hey-sensitive").is_some() {
        FieldType::SensitiveString
    } else if format == Some("date-time") || name.ends_with("_at") {
        FieldType::DateTime
    } else if format == Some("date") || name.ends_with("_on") {
        FieldType::Date
    } else {
        FieldType::String
    }
}

fn build_services(
    openapi: &Value,
    behavior: &Value,
    naming: &Naming,
    resource_types: &ResourceTypes,
) -> Result<Vec<Service>, String> {
    let paths = openapi["paths"]
        .as_object()
        .ok_or("openapi.json has no paths")?;
    let behaviors = behavior["operations"]
        .as_object()
        .ok_or("behavior-model.json has no operations")?;
    let mut services: BTreeMap<String, Vec<Operation>> = BTreeMap::new();

    for (path, item) in paths {
        for (http_method, operation) in
            item.as_object().ok_or(format!("{path} is not an object"))?
        {
            match http_method.as_str() {
                "get" | "post" | "put" | "patch" | "delete" => {}
                "head" | "options" | "trace" => {
                    return Err(format!(
                        "{path} has a {http_method} operation, which the generator does not emit"
                    ));
                }
                _ => continue,
            }
            let id = operation["operationId"]
                .as_str()
                .ok_or(format!("{http_method} {path} has no operationId"))?;
            let tag = operation["tags"][0]
                .as_str()
                .ok_or(format!("{id} has no tag"))?;
            let service = naming.service_for(id, tag);
            let semantics = behaviors
                .get(id)
                .ok_or(format!("{id} is missing from behavior-model.json"))?;
            let operation = Operation {
                id: id.to_string(),
                service: struct_name(&service),
                method_name: naming.method_for(id, &service)?,
                description: description_of(operation),
                http_method: http_method.to_uppercase(),
                path: path.clone(),
                resource: tag.to_string(),
                resource_type: resource_types.for_operation(id, &service)?,
                path_params: path_params(operation, path)?,
                query_params: query_params(operation)?,
                body: body_of(operation, naming)?,
                response: response_of(operation, naming)?,
                idempotent: idempotent(http_method, operation),
                readonly: readonly(semantics, id)?,
                empty_on: empty_on(operation),
                pagination: pagination(semantics)?,
                retry: retry(semantics),
            };
            services.entry(service).or_default().push(operation);
        }
    }

    let mut result = Vec::new();
    for (name, mut operations) in services {
        operations.sort_by(|a, b| a.method_name.cmp(&b.method_name).then(a.id.cmp(&b.id)));
        for pair in operations.windows(2) {
            if pair[0].method_name == pair[1].method_name {
                return Err(format!(
                    "{} and {} both become {}::{}; add an [operation_methods] override to names.toml",
                    pair[0].id, pair[1].id, name, pair[0].method_name
                ));
            }
        }
        result.push(Service { name, operations });
    }
    Ok(result)
}

fn path_params(operation: &Value, path: &str) -> Result<Vec<PathParam>, String> {
    let last_segment = path
        .trim_end_matches(".json")
        .rsplit('/')
        .next()
        .unwrap_or_default();
    let mut params = Vec::new();
    for parameter in parameters_in(operation, "path") {
        let wire_name = parameter["name"].as_str().unwrap().to_string();
        let role = if last_segment == format!("{{{wire_name}}}") {
            ParamRole::Recording
        } else {
            ParamRole::Parent
        };
        params.push(PathParam {
            wire_name,
            kind: param_kind(parameter)?,
            role,
        });
    }
    Ok(params)
}

fn query_params(operation: &Value) -> Result<Vec<QueryParam>, String> {
    let mut params = Vec::new();
    for parameter in parameters_in(operation, "query") {
        params.push(QueryParam {
            wire_name: parameter["name"].as_str().unwrap().to_string(),
            kind: param_kind(parameter)?,
            required: parameter["required"].as_bool().unwrap_or(false),
        });
    }
    Ok(params)
}

fn parameters_in<'a>(operation: &'a Value, location: &'a str) -> impl Iterator<Item = &'a Value> {
    operation["parameters"]
        .as_array()
        .into_iter()
        .flatten()
        .filter(move |parameter| parameter["in"].as_str() == Some(location))
}

fn param_kind(parameter: &Value) -> Result<ParamKind, String> {
    let schema = &parameter["schema"];
    match (schema["type"].as_str(), schema["format"].as_str()) {
        (Some("string"), _) => Ok(ParamKind::String),
        (Some("boolean"), _) => Ok(ParamKind::Bool),
        (Some("integer"), Some("int32")) => Ok(ParamKind::Int32),
        (Some("integer"), _) => Ok(ParamKind::Int64),
        other => Err(format!(
            "parameter {}: unsupported type {other:?}",
            parameter["name"]
        )),
    }
}

fn body_of(operation: &Value, naming: &Naming) -> Result<Option<String>, String> {
    operation["requestBody"]["content"]["application/json"]["schema"]["$ref"]
        .as_str()
        .map(|reference| Ok(naming.type_for(&reference_name(reference)?)))
        .transpose()
}

/// The representations the generator can emit a method for. Anything else fails generation:
/// a route the model describes and the crate cannot call is worse than no crate at all,
/// and a method that silently returned `()` for a page HEY serves is how the gate went red
/// after the first `text/html` route arrived.
const REPRESENTATIONS: &[&str] = &["application/json", "text/html"];

fn response_of(operation: &Value, naming: &Naming) -> Result<Response, String> {
    let id = operation["operationId"].as_str().unwrap_or("operation");
    let responses = operation["responses"]
        .as_object()
        .ok_or(format!("{id} has no responses"))?;
    let mut agreed: Option<(&str, Response)> = None;
    for (status, response) in responses {
        if !status.starts_with('2') {
            continue;
        }
        let representation = representation_of(id, status, response, naming)?;
        match &agreed {
            None => agreed = Some((status, representation)),
            Some((first, expected)) if *expected != representation => {
                return Err(format!(
                    "{id} answers {first} and {status} differently ({} and {}); the generator emits one representation per operation",
                    expected.describe(),
                    representation.describe()
                ));
            }
            Some(_) => {}
        }
    }
    agreed
        .map(|(_, representation)| representation)
        .ok_or(format!("{id} has no 2xx response"))
}

/// What one success response is answered as. A response object that is itself a `$ref`
/// is refused rather than read as bodyless: what it points at may well be a page.
fn representation_of(
    id: &str,
    status: &str,
    response: &Value,
    naming: &Naming,
) -> Result<Response, String> {
    if response.get("$ref").is_some() {
        return Err(format!(
            "{id} answers {status} through a $ref response, which the generator does not resolve; write the response inline"
        ));
    }
    let Some(content) = response.get("content").and_then(Value::as_object) else {
        return Ok(Response::Empty);
    };
    let representations: Vec<&str> = content.keys().map(String::as_str).collect();
    let (media_type, body) = match representations.as_slice() {
        [] => return Ok(Response::Empty),
        [one] => (*one, &content[*one]),
        many => {
            return Err(format!(
                "{id} answers {status} in {} representations ({}); the generator emits one",
                many.len(),
                many.join(", ")
            ));
        }
    };
    if !REPRESENTATIONS.contains(&media_type) {
        return Err(format!(
            "{id} answers {status} as {media_type}, which the generator has no representation for; it emits {}",
            REPRESENTATIONS.join(" and ")
        ));
    }
    let reference = body["schema"]["$ref"].as_str().ok_or(format!(
        "{id} answers {status} as {media_type} with a schema that is not a $ref to components.schemas"
    ))?;
    let name = naming.type_for(&reference_name(reference)?);
    Ok(match media_type {
        "text/html" => Response::Html(name),
        _ => Response::Json(name),
    })
}

fn idempotent(http_method: &str, operation: &Value) -> bool {
    match operation["x-hey-idempotent"]["natural"].as_bool() {
        Some(natural) => natural,
        None => matches!(http_method, "get" | "head" | "put" | "delete"),
    }
}

fn readonly(semantics: &Value, id: &str) -> Result<bool, String> {
    semantics["readonly"]
        .as_bool()
        .ok_or(format!("{id} has no readonly in behavior-model.json"))
}

fn empty_on(operation: &Value) -> Vec<u16> {
    status_codes(&operation["x-hey-empty-on"]["statusCodes"])
}

fn pagination(semantics: &Value) -> Result<Pagination, String> {
    match semantics["pagination"]["style"].as_str() {
        None => Ok(Pagination::None),
        Some("link") => Ok(Pagination::Link),
        Some("window") => Ok(Pagination::Window),
        Some(other) => Err(format!("unsupported pagination style {other}")),
    }
}

fn retry(semantics: &Value) -> Retry {
    let retry = &semantics["retry"];
    Retry {
        max: u32::try_from(retry["max"].as_u64().unwrap_or(0)).unwrap_or(u32::MAX),
        base_delay_ms: retry["base_delay_ms"].as_u64().unwrap_or(1000),
        on: status_codes(&retry["retry_on"]),
    }
}

/// The HTTP statuses a list names. Anything that is not one — not a number, or past what a
/// status can be — is left out.
fn status_codes(codes: &Value) -> Vec<u16> {
    codes
        .as_array()
        .into_iter()
        .flatten()
        .filter_map(Value::as_u64)
        .filter_map(|code| u16::try_from(code).ok())
        .collect()
}

fn description_of(value: &Value) -> Option<String> {
    value["description"].as_str().map(str::to_string)
}

const SCHEMA_REFERENCE: &str = "#/components/schemas/";

/// The schema a `$ref` names. Only a reference into this document's own schemas can
/// become a type here; anything else would be emitted as a name Rust has never heard of.
fn reference_name(reference: &str) -> Result<String, String> {
    reference
        .strip_prefix(SCHEMA_REFERENCE)
        .filter(|name| !name.is_empty() && !name.contains('/'))
        .map(str::to_string)
        .ok_or(format!(
            "$ref {reference} does not point into {SCHEMA_REFERENCE}; the generator resolves nothing else"
        ))
}

/// Every schema a field, a body or a response names has to exist, or the emitted type would
/// refer to something that is nowhere.
fn check_references(schemas: &[Schema], services: &[Service]) -> Result<(), String> {
    let known: BTreeSet<&str> = schemas.iter().map(|schema| schema.name.as_str()).collect();
    for schema in schemas {
        let mentioned: Vec<&FieldType> = match &schema.shape {
            Shape::Struct(shape) => shape.fields.iter().map(|field| &field.kind).collect(),
            Shape::Alias(kind) => vec![kind],
        };
        for kind in mentioned {
            if let Some(name) = kind.named()
                && !known.contains(name.as_str())
            {
                return Err(format!(
                    "{} refers to {name}, which is not in components.schemas",
                    schema.name
                ));
            }
        }
    }
    for operation in services.iter().flat_map(|service| &service.operations) {
        let mentioned = [
            operation.body.as_deref(),
            match &operation.response {
                Response::Empty => None,
                Response::Json(name) | Response::Html(name) => Some(name.as_str()),
            },
        ];
        for name in mentioned.into_iter().flatten() {
            if !known.contains(name) {
                return Err(format!(
                    "{} refers to {name}, which is not in components.schemas",
                    operation.id
                ));
            }
        }
    }
    Ok(())
}

impl FieldType {
    /// The schema this type names, if it names one.
    fn named(&self) -> Option<String> {
        match self {
            FieldType::Named(name) => Some(name.clone()),
            FieldType::List(inner) | FieldType::Map(inner) => inner.named(),
            _ => None,
        }
    }

    fn mentions(&self, schema: &str) -> bool {
        match self {
            FieldType::Named(name) => name == schema,
            FieldType::List(inner) | FieldType::Map(inner) => inner.mentions(schema),
            _ => false,
        }
    }
}
