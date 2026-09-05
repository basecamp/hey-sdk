use std::fmt::Write;

use crate::emit::{HEADER, doc_comment};
use crate::model::{
    Model, Operation, Pagination, ParamKind, ParamRole, PathParam, Response, Service,
};
use crate::naming::{constant_name, field_ident, struct_name};

pub fn render_mod(model: &Model) -> String {
    let mut out = String::from(HEADER);
    for service in &model.services {
        writeln!(out, "pub mod {};", service.name).unwrap();
    }
    out.push_str("\nuse crate::client::Client;\n\n");
    out.push_str("impl Client {\n");
    for service in &model.services {
        let name = struct_name(&service.name);
        writeln!(
            out,
            "    pub fn {}(&self) -> {}::{name}<'_> {{",
            service.name, service.name
        )
        .unwrap();
        writeln!(out, "        {}::{name}::new(self)", service.name).unwrap();
        out.push_str("    }\n\n");
    }
    out.push_str("}\n");
    out
}

pub fn render_service(service: &Service) -> String {
    let name = struct_name(&service.name);
    let mut out = String::from(HEADER);
    out.push_str("#![allow(clippy::too_many_arguments)]\n\n");
    out.push_str("use crate::client::Client;\n");
    out.push_str("use crate::error::Error;\n");
    out.push_str("use crate::generated::routes;\n");
    if service.operations.iter().any(uses_types) {
        out.push_str("use crate::generated::types::*;\n");
    }
    if service
        .operations
        .iter()
        .any(|operation| operation.pagination != Pagination::None)
    {
        out.push_str("use crate::pagination::Page;\n");
    }
    out.push('\n');

    for operation in &service.operations {
        render_params(&mut out, operation);
    }

    writeln!(out, "pub struct {name}<'a> {{").unwrap();
    out.push_str("    client: &'a Client,\n}\n\n");
    writeln!(out, "impl<'a> {name}<'a> {{").unwrap();
    out.push_str(
        "    pub(crate) fn new(client: &'a Client) -> Self {\n        Self { client }\n    }\n\n",
    );
    out.push_str("    /// The client this service sends through.\n");
    out.push_str("    pub fn client(&self) -> &'a Client {\n        self.client\n    }\n\n");
    for operation in &service.operations {
        render_method(&mut out, operation);
    }
    out.push_str("}\n");
    out
}

fn render_params(out: &mut String, operation: &Operation) {
    let optional: Vec<_> = operation
        .query_params
        .iter()
        .filter(|param| !param.required)
        .collect();
    if optional.is_empty() {
        return;
    }
    writeln!(out, "/// Optional query parameters for `{}`.", operation.id).unwrap();
    out.push_str("#[derive(Debug, Clone, Default, PartialEq)]\n");
    writeln!(out, "pub struct {}Params {{", operation.id).unwrap();
    for param in optional {
        writeln!(
            out,
            "    pub {}: Option<{}>,",
            field_ident(&param.wire_name),
            owned_type(param.kind)
        )
        .unwrap();
    }
    out.push_str("}\n\n");
}

fn render_method(out: &mut String, operation: &Operation) {
    let mut arguments = vec!["&self".to_string()];
    for param in &operation.path_params {
        arguments.push(format!(
            "{}: {}",
            field_ident(&param.wire_name),
            borrowed_type(param.kind)
        ));
    }
    for param in operation.query_params.iter().filter(|param| param.required) {
        arguments.push(format!(
            "{}: {}",
            field_ident(&param.wire_name),
            borrowed_type(param.kind)
        ));
    }
    let has_params = operation.query_params.iter().any(|param| !param.required);
    if has_params {
        arguments.push(format!("params: &{}Params", operation.id));
    }
    if let Some(body) = &operation.body {
        arguments.push(format!("body: &{body}"));
    }

    out.push_str(&doc_comment(operation.description.as_deref(), "    "));
    writeln!(
        out,
        "    pub async fn {}({}) -> Result<{}, Error> {{",
        operation.method_name,
        arguments.join(", "),
        return_type(operation)
    )
    .unwrap();

    let path_arguments: Vec<String> = operation
        .path_params
        .iter()
        .map(|param| format!("&{}", field_ident(&param.wire_name)))
        .collect();
    let named_record = named_record(operation);
    let binding = if operation.query_params.is_empty()
        && operation.body.is_none()
        && named_record.is_none()
    {
        "let"
    } else {
        "let mut"
    };
    writeln!(
        out,
        "        {binding} operation = self.client.operation(&routes::{}, &[{}]);",
        constant_name(&operation.id),
        path_arguments.join(", ")
    )
    .unwrap();
    if let Some(param) = named_record {
        writeln!(
            out,
            "        operation.resource_id({});",
            resource_id(param)
        )
        .unwrap();
    }
    for param in &operation.query_params {
        let ident = field_ident(&param.wire_name);
        if param.required {
            writeln!(
                out,
                "        operation.query(\"{}\", {ident});",
                param.wire_name
            )
            .unwrap();
        } else {
            writeln!(
                out,
                "        operation.query_optional(\"{}\", params.{ident}.as_ref());",
                param.wire_name
            )
            .unwrap();
        }
    }
    if operation.body.is_some() {
        out.push_str("        operation.json(body)?;\n");
    }
    writeln!(
        out,
        "        self.client.{}(operation).await",
        send_method(operation)
    )
    .unwrap();
    out.push_str("    }\n\n");
}

/// The path parameter that names the record the operation acts on: the id in the last
/// segment when the path ends in one, and otherwise the outermost parent's — the box a
/// group is created under, the topic whose entries are read, the topic a workflow staging
/// files. That is what Go names by hand in every one of these, staging included. A name or
/// a date says nothing an observer can look a record up by, so only numbers count.
fn named_record(operation: &Operation) -> Option<&PathParam> {
    numbered(operation, ParamRole::Recording)
        .next()
        .or_else(|| numbered(operation, ParamRole::Parent).next())
}

fn numbered(operation: &Operation, role: ParamRole) -> impl Iterator<Item = &PathParam> {
    operation.path_params.iter().filter(move |param| {
        param.role == role && matches!(param.kind, ParamKind::Int32 | ParamKind::Int64)
    })
}

fn resource_id(param: &PathParam) -> String {
    let ident = field_ident(&param.wire_name);
    match param.kind {
        ParamKind::Int32 => format!("i64::from({ident})"),
        _ => ident,
    }
}

fn uses_types(operation: &Operation) -> bool {
    operation.body.is_some() || matches!(operation.response, Response::Json(_))
}

/// A paginated read answers a [`Page`], whichever style it paginates in. A window read is
/// paginated too: HEY draws the window the caller asked for and still sends the `Link` and
/// `X-Total-Count` headers when there is more of it than one answer holds, so the cursor has
/// to be reachable there as well.
fn paginated(operation: &Operation) -> bool {
    operation.pagination != Pagination::None
}

fn return_type(operation: &Operation) -> String {
    match (
        &operation.response,
        paginated(operation),
        operation.empty_on.is_empty(),
    ) {
        (Response::Empty, _, _) => "()".into(),
        (Response::Json(name), true, _) => format!("Page<{name}>"),
        (Response::Json(name), _, false) => format!("Option<{name}>"),
        (Response::Json(name), _, true) => name.clone(),
    }
}

fn send_method(operation: &Operation) -> &'static str {
    match (
        &operation.response,
        paginated(operation),
        operation.empty_on.is_empty(),
    ) {
        (Response::Empty, _, _) => "send_unit",
        (Response::Json(_), true, _) => "send_page",
        (Response::Json(_), _, false) => "send_optional",
        (Response::Json(_), _, true) => "send",
    }
}

fn owned_type(kind: ParamKind) -> &'static str {
    match kind {
        ParamKind::String => "String",
        ParamKind::Bool => "bool",
        ParamKind::Int32 => "i32",
        ParamKind::Int64 => "i64",
    }
}

fn borrowed_type(kind: ParamKind) -> &'static str {
    match kind {
        ParamKind::String => "&str",
        ParamKind::Bool => "bool",
        ParamKind::Int32 => "i32",
        ParamKind::Int64 => "i64",
    }
}
