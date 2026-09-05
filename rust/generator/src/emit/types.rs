use std::fmt::Write;

use crate::emit::{HEADER, doc_comment};
use crate::model::{FieldType, Model, Schema, Shape};
use crate::naming::{field_ident, variant_method};

pub fn render(model: &Model) -> String {
    let mut out = String::from(HEADER);
    out.push_str("use std::collections::BTreeMap;\n\n");
    out.push_str("use serde::{Deserialize, Serialize};\n\n");
    out.push_str("use crate::types::{Date, DateTime, SensitiveString};\n\n");
    for schema in &model.schemas {
        render_schema(&mut out, schema);
    }
    out
}

fn render_schema(out: &mut String, schema: &Schema) {
    out.push_str(&doc_comment(schema.description.as_deref(), ""));
    match &schema.shape {
        Shape::Alias(kind) => {
            writeln!(
                out,
                "pub type {} = {};\n",
                schema.name,
                rust_type(kind, false)
            )
            .unwrap();
        }
        Shape::Struct(shape) => {
            out.push_str("#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]\n");
            writeln!(out, "pub struct {} {{", schema.name).unwrap();
            for field in &shape.fields {
                out.push_str(&doc_comment(field.description.as_deref(), "    "));
                let ident = field_ident(&field.wire_name);
                if ident.trim_start_matches("r#") != field.wire_name {
                    writeln!(out, "    #[serde(rename = \"{}\")]", field.wire_name).unwrap();
                }
                let kind = rust_type(&field.kind, field.recursive);
                // A required field HEY leaves out — or blanks with null, which Go's
                // encoding/json reads into a non-pointer as a no-op — reads as its zero
                // value rather than failing the whole response.
                //
                // A moment has no zero value worth having: an omitted DateTime or Date
                // would read as the epoch, which is a wrong answer rather than an empty
                // one, so those carry no default and a response missing one fails.
                if field.required {
                    match required_default(&field.kind) {
                        RequiredDefault::NullOrMissing => out.push_str("    #[serde(default, deserialize_with = \"crate::types::null_as_default::deserialize\")]\n"),
                        RequiredDefault::Missing => out.push_str("    #[serde(default)]\n"),
                        RequiredDefault::None => {}
                    }
                    writeln!(out, "    pub {ident}: {kind},").unwrap();
                } else {
                    // HEY blanks a date it has nothing for rather than leaving it out, and
                    // "" is not a date, so an optional one is read through a reader that
                    // takes both that and null as "no date".
                    if field.kind == FieldType::Date {
                        out.push_str("    #[serde(default, with = \"crate::types::optional_date\", skip_serializing_if = \"Option::is_none\")]\n");
                    } else {
                        out.push_str(
                            "    #[serde(default, skip_serializing_if = \"Option::is_none\")]\n",
                        );
                    }
                    writeln!(out, "    pub {ident}: Option<{kind}>,").unwrap();
                }
            }
            out.push_str("}\n\n");
            if let Some(polymorphic) = &shape.polymorphic {
                writeln!(out, "impl {} {{", schema.name).unwrap();
                let discriminator = field_ident(&polymorphic.discriminator);
                for variant in &polymorphic.variants {
                    writeln!(
                        out,
                        "    pub fn {}(&self) -> bool {{",
                        variant_method(variant)
                    )
                    .unwrap();
                    writeln!(out, "        self.{discriminator} == \"{variant}\"").unwrap();
                    out.push_str("    }\n\n");
                }
                out.push_str("}\n\n");
            }
        }
    }
}

/// How far a required field's absence is forgiven.
enum RequiredDefault {
    /// The type has a zero value HEY's own clients would read, so both a missing field and
    /// an explicit `null` land on it.
    NullOrMissing,
    /// A missing field lands on the type's default; a `null` still fails.
    Missing,
    /// Neither is forgiven.
    None,
}

fn required_default(kind: &FieldType) -> RequiredDefault {
    match kind {
        FieldType::String
        | FieldType::SensitiveString
        | FieldType::Bool
        | FieldType::Int32
        | FieldType::Int64
        | FieldType::List(_)
        | FieldType::Map(_) => RequiredDefault::NullOrMissing,
        FieldType::DateTime | FieldType::Date => RequiredDefault::None,
        FieldType::Json | FieldType::Named(_) => RequiredDefault::Missing,
    }
}

pub fn rust_type(kind: &FieldType, recursive: bool) -> String {
    match kind {
        FieldType::String => "String".into(),
        FieldType::SensitiveString => "SensitiveString".into(),
        FieldType::DateTime => "DateTime".into(),
        FieldType::Date => "Date".into(),
        FieldType::Bool => "bool".into(),
        FieldType::Int32 => "i32".into(),
        FieldType::Int64 => "i64".into(),
        FieldType::Json => "serde_json::Value".into(),
        FieldType::Named(name) if recursive => format!("::std::boxed::Box<{name}>"),
        FieldType::Named(name) => name.clone(),
        FieldType::List(inner) => format!("Vec<{}>", rust_type(inner, false)),
        FieldType::Map(inner) => format!("BTreeMap<String, {}>", rust_type(inner, false)),
    }
}
