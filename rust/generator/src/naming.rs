use std::collections::BTreeMap;

use heck::{ToPascalCase, ToSnakeCase};
use serde::Deserialize;

#[derive(Deserialize, Default)]
pub struct Naming {
    #[serde(default)]
    services: BTreeMap<String, String>,
    #[serde(default)]
    operation_services: BTreeMap<String, String>,
    #[serde(default)]
    operation_methods: BTreeMap<String, String>,
    #[serde(default)]
    type_names: BTreeMap<String, String>,
}

const KEYWORDS: &[&str] = &[
    "abstract", "as", "async", "await", "become", "box", "break", "const", "continue", "crate",
    "do", "dyn", "else", "enum", "extern", "false", "final", "fn", "for", "gen", "if", "impl",
    "in", "let", "loop", "macro", "match", "mod", "move", "mut", "override", "priv", "pub", "ref",
    "return", "self", "static", "struct", "super", "trait", "true", "try", "type", "typeof",
    "unsafe", "unsized", "use", "virtual", "where", "while", "yield",
];

impl Naming {
    pub fn parse(source: &str) -> Result<Naming, String> {
        toml::from_str(source).map_err(|error| format!("names.toml: {error}"))
    }

    pub fn service_for(&self, operation_id: &str, tag: &str) -> String {
        if let Some(service) = self.operation_services.get(operation_id) {
            service.clone()
        } else if let Some(service) = self.services.get(tag) {
            service.clone()
        } else {
            tag.to_snake_case()
        }
    }

    pub fn method_for(&self, operation_id: &str, service: &str) -> String {
        if let Some(method) = self.operation_methods.get(operation_id) {
            return method.clone();
        }
        let service_words: Vec<String> = service.split('_').map(singular).collect();
        let words: Vec<String> = camel_words(operation_id)
            .into_iter()
            .map(|word| word.to_lowercase())
            .filter(|word| !service_words.contains(&singular(word)))
            .collect();
        let method = words.join("_");
        if method.is_empty() || KEYWORDS.contains(&method.as_str()) {
            panic!(
                "{operation_id} becomes `{method}` in {service}; add an [operation_methods] override to names.toml"
            );
        }
        method
    }

    /// What a schema is called in Rust. A shape whose Smithy name collides with something
    /// the language already has is renamed here; the wire is untouched, since a type name
    /// never serializes.
    pub fn type_for(&self, schema: &str) -> String {
        self.type_names
            .get(schema)
            .cloned()
            .unwrap_or_else(|| schema.to_string())
    }
}

pub fn struct_name(service: &str) -> String {
    service.to_pascal_case()
}

pub fn field_ident(wire_name: &str) -> String {
    let ident = wire_name
        .replace(['[', ']'], "_")
        .trim_end_matches('_')
        .to_snake_case();
    if KEYWORDS.contains(&ident.as_str()) {
        format!("r#{ident}")
    } else {
        ident
    }
}

pub fn constant_name(operation_id: &str) -> String {
    operation_id.to_snake_case().to_uppercase()
}

pub fn variant_method(variant: &str) -> String {
    format!("is_{}", variant.to_snake_case())
}

fn camel_words(source: &str) -> Vec<String> {
    let mut words = Vec::new();
    let mut current = String::new();
    for character in source.chars() {
        if character.is_uppercase() && !current.is_empty() {
            words.push(std::mem::take(&mut current));
        }
        current.push(character);
    }
    if !current.is_empty() {
        words.push(current);
    }
    words
}

fn singular(word: &str) -> String {
    if let Some(stem) = word.strip_suffix("ies") {
        format!("{stem}y")
    } else if word.ends_with("ses")
        || word.ends_with("xes")
        || word.ends_with("ches")
        || word.ends_with("shes")
    {
        word[..word.len() - 2].to_string()
    } else if word.ends_with('s') && !word.ends_with("ss") {
        word[..word.len() - 1].to_string()
    } else {
        word.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn methods_drop_the_service_noun() {
        let naming = Naming::default();
        assert_eq!(naming.method_for("ListBoxes", "boxes"), "list");
        assert_eq!(
            naming.method_for("GetBoxPostingChanges", "postings"),
            "get_box_changes"
        );
        assert_eq!(
            naming.method_for("GetTopicEntries", "topics"),
            "get_entries"
        );
        assert_eq!(
            naming.method_for("ListTimeTrackCategories", "time_tracks"),
            "list_categories"
        );
        assert_eq!(
            naming.method_for("GetOngoingTimeTrack", "time_tracks"),
            "get_ongoing"
        );
        assert_eq!(naming.method_for("CreateSticky", "stickies"), "create");
        assert_eq!(
            naming.method_for("UpdateContactClearance", "contacts"),
            "update_clearance"
        );
        assert_eq!(
            naming.method_for("UpdateMyClearance", "clearances"),
            "update_my"
        );
    }

    #[test]
    fn variant_methods_drop_the_ruby_namespace_separator() {
        assert_eq!(variant_method("Calendar::Event"), "is_calendar_event");
        assert_eq!(
            variant_method("Calendar::JournalEntry"),
            "is_calendar_journal_entry"
        );
        assert_eq!(
            variant_method("Calendar::Habit::Completion"),
            "is_calendar_habit_completion"
        );
        assert_eq!(variant_method("bundle"), "is_bundle");
    }

    #[test]
    fn type_names_are_the_models_own_unless_overridden() {
        let naming = Naming::parse("[type_names]\nBox = \"Mailbox\"\n").unwrap();

        assert_eq!(naming.type_for("Box"), "Mailbox");
        assert_eq!(naming.type_for("BoxGroup"), "BoxGroup");
        assert_eq!(Naming::default().type_for("Box"), "Box");
    }

    #[test]
    fn fields_escape_keywords_and_brackets() {
        assert_eq!(field_ident("type"), "r#type");
        assert_eq!(field_ident("box"), "r#box");
        assert_eq!(field_ident("boxId"), "box_id");
        assert_eq!(field_ident("refine[from]"), "refine_from");
    }
}
