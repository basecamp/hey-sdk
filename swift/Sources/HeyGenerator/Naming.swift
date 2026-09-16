import Foundation

/// The naming overrides read from `swift/names.toml`: which service an operation files under,
/// what a method is called when the derivation gets it wrong, what a schema is called when its
/// Smithy name collides with Swift's, and the noun each service's operations act on.
struct Naming {
    var services: [String: String] = [:]
    var operationServices: [String: String] = [:]
    var operationMethods: [String: String] = [:]
    var typeNames: [String: String] = [:]
    var resourceTypes: [String: String] = [:]
    var operationResourceTypes: [String: String] = [:]

    /// The service an operation files under, in `snake_case`: `boxes`, `time_tracks`.
    func service(for operationId: String, tag: String) -> String {
        operationServices[operationId] ?? services[tag] ?? snakeCase(tag)
    }

    /// The method an operation becomes on its service: the operation id's words with the
    /// service's own noun dropped, camelCased. `ListBoxes` on `boxes` is `list`;
    /// `GetBoxPostingChanges` on `postings` is `getBoxChanges`. An empty name or a Swift
    /// keyword is refused, and settled with an `[operation_methods]` override.
    func method(for operationId: String, service: String) throws -> String {
        let method: String
        if let override = operationMethods[operationId] {
            method = override
        } else {
            let serviceWords = service.split(separator: "_").map { singular(String($0)) }
            method = camelCase(camelWords(operationId)
                .map { $0.lowercased() }
                .filter { !serviceWords.contains(singular($0)) })
        }
        if method.isEmpty || swiftKeywords.contains(method) {
            throw GeneratorError(
                "\(operationId) becomes `\(method)` in \(service); add an [operation_methods] override to names.toml")
        }
        return method
    }

    /// What a schema is called in Swift: its own name unless `[type_names]` renames it.
    func type(for schema: String) -> String { typeNames[schema] ?? schema }

    /// The noun an operation acts on, as the hooks report it.
    func resourceType(for operationId: String, service: String) throws -> String {
        if let type = operationResourceTypes[operationId] ?? resourceTypes[service] { return type }
        throw GeneratorError("\(service) has no resource type; add one to the [resource_types] table in names.toml")
    }

    /// Reads the subset of TOML `names.toml` is written in: `[tables]` of `key = "value"`
    /// lines, keys quoted or bare, comments after `#`.
    static func parse(_ source: String) throws -> Naming {
        var tables: [String: [String: String]] = [:]
        var current: String?
        for (number, raw) in source.split(separator: "\n", omittingEmptySubsequences: false).enumerated() {
            let line = String(raw.split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false).first ?? "")
                .trimmingCharacters(in: .whitespaces)
            if line.isEmpty { continue }
            if line.hasPrefix("["), line.hasSuffix("]") {
                let name = String(line.dropFirst().dropLast()).trimmingCharacters(in: .whitespaces)
                tables[name, default: [:]] = tables[name, default: [:]]
                current = name
                continue
            }
            guard let table = current else {
                throw GeneratorError("names.toml line \(number + 1): key outside a table")
            }
            guard let separator = line.firstIndex(of: "=") else {
                throw GeneratorError("names.toml line \(number + 1): expected key = \"value\"")
            }
            let key = unquote(String(line[..<separator]).trimmingCharacters(in: .whitespaces))
            let value = String(line[line.index(after: separator)...]).trimmingCharacters(in: .whitespaces)
            guard value.count >= 2, value.hasPrefix("\""), value.hasSuffix("\"") else {
                throw GeneratorError("names.toml line \(number + 1): value must be a quoted string")
            }
            tables[table, default: [:]][key] = unquote(value)
        }
        return Naming(
            services: tables["services"] ?? [:],
            operationServices: tables["operation_services"] ?? [:],
            operationMethods: tables["operation_methods"] ?? [:],
            typeNames: tables["type_names"] ?? [:],
            resourceTypes: tables["resource_types"] ?? [:],
            operationResourceTypes: tables["operation_resource_types"] ?? [:]
        )
    }

    private static func unquote(_ value: String) -> String {
        value.count >= 2 && value.hasPrefix("\"") && value.hasSuffix("\"") ? String(value.dropFirst().dropLast()) : value
    }
}

/// Swift's keywords: the words that cannot be an identifier without backticks.
let swiftKeywords: Set<String> = [
    "associatedtype", "class", "deinit", "enum", "extension", "fileprivate", "func", "import", "init",
    "inout", "internal", "let", "open", "operator", "private", "precedencegroup", "protocol", "public",
    "rethrows", "static", "struct", "subscript", "typealias", "var", "break", "case", "catch", "continue",
    "default", "defer", "do", "else", "fallthrough", "for", "guard", "if", "in", "repeat", "return", "throw",
    "switch", "where", "while", "Any", "as", "await", "false", "is", "nil", "self", "Self", "super", "throws",
    "true", "try", "some", "any",
]

/// The class a service becomes: `time_tracks` is `TimeTracksService`.
func serviceClassName(_ service: String) -> String { pascalCase(service) + "Service" }

/// The property a service is reached through on the client: `time_tracks` is `timeTracks`.
func serviceAccessorName(_ service: String) -> String { lowerCamelCase(service) }

/// The bare identifier a wire name becomes: `email_address` is `emailAddress`, `refine[from]`
/// is `refineFrom`.
func identifier(_ wireName: String) -> String {
    let flattened = wireName.replacingOccurrences(of: "[", with: "_").replacingOccurrences(of: "]", with: "_")
    return lowerCamelCase(String(flattened.reversed().drop { $0 == "_" }.reversed()))
}

/// A wire name as it is declared: backticked when it is a keyword.
func declared(_ wireName: String) -> String {
    let ident = identifier(wireName)
    return swiftKeywords.contains(ident) ? "`\(ident)`" : ident
}

/// The constant a route is held under: `ListBoxes` is `listBoxes`.
func routeConstant(_ operationId: String) -> String {
    guard let first = operationId.first else { return operationId }
    return first.lowercased() + operationId.dropFirst()
}

/// The predicate a polymorphic variant answers to: `Calendar::Event` is `isCalendarEvent`.
func variantProperty(_ variant: String) -> String {
    "is" + variant.components(separatedBy: "::").map(pascalCase).joined()
}

/// `GetBoxPostingChanges` is `Get`, `Box`, `Posting`, `Changes`.
func camelWords(_ source: String) -> [String] {
    var words: [String] = []
    var current = ""
    for character in source {
        if character.isUppercase, !current.isEmpty {
            words.append(current)
            current = ""
        }
        current.append(character)
    }
    if !current.isEmpty { words.append(current) }
    return words
}

private func camelCase(_ words: [String]) -> String {
    words.enumerated().map { index, word in index == 0 ? word : capitalizedFirst(word) }.joined()
}

private func capitalizedFirst(_ word: String) -> String {
    guard let first = word.first else { return word }
    return first.uppercased() + word.dropFirst()
}

/// `boxes` is `box`, `entries` is `entry`, `addresses` is `address`, `status` is `status`.
func singular(_ word: String) -> String {
    if word.hasSuffix("ies") { return String(word.dropLast(3)) + "y" }
    if word.hasSuffix("ses") || word.hasSuffix("xes") || word.hasSuffix("ches") || word.hasSuffix("shes") {
        return String(word.dropLast(2))
    }
    if word.hasSuffix("s"), !word.hasSuffix("ss") { return String(word.dropLast()) }
    return word
}

/// `Calendar Periods` and `CalendarPeriods` are both `calendar_periods`.
func snakeCase(_ source: String) -> String {
    var out = ""
    var previousLower = false
    for character in source {
        if character == " " || character == "-" || character == "_" {
            if !out.isEmpty, out.last != "_" { out.append("_") }
            previousLower = false
        } else if character.isUppercase {
            if !out.isEmpty, out.last != "_", previousLower { out.append("_") }
            out.append(contentsOf: character.lowercased())
            previousLower = false
        } else {
            out.append(character)
            previousLower = character.isLowercase || character.isNumber
        }
    }
    return out.trimmingCharacters(in: CharacterSet(charactersIn: "_"))
}

/// `time_tracks` is `timeTracks`.
func lowerCamelCase(_ source: String) -> String {
    let parts = source.split(separator: "_").map(String.init).filter { !$0.isEmpty }
    return parts.enumerated().map { index, part in
        index == 0 ? (part.first.map { $0.lowercased() + part.dropFirst() } ?? part) : capitalizedFirst(part)
    }.joined()
}

/// `time_tracks` is `TimeTracks`.
func pascalCase(_ source: String) -> String {
    source.split(separator: "_").map(String.init).filter { !$0.isEmpty }.map(capitalizedFirst).joined()
}
