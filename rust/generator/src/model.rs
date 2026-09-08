use std::collections::{BTreeMap, BTreeSet};

use serde::Deserialize;
use serde_json::Value;

use crate::naming::{Naming, struct_name};

/// Which noun each operation acts on, read from the same `names.toml` the naming
/// overrides come from. Nothing in OpenAPI says it, and the SDKs have to agree on it, so
/// it is written down per service and overridden per operation where the two differ.
#[derive(Deserialize, Default)]
pub struct ResourceTypes {
    #[serde(default)]
    resource_types: BTreeMap<String, String>,
    #[serde(default)]
    operation_resource_types: BTreeMap<String, String>,
}

impl ResourceTypes {
    pub fn parse(source: &str) -> Result<ResourceTypes, String> {
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

pub struct Model {
    pub api_version: String,
    pub schemas: Vec<Schema>,
    pub services: Vec<Service>,
}

pub struct Schema {
    pub name: String,
    pub description: Option<String>,
    pub shape: Shape,
}

pub enum Shape {
    Struct(Struct),
    Alias(FieldType),
}

pub struct Struct {
    pub fields: Vec<Field>,
    pub polymorphic: Option<Polymorphic>,
}

pub struct Polymorphic {
    pub discriminator: String,
    pub variants: Vec<String>,
}

pub struct Field {
    pub wire_name: String,
    pub description: Option<String>,
    pub kind: FieldType,
    pub required: bool,
    pub recursive: bool,
}

#[derive(Clone, PartialEq)]
pub enum FieldType {
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

pub struct Service {
    pub name: String,
    pub operations: Vec<Operation>,
}

pub struct Operation {
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

pub struct PathParam {
    pub wire_name: String,
    pub kind: ParamKind,
    pub role: ParamRole,
}

/// Where a path parameter sits: the last segment names the record itself, anything before
/// it names a parent.
#[derive(Clone, Copy, PartialEq)]
pub enum ParamRole {
    Parent,
    Recording,
}

pub struct QueryParam {
    pub wire_name: String,
    pub kind: ParamKind,
    pub required: bool,
}

#[derive(Clone, Copy, PartialEq)]
pub enum ParamKind {
    String,
    Bool,
    Int32,
    Int64,
}

pub enum Response {
    Empty,
    Json(String),
}

#[derive(Clone, Copy, PartialEq)]
pub enum Pagination {
    None,
    Link,
    Window,
}

pub struct Retry {
    pub max: u32,
    pub base_delay_ms: u64,
    pub retry_on: Vec<u16>,
}

impl Model {
    pub fn build(
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
        Ok(Model {
            api_version,
            schemas,
            services,
        })
    }
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
            naming.type_for(&reference_name(reference)),
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
            if !matches!(
                http_method.as_str(),
                "get" | "post" | "put" | "patch" | "delete"
            ) {
                continue;
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
                method_name: naming.method_for(id, &service),
                description: description_of(operation),
                http_method: http_method.to_uppercase(),
                path: path.clone(),
                resource: tag.to_string(),
                resource_type: resource_types.for_operation(id, &service)?,
                path_params: path_params(operation, path)?,
                query_params: query_params(operation)?,
                body: body_of(operation, naming),
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

fn body_of(operation: &Value, naming: &Naming) -> Option<String> {
    operation["requestBody"]["content"]["application/json"]["schema"]["$ref"]
        .as_str()
        .map(|reference| naming.type_for(&reference_name(reference)))
}

fn response_of(operation: &Value, naming: &Naming) -> Result<Response, String> {
    let responses = operation["responses"]
        .as_object()
        .ok_or("operation has no responses")?;
    for (status, response) in responses {
        if status.starts_with('2') {
            return Ok(
                match response["content"]["application/json"]["schema"]["$ref"].as_str() {
                    Some(reference) => Response::Json(naming.type_for(&reference_name(reference))),
                    None => Response::Empty,
                },
            );
        }
    }
    Err(format!("{} has no 2xx response", operation["operationId"]))
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
    operation["x-hey-empty-on"]["statusCodes"]
        .as_array()
        .map(|codes| {
            codes
                .iter()
                .filter_map(Value::as_u64)
                .map(|code| code as u16)
                .collect()
        })
        .unwrap_or_default()
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
        max: retry["max"].as_u64().unwrap_or(0) as u32,
        base_delay_ms: retry["base_delay_ms"].as_u64().unwrap_or(1000),
        retry_on: retry["retry_on"]
            .as_array()
            .map(|codes| {
                codes
                    .iter()
                    .filter_map(Value::as_u64)
                    .map(|code| code as u16)
                    .collect()
            })
            .unwrap_or_default(),
    }
}

fn description_of(value: &Value) -> Option<String> {
    value["description"].as_str().map(str::to_string)
}

fn reference_name(reference: &str) -> String {
    reference
        .trim_start_matches("#/components/schemas/")
        .to_string()
}

impl FieldType {
    fn mentions(&self, schema: &str) -> bool {
        match self {
            FieldType::Named(name) => name == schema,
            FieldType::List(inner) | FieldType::Map(inner) => inner.mentions(schema),
            _ => false,
        }
    }
}
