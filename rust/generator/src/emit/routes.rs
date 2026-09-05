use std::fmt::Write;

use crate::emit::HEADER;
use crate::model::{Model, Operation, Pagination, ParamKind, ParamRole};
use crate::naming::constant_name;

pub fn render(model: &Model) -> String {
    let mut out = String::from(HEADER);
    out.push_str("use reqwest::Method;\n\n");
    out.push_str(
        "use crate::route::{Pagination, ParamKind, ParamRole, Retry, Route, RouteParam};\n\n",
    );

    let mut operations: Vec<&Operation> = model
        .services
        .iter()
        .flat_map(|service| &service.operations)
        .collect();
    operations.sort_by(|a, b| a.id.cmp(&b.id));

    for operation in &operations {
        render_route(&mut out, operation);
    }

    out.push_str("/// Every route the SDK knows, one per operation.\n");
    out.push_str("pub static ROUTES: &[&Route] = &[\n");
    for operation in &operations {
        writeln!(out, "    &{},", constant_name(&operation.id)).unwrap();
    }
    out.push_str("];\n");
    out
}

fn render_route(out: &mut String, operation: &Operation) {
    let pattern = operation.path.trim_end_matches(".json");
    writeln!(
        out,
        "pub static {}: Route = Route {{",
        constant_name(&operation.id)
    )
    .unwrap();
    writeln!(out, "    id: \"{}\",", operation.id).unwrap();
    writeln!(out, "    service: \"{}\",", operation.service).unwrap();
    writeln!(out, "    method: Method::{},", operation.http_method).unwrap();
    writeln!(out, "    path: \"{}\",", operation.path).unwrap();
    writeln!(out, "    pattern: \"{pattern}\",").unwrap();
    writeln!(out, "    resource: \"{}\",", operation.resource).unwrap();
    writeln!(out, "    resource_type: \"{}\",", operation.resource_type).unwrap();
    out.push_str("    params: &[\n");
    for param in &operation.path_params {
        writeln!(
            out,
            "        RouteParam {{ name: \"{}\", role: ParamRole::{}, kind: ParamKind::{} }},",
            param.wire_name,
            param_role(param.role),
            param_kind(param.kind)
        )
        .unwrap();
    }
    out.push_str("    ],\n");
    writeln!(out, "    idempotent: {},", operation.idempotent).unwrap();
    writeln!(out, "    readonly: {},", operation.readonly).unwrap();
    writeln!(out, "    empty_on: &{:?},", operation.empty_on).unwrap();
    let pagination = match operation.pagination {
        Pagination::None => "None",
        Pagination::Link => "Link",
        Pagination::Window => "Window",
    };
    writeln!(out, "    pagination: Pagination::{pagination},").unwrap();
    writeln!(
        out,
        "    retry: Retry {{ max: {}, base_delay_ms: {}, retry_on: &{:?} }},",
        operation.retry.max, operation.retry.base_delay_ms, operation.retry.retry_on
    )
    .unwrap();
    out.push_str("};\n\n");
}

fn param_role(role: ParamRole) -> &'static str {
    match role {
        ParamRole::Parent => "Parent",
        ParamRole::Recording => "Recording",
    }
}

fn param_kind(kind: ParamKind) -> &'static str {
    match kind {
        ParamKind::String => "String",
        ParamKind::Bool => "Bool",
        ParamKind::Int32 => "Int32",
        ParamKind::Int64 => "Int64",
    }
}
