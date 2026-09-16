/// Everything the emitters need, read once out of `openapi.json` and `behavior-model.json`.
struct Model {
    let apiVersion: String
    let schemas: [Schema]
    let services: [Service]

    static func build(openapi: JSON, behavior: JSON, naming: Naming) throws -> Model {
        guard let apiVersion = openapi["info"]?["version"]?.string else {
            throw GeneratorError("openapi.json has no info.version")
        }
        let schemas = try buildSchemas(openapi, naming)
        let services = try buildServices(openapi, behavior, naming)
        try checkReferences(schemas, services)
        try checkHtmlResponses(schemas, services)
        return Model(apiVersion: apiVersion, schemas: schemas, services: services)
    }
}

struct Schema {
    let name: String
    let description: String?
    let shape: Shape
}

enum Shape {
    case structure(fields: [Field], polymorphic: Polymorphic?)
    case alias(FieldType)
}

/// A schema with a discriminator: each variant is named by the model, and matched by every
/// value the model says HEY may send for it.
struct Polymorphic {
    struct Variant {
        let name: String
        let values: [String]
    }

    let discriminator: String
    let variants: [Variant]
}

struct Field {
    let wireName: String
    let description: String?
    let kind: FieldType
    let required: Bool
    /// The field names the type it belongs to, so it is optional whatever the model says.
    let recursive: Bool
    /// The field is the type it belongs to, not a collection of it, so a struct can only hold
    /// it through a box.
    let boxed: Bool
}

indirect enum FieldType: Equatable {
    case string
    case sensitiveString
    case dateTime
    case date
    case bool
    case int32
    case int64
    case json
    case named(String)
    case list(FieldType)
    case map(FieldType)

    /// The schema this type names, if it names one.
    var named: String? {
        switch self {
        case let .named(name): return name
        case let .list(inner), let .map(inner): return inner.named
        default: return nil
        }
    }
}

struct Service {
    let name: String
    let operations: [Operation]
}

struct Operation {
    let id: String
    /// The service in `snake_case`: `boxes`, `time_tracks`.
    let service: String
    let methodName: String
    let description: String?
    let httpMethod: String
    let path: String
    /// The tag the model files the operation under: `Boxes`, `Calendar Time Tracks`.
    let resource: String
    let resourceType: String
    let pathParams: [PathParam]
    let queryParams: [QueryParam]
    let body: String?
    let response: Response
    let idempotent: Bool
    let readonly: Bool
    let emptyOn: [Int]
    let pagination: Pagination
    let pageParameter: String?
    let retry: Retry
}

struct PathParam {
    let wireName: String
    let kind: ParamKind
    let role: ParamRole
}

struct QueryParam {
    let wireName: String
    let kind: ParamKind
    let required: Bool
}

enum ParamKind: String { case string, bool, int32, int64 }

/// Where a path parameter sits: the last segment names the record itself, anything before it
/// a parent.
enum ParamRole: String { case parent, recording }

enum Response: Equatable {
    case empty
    case json(String)
    case html(String)

    var describe: String {
        switch self {
        case .empty: return "no body"
        case let .json(name): return "\(name) as JSON"
        case let .html(name): return "\(name) as HTML"
        }
    }
}

enum Pagination: String { case none, link, window }

struct Retry {
    let max: Int
    let baseDelayMs: Int
    let on: [Int]
}

private let schemaReference = "#/components/schemas/"

/// The representations the generator can emit a method for. Anything else fails generation.
private let representations = ["application/json", "text/html"]

private func buildSchemas(_ openapi: JSON, _ naming: Naming) throws -> [Schema] {
    guard let components = openapi["components"]?["schemas"]?.object else {
        throw GeneratorError("openapi.json has no components.schemas")
    }
    return try components.sorted { $0.0 < $1.0 }.map { schemaName, schema in
        let name = naming.type(for: schemaName)
        let shape: Shape
        if let properties = schema["properties"] {
            let required = Set(schema["required"]?.array?.compactMap(\.string) ?? [])
            guard let members = properties.object else {
                throw GeneratorError("\(name).properties is not an object")
            }
            let fields = try members.map { wireName, property in
                let kind = try fieldType(wireName, property, naming)
                return Field(
                    wireName: wireName,
                    description: property["description"]?.string,
                    kind: kind,
                    required: required.contains(wireName),
                    recursive: kind.named == name,
                    boxed: kind == .named(name)
                )
            }
            shape = .structure(fields: fields, polymorphic: try polymorphic(schemaName, schema))
        } else {
            shape = .alias(try fieldType(schemaName, schema, naming))
        }
        return Schema(name: name, description: schema["description"]?.string, shape: shape)
    }
}

private func polymorphic(_ name: String, _ schema: JSON) throws -> Polymorphic? {
    guard let extensionValue = schema["x-hey-polymorphic"],
          let discriminator = extensionValue["discriminator"]?.string,
          let variants = extensionValue["variants"]?.object?.map(\.0)
    else { return nil }
    let aliases = extensionValue["discriminatorValues"]
    // A value list for a variant the schema does not declare is a model error, as the
    // TypeScript generator treats it; an empty or missing list means the name alone, as the
    // Rust generator treats it. Neither becomes a check that can never be true.
    if let stray = aliases?.keys.first(where: { !variants.contains($0) }) {
        throw GeneratorError("\(name): discriminatorValues names \(stray), which is not a variant")
    }
    return Polymorphic(
        discriminator: discriminator,
        variants: variants.map { variant in
            let declared = aliases?[variant]?.array?.compactMap(\.string).filter { !$0.isEmpty } ?? []
            return Polymorphic.Variant(name: variant, values: declared.isEmpty ? [variant] : declared)
        }
    )
}

private func fieldType(_ name: String, _ property: JSON, _ naming: Naming) throws -> FieldType {
    if let reference = property["$ref"]?.string {
        return .named(naming.type(for: try referenceName(reference)))
    }
    let format = property["format"]?.string
    switch property["type"]?.string {
    case "string":
        if property["x-hey-sensitive"] != nil { return .sensitiveString }
        if format == "date-time" || name.hasSuffix("_at") { return .dateTime }
        if format == "date" || name.hasSuffix("_on") { return .date }
        return .string
    case "boolean":
        return .bool
    case "integer":
        return format == "int32" ? .int32 : .int64
    case "array":
        return .list(try fieldType(name, property["items"] ?? .object([]), naming))
    case "object":
        if let additional = property["additionalProperties"] {
            return .map(try fieldType(name, additional, naming))
        }
        return .json
    case let other:
        throw GeneratorError("\(name): unsupported schema type \(other ?? "none")")
    }
}

private func buildServices(_ openapi: JSON, _ behavior: JSON, _ naming: Naming) throws -> [Service] {
    guard let paths = openapi["paths"]?.object else { throw GeneratorError("openapi.json has no paths") }
    guard let behaviors = behavior["operations"] else {
        throw GeneratorError("behavior-model.json has no operations")
    }
    var services: [String: [Operation]] = [:]

    for (path, item) in paths.sorted(by: { $0.0 < $1.0 }) {
        guard let methods = item.object else { throw GeneratorError("\(path) is not an object") }
        for (httpMethod, operation) in methods {
            switch httpMethod {
            case "get", "post", "put", "patch", "delete": break
            case "head", "options", "trace":
                throw GeneratorError("\(path) has a \(httpMethod) operation, which the generator does not emit")
            default: continue
            }
            guard let id = operation["operationId"]?.string else {
                throw GeneratorError("\(httpMethod) \(path) has no operationId")
            }
            guard let tag = operation["tags"]?.array?.first?.string else { throw GeneratorError("\(id) has no tag") }
            let service = naming.service(for: id, tag: tag)
            guard let semantics = behaviors[id] else {
                throw GeneratorError("\(id) is missing from behavior-model.json")
            }
            guard let readonly = semantics["readonly"]?.bool else {
                throw GeneratorError("\(id) has no readonly in behavior-model.json")
            }
            services[service, default: []].append(Operation(
                id: id,
                service: service,
                methodName: try naming.method(for: id, service: service),
                description: operation["description"]?.string,
                httpMethod: httpMethod.uppercased(),
                path: path,
                resource: tag,
                resourceType: try naming.resourceType(for: id, service: service),
                pathParams: try pathParams(operation, path),
                queryParams: try queryParams(operation),
                body: try body(id, operation, naming),
                response: try response(id, operation, naming),
                idempotent: idempotent(httpMethod, operation, semantics),
                readonly: readonly,
                emptyOn: statusCodes(operation["x-hey-empty-on"]?["statusCodes"]),
                pagination: try pagination(semantics),
                pageParameter: semantics["pagination"]?["pageParameter"]?.string,
                retry: retry(semantics)
            ))
        }
    }

    return try services.sorted { $0.key < $1.key }.map { name, operations in
        let sorted = operations.sorted { ($0.methodName, $0.id) < ($1.methodName, $1.id) }
        for (a, b) in zip(sorted, sorted.dropFirst()) where a.methodName == b.methodName {
            throw GeneratorError(
                "\(a.id) and \(b.id) both become \(serviceClassName(name)).\(a.methodName); add an [operation_methods] override to names.toml")
        }
        return Service(name: name, operations: sorted)
    }
}

private func pathParams(_ operation: JSON, _ path: String) throws -> [PathParam] {
    var trimmed = path
    if trimmed.hasSuffix(".json") { trimmed.removeLast(5) }
    let lastSegment = trimmed.split(separator: "/", omittingEmptySubsequences: false).last.map(String.init) ?? ""
    return try parameters(operation, in: "path").map { parameter in
        guard let wireName = parameter["name"]?.string else {
            throw GeneratorError("a path parameter of \(path) has no name")
        }
        return PathParam(
            wireName: wireName,
            kind: try paramKind(parameter),
            role: lastSegment == "{\(wireName)}" ? .recording : .parent
        )
    }
}

private func queryParams(_ operation: JSON) throws -> [QueryParam] {
    try parameters(operation, in: "query").map { parameter in
        guard let wireName = parameter["name"]?.string else { throw GeneratorError("a query parameter has no name") }
        return QueryParam(wireName: wireName, kind: try paramKind(parameter), required: parameter["required"]?.bool ?? false)
    }
}

private func parameters(_ operation: JSON, in location: String) -> [JSON] {
    (operation["parameters"]?.array ?? []).filter { $0["in"]?.string == location }
}

private func paramKind(_ parameter: JSON) throws -> ParamKind {
    let schema = parameter["schema"] ?? .object([])
    switch schema["type"]?.string {
    case "string": return .string
    case "boolean": return .bool
    case "integer": return schema["format"]?.string == "int32" ? .int32 : .int64
    case let other:
        throw GeneratorError("parameter \(parameter["name"]?.string ?? "?"): unsupported type \(other ?? "none")")
    }
}

/// The type an operation's request body is, or nil when it takes none. A body the generator
/// cannot send as the model describes it — in another representation, through a `$ref`
/// request body, or with a schema that is not a `$ref` — fails generation naming the
/// operation, rather than becoming a method that quietly sends nothing.
private func body(_ id: String, _ operation: JSON, _ naming: Naming) throws -> String? {
    guard let requestBody = operation["requestBody"] else { return nil }
    if requestBody["$ref"] != nil {
        throw GeneratorError("\(id) takes a $ref request body, which the generator does not resolve; write the body inline")
    }
    guard let content = requestBody["content"] else {
        throw GeneratorError("\(id) takes a request body with no content")
    }
    let mediaTypes = content.keys
    guard mediaTypes.count == 1, let mediaType = mediaTypes.first else {
        if mediaTypes.isEmpty { throw GeneratorError("\(id) takes a request body with no representation") }
        throw GeneratorError(
            "\(id) takes a request body in \(mediaTypes.count) representations (\(mediaTypes.joined(separator: ", "))); the generator sends one")
    }
    guard mediaType == "application/json" else {
        throw GeneratorError(
            "\(id) takes a \(mediaType) request body, which the generator has no representation for; it sends application/json")
    }
    guard let reference = content[mediaType]?["schema"]?["$ref"]?.string else {
        throw GeneratorError("\(id) takes a request body with a schema that is not a $ref to components.schemas")
    }
    return naming.type(for: try referenceName(reference))
}

private func response(_ id: String, _ operation: JSON, _ naming: Naming) throws -> Response {
    guard let responses = operation["responses"]?.object else { throw GeneratorError("\(id) has no responses") }
    var agreed: (String, Response)?
    for (status, response) in responses where status.hasPrefix("2") {
        let representation = try representationOf(id, status, response, naming)
        if let first = agreed {
            if first.1 != representation {
                throw GeneratorError(
                    "\(id) answers \(first.0) and \(status) differently (\(first.1.describe) and \(representation.describe)); the generator emits one representation per operation")
            }
        } else {
            agreed = (status, representation)
        }
    }
    guard let (_, representation) = agreed else { throw GeneratorError("\(id) has no 2xx response") }
    return representation
}

private func representationOf(_ id: String, _ status: String, _ response: JSON, _ naming: Naming) throws -> Response {
    if response["$ref"] != nil {
        throw GeneratorError(
            "\(id) answers \(status) through a $ref response, which the generator does not resolve; write the response inline")
    }
    guard let content = response["content"] else { return .empty }
    let mediaTypes = content.keys
    if mediaTypes.isEmpty { return .empty }
    guard mediaTypes.count == 1, let mediaType = mediaTypes.first else {
        throw GeneratorError(
            "\(id) answers \(status) in \(mediaTypes.count) representations (\(mediaTypes.joined(separator: ", "))); the generator emits one")
    }
    guard representations.contains(mediaType) else {
        throw GeneratorError(
            "\(id) answers \(status) as \(mediaType), which the generator has no representation for; it emits \(representations.joined(separator: " and "))")
    }
    guard let reference = content[mediaType]?["schema"]?["$ref"]?.string else {
        throw GeneratorError(
            "\(id) answers \(status) as \(mediaType) with a schema that is not a $ref to components.schemas")
    }
    let name = naming.type(for: try referenceName(reference))
    return mediaType == "text/html" ? .html(name) : .json(name)
}

/// Whether the route may be sent again after a failure: an explicit `x-hey-idempotent`
/// override first (UpdateMessage is a PUT that is not); otherwise what the behaviour model
/// says, as the TypeScript and Kotlin generators read it — a read, or an operation the Smithy
/// model calls idempotent, which is how a PATCH earns a resend — with the verb standing in
/// when the model says neither.
private func idempotent(_ httpMethod: String, _ operation: JSON, _ semantics: JSON) -> Bool {
    if let natural = operation["x-hey-idempotent"]?["natural"]?.bool { return natural }
    let readonly = semantics["readonly"]?.bool ?? false
    let modelled = semantics["idempotent"]?.bool ?? false
    return readonly || modelled || ["get", "head", "put", "delete"].contains(httpMethod)
}

private func pagination(_ semantics: JSON) throws -> Pagination {
    switch semantics["pagination"]?["style"]?.string {
    case nil: return .none
    case "link": return .link
    case "window": return .window
    case let other: throw GeneratorError("unsupported pagination style \(other ?? "")")
    }
}

private func retry(_ semantics: JSON) -> Retry {
    let retry = semantics["retry"] ?? .object([])
    return Retry(
        max: retry["max"]?.int ?? 0,
        baseDelayMs: retry["base_delay_ms"]?.int ?? 1000,
        on: statusCodes(retry["retry_on"])
    )
}

private func statusCodes(_ codes: JSON?) -> [Int] {
    (codes?.array ?? []).compactMap(\.int).filter { (100...999).contains($0) }
}

/// The schema a `$ref` names. Only a reference into this document's own schemas can become a
/// type.
private func referenceName(_ reference: String) throws -> String {
    guard reference.hasPrefix(schemaReference) else {
        throw GeneratorError("$ref \(reference) does not point into \(schemaReference); the generator resolves nothing else")
    }
    let name = String(reference.dropFirst(schemaReference.count))
    guard !name.isEmpty, !name.contains("/") else {
        throw GeneratorError("$ref \(reference) does not point into \(schemaReference); the generator resolves nothing else")
    }
    return name
}

private func checkReferences(_ schemas: [Schema], _ services: [Service]) throws {
    let known = Set(schemas.map(\.name))
    for schema in schemas {
        let mentioned: [FieldType]
        switch schema.shape {
        case let .structure(fields, _): mentioned = fields.map(\.kind)
        case let .alias(kind): mentioned = [kind]
        }
        for kind in mentioned {
            if let name = kind.named, !known.contains(name) {
                throw GeneratorError("\(schema.name) refers to \(name), which is not in components.schemas")
            }
        }
    }
    for operation in services.flatMap(\.operations) {
        var mentioned: [String] = []
        if let body = operation.body { mentioned.append(body) }
        switch operation.response {
        case .empty: break
        case let .json(name), let .html(name): mentioned.append(name)
        }
        for name in mentioned where !known.contains(name) {
            throw GeneratorError("\(operation.id) refers to \(name), which is not in components.schemas")
        }
    }
}

/// A page is handed back as the `String` it arrived as, so the schema an HTML response names
/// has to be a string alias.
private func checkHtmlResponses(_ schemas: [Schema], _ services: [Service]) throws {
    for operation in services.flatMap(\.operations) {
        guard case let .html(name) = operation.response else { continue }
        if !resolvesToString(schemas, name) {
            throw GeneratorError(
                "\(operation.id) answers text/html as \(name), which is not a string schema; an HTML page is handed back as a String")
        }
    }
}

private func resolvesToString(_ schemas: [Schema], _ name: String) -> Bool {
    var current = name
    for _ in 0...schemas.count {
        guard let shape = schemas.first(where: { $0.name == current })?.shape else { return false }
        switch shape {
        case .alias(.string): return true
        case let .alias(.named(next)): current = next
        default: return false
        }
    }
    return false
}
