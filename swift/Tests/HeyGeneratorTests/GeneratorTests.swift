import XCTest

@testable import HeyGenerator

final class NamingTests: XCTestCase {
    private let naming = Naming()

    func testMethodsDropTheServiceNoun() throws {
        XCTAssertEqual(try naming.method(for: "ListBoxes", service: "boxes"), "list")
        XCTAssertEqual(try naming.method(for: "GetBoxPostingChanges", service: "postings"), "getBoxChanges")
        XCTAssertEqual(try naming.method(for: "GetTopicEntries", service: "topics"), "getEntries")
        XCTAssertEqual(try naming.method(for: "ListTimeTrackCategories", service: "time_tracks"), "listCategories")
        XCTAssertEqual(try naming.method(for: "GetOngoingTimeTrack", service: "time_tracks"), "getOngoing")
        XCTAssertEqual(try naming.method(for: "CreateSticky", service: "stickies"), "create")
        XCTAssertEqual(try naming.method(for: "UpdateContactClearance", service: "contacts"), "updateClearance")
        XCTAssertEqual(try naming.method(for: "UpdateMyClearance", service: "clearances"), "updateMy")
    }

    func testAKeywordOrAnEmptyNameIsRefused() {
        XCTAssertThrowsError(try naming.method(for: "Boxes", service: "boxes"))
        XCTAssertThrowsError(try naming.method(for: "InTopic", service: "topics"))
        XCTAssertThrowsError(try naming.method(for: "DefaultBox", service: "boxes"))
    }

    func testOverridesWin() throws {
        let naming = try Naming.parse("""
            [services]
            "Bulk Reply" = "bulk_replies"
            [operation_services]
            GetClearances = "clearances"
            [operation_methods]
            MoveTopic = "moveTopic"
            [type_names]
            Box = "Mailbox"
            [resource_types]
            boxes = "box" # a comment
            [operation_resource_types]
            CreateBoxGroup = "box_group"
            """)
        XCTAssertEqual(naming.service(for: "NewBulkReply", tag: "Bulk Reply"), "bulk_replies")
        XCTAssertEqual(naming.service(for: "GetClearances", tag: "Contacts"), "clearances")
        XCTAssertEqual(naming.service(for: "ListCalendarDays", tag: "Calendar Periods"), "calendar_periods")
        XCTAssertEqual(try naming.method(for: "MoveTopic", service: "topics"), "moveTopic")
        XCTAssertEqual(naming.type(for: "Box"), "Mailbox")
        XCTAssertEqual(naming.type(for: "BoxGroup"), "BoxGroup")
        XCTAssertEqual(try naming.resourceType(for: "ListBoxes", service: "boxes"), "box")
        XCTAssertEqual(try naming.resourceType(for: "CreateBoxGroup", service: "boxes"), "box_group")
        XCTAssertThrowsError(try naming.resourceType(for: "ListTopics", service: "topics"))
    }

    func testANamesFileThatIsNotTheSubsetIsRefused() {
        XCTAssertThrowsError(try Naming.parse("boxes = \"box\""))
        XCTAssertThrowsError(try Naming.parse("[resource_types]\nboxes box"))
        XCTAssertThrowsError(try Naming.parse("[resource_types]\nboxes = box"))
    }

    func testIdentifiersEscapeKeywordsAndBrackets() {
        XCTAssertEqual(declared("default"), "`default`")
        XCTAssertEqual(identifier("default"), "default")
        XCTAssertEqual(declared("box_id"), "boxId")
        XCTAssertEqual(declared("boxId"), "boxId")
        XCTAssertEqual(declared("refine[from]"), "refineFrom")
        XCTAssertEqual(routeConstant("ListBoxes"), "listBoxes")
        XCTAssertEqual(routeConstant("GetBoxPostingChanges"), "getBoxPostingChanges")
        XCTAssertEqual(serviceClassName("time_tracks"), "TimeTracksService")
        XCTAssertEqual(serviceAccessorName("time_tracks"), "timeTracks")
        XCTAssertEqual(variantProperty("Calendar::Event"), "isCalendarEvent")
        XCTAssertEqual(variantProperty("bundle"), "isBundle")
    }

    func testCaseConversions() {
        XCTAssertEqual(snakeCase("Calendar Periods"), "calendar_periods")
        XCTAssertEqual(snakeCase("CalendarPeriods"), "calendar_periods")
        XCTAssertEqual(snakeCase("time_tracks"), "time_tracks")
        XCTAssertEqual(pascalCase("time_tracks"), "TimeTracks")
        XCTAssertEqual(singular("entries"), "entry")
        XCTAssertEqual(singular("addresses"), "address")
        XCTAssertEqual(singular("boxes"), "box")
    }

    func testTheJSONReaderKeepsOrderAndExactNumbers() throws {
        let json = try JSON.parse(#"{"b":1,"a":[true,null,"xé\n"],"n":9007199254740993,"f":-1.5e3}"#)
        XCTAssertEqual(json.keys, ["b", "a", "n", "f"])
        XCTAssertEqual(json["n"], .number("9007199254740993"))
        XCTAssertEqual(json["n"]?.int, 9_007_199_254_740_993)
        XCTAssertEqual(json["a"], .array([.bool(true), .null, .string("xé\n")]))
        XCTAssertEqual(json["f"], .number("-1.5e3"))
        XCTAssertThrowsError(try JSON.parse(#"{"a":1"#))
        XCTAssertThrowsError(try JSON.parse(#"{"a":1} x"#))
    }
}

final class ModelTests: XCTestCase {
    private let names = """
        [resource_types]
        boxes = "box"
        workflows = "workflow"
        senders = "sender"
        """

    private func model(_ paths: String, _ schemas: String, behavior: [String: String] = [:]) throws -> Model {
        let openapi = try JSON.parse(#"{"openapi":"3.1.0","info":{"version":"2026-01-01"},"paths":\#(paths),"components":{"schemas":\#(schemas)}}"#)
        var operations: [String] = []
        for (_, item) in openapi["paths"]?.object ?? [] {
            for (_, operation) in item.object ?? [] {
                let id = operation["operationId"]?.string ?? ""
                operations.append("\"\(id)\":\(behavior[id] ?? #"{"readonly": false}"#)")
            }
        }
        let behaviorModel = try JSON.parse("{\"operations\":{\(operations.joined(separator: ","))}}")
        return try Model.build(openapi: openapi, behavior: behaviorModel, naming: try Naming.parse(names))
    }

    private let boxSchemas = #"""
        {
        "Box": {"type":"object","properties":{"id":{"type":"integer","format":"int64"},"kind":{"type":"string"},"owner":{"$ref":"#/components/schemas/Owner"},"created_at":{"type":"string"},"trial_ends_on":{"type":"string"},"email_address":{"type":"string","x-hey-sensitive":{"category":"pii"}},"labels":{"type":"array","items":{"type":"string"}},"extra":{"type":"object"},"counts":{"type":"object","additionalProperties":{"type":"integer","format":"int32"}},"default":{"type":"boolean"},"parent":{"$ref":"#/components/schemas/Box"},"children":{"type":"array","items":{"$ref":"#/components/schemas/Box"}}},"required":["id","kind","parent"],"x-hey-polymorphic":{"discriminator":"kind","variants":{"topic":["name"],"Calendar::Event":[]},"discriminatorValues":{"Calendar::Event":["CalendarEvent","Calendar::Event"]}}},
        "Owner": {"type":"object","properties":{"name":{"type":"string"}}},
        "Empty": {"type":"object","properties":{}},
        "GetBoxResponseContent": {"$ref":"#/components/schemas/Box"},
        "ListBoxesResponseContent": {"type":"array","items":{"$ref":"#/components/schemas/Box"}},
        "StagePage": {"type":"string"}
        }
        """#

    private let boxPaths = #"""
        {
        "/boxes.json": {"get": {"operationId":"ListBoxes","tags":["Boxes"],"description":"List the boxes","responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ListBoxesResponseContent"}}}}}}},
        "/boxes/{boxId}": {"get": {"operationId":"GetBox","tags":["Boxes"],"parameters":[{"name":"boxId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}},{"name":"page","in":"query","schema":{"type":"string"}},{"name":"since","in":"query","required":true,"schema":{"type":"string"}}],"x-hey-empty-on":{"statusCodes":[404]},"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/GetBoxResponseContent"}}}},"404":{"description":"gone"}}}},
        "/boxes/{boxId}/observation.json": {"post": {"operationId":"MarkBoxSeen","tags":["Boxes"],"x-hey-idempotent":{"natural":true},"parameters":[{"name":"boxId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}}],"responses":{"200":{"description":"ok"}}}},
        "/workflows/{workflowId}/stages/{stageId}": {"get": {"operationId":"GetWorkflowStage","tags":["Workflows"],"parameters":[{"name":"workflowId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}},{"name":"stageId","in":"path","required":true,"schema":{"type":"integer","format":"int32"}}],"responses":{"200":{"content":{"text/html":{"schema":{"$ref":"#/components/schemas/StagePage"}}}}}}}
        }
        """#

    func testSchemasBecomeStructsAndAliases() throws {
        let model = try model(boxPaths, boxSchemas)
        XCTAssertEqual(model.apiVersion, "2026-01-01")
        let box = try XCTUnwrap(model.schemas.first { $0.name == "Box" })
        guard case let .structure(fields, polymorphic) = box.shape else { return XCTFail("Box is a struct") }
        let byName = Dictionary(uniqueKeysWithValues: fields.map { ($0.wireName, $0) })
        XCTAssertEqual(byName["id"]?.kind, .int64)
        XCTAssertEqual(byName["id"]?.required, true)
        XCTAssertEqual(byName["owner"]?.kind, .named("Owner"))
        XCTAssertEqual(byName["created_at"]?.kind, .dateTime)
        XCTAssertEqual(byName["trial_ends_on"]?.kind, .date)
        XCTAssertEqual(byName["email_address"]?.kind, .sensitiveString)
        XCTAssertEqual(byName["labels"]?.kind, .list(.string))
        XCTAssertEqual(byName["extra"]?.kind, .json)
        XCTAssertEqual(byName["counts"]?.kind, .map(.int32))
        XCTAssertEqual(byName["parent"]?.boxed, true)
        XCTAssertEqual(byName["parent"]?.recursive, true)
        XCTAssertEqual(byName["children"]?.boxed, false, "an array of the type needs no box")
        XCTAssertEqual(byName["children"]?.recursive, true)
        XCTAssertEqual(polymorphic?.variants.map(\.name), ["topic", "Calendar::Event"])
        XCTAssertEqual(polymorphic?.variants[0].values, ["topic"], "a variant the model gives no aliases is matched by its name")
        XCTAssertEqual(polymorphic?.variants[1].values, ["CalendarEvent", "Calendar::Event"])
        guard case .alias(.list(.named("Box"))) = model.schemas.first(where: { $0.name == "ListBoxesResponseContent" })?.shape else {
            return XCTFail("ListBoxesResponseContent is an alias of a list")
        }
    }

    func testOperationsCarryTheirBehaviour() throws {
        let model = try model(boxPaths, boxSchemas, behavior: [
            "ListBoxes": #"{"readonly":true,"pagination":{"style":"link"},"retry":{"max":3,"base_delay_ms":1000,"retry_on":[429,503]}}"#,
        ])
        let boxes = try XCTUnwrap(model.services.first { $0.name == "boxes" })
        XCTAssertEqual(boxes.operations.map(\.methodName), ["get", "list", "markSeen"])
        let list = try XCTUnwrap(boxes.operations.first { $0.id == "ListBoxes" })
        XCTAssertEqual(list.pagination, .link)
        XCTAssertEqual(list.retry.max, 3)
        XCTAssertEqual(list.retry.on, [429, 503])
        XCTAssertTrue(list.readonly)
        XCTAssertTrue(list.idempotent)
        XCTAssertEqual(list.response, .json("ListBoxesResponseContent"))

        let get = try XCTUnwrap(boxes.operations.first { $0.id == "GetBox" })
        XCTAssertEqual(get.emptyOn, [404])
        XCTAssertEqual(get.pathParams.map(\.role), [.recording])
        XCTAssertEqual(get.queryParams.map { "\($0.wireName):\($0.required)" }, ["page:false", "since:true"])
        XCTAssertEqual(get.retry.max, 0)

        let seen = try XCTUnwrap(boxes.operations.first { $0.id == "MarkBoxSeen" })
        XCTAssertTrue(seen.idempotent, "x-hey-idempotent.natural wins over the method")
        XCTAssertEqual(seen.response, .empty)

        let stage = try XCTUnwrap(model.services.first { $0.name == "workflows" }?.operations.first)
        XCTAssertEqual(stage.response, .html("StagePage"))
        XCTAssertEqual(stage.pathParams.map(\.role), [.parent, .recording])
    }

    func testAnHTMLResponseThatIsNotAStringIsRefused() {
        let schemas = boxSchemas.replacingOccurrences(of: #""StagePage": {"type":"string"}"#, with: #""StagePage": {"type":"object","properties":{}}"#)
        XCTAssertThrowsError(try model(boxPaths, schemas)) { error in
            XCTAssertTrue("\(error)".contains("GetWorkflowStage answers text/html as StagePage"), "\(error)")
        }
    }

    func testAMissingReferenceIsRefused() {
        let schemas = boxSchemas.replacingOccurrences(of: #""Owner": {"type":"object","properties":{"name":{"type":"string"}}},"#, with: "")
        XCTAssertThrowsError(try model(boxPaths, schemas)) { error in
            XCTAssertTrue("\(error)".contains("Box refers to Owner"), "\(error)")
        }
    }

    func testAnUnknownRepresentationIsRefused() {
        let paths = boxPaths.replacingOccurrences(of: "text/html", with: "image/png")
        XCTAssertThrowsError(try model(paths, boxSchemas)) { error in
            XCTAssertTrue("\(error)".contains("image/png"), "\(error)")
        }
    }

    func testARequestBodyTheGeneratorCannotSendIsRefused() throws {
        let markSeen = #""operationId":"MarkBoxSeen","tags":["Boxes"],"#
        let inline = boxPaths.replacingOccurrences(
            of: markSeen,
            with: markSeen + #""requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"seen":{"type":"boolean"}}}}}},"#)
        XCTAssertThrowsError(try model(inline, boxSchemas)) { error in
            XCTAssertTrue("\(error)".contains("MarkBoxSeen takes a request body with a schema that is not a $ref"), "\(error)")
        }
        let form = boxPaths.replacingOccurrences(
            of: markSeen,
            with: markSeen + ##""requestBody":{"content":{"application/x-www-form-urlencoded":{"schema":{"$ref":"#/components/schemas/Box"}}}},"##)
        XCTAssertThrowsError(try model(form, boxSchemas)) { error in
            XCTAssertTrue("\(error)".contains("MarkBoxSeen takes a application/x-www-form-urlencoded request body"), "\(error)")
        }
        let referenced = boxPaths.replacingOccurrences(of: markSeen, with: markSeen + ##""requestBody":{"$ref":"#/components/requestBodies/Seen"},"##)
        XCTAssertThrowsError(try model(referenced, boxSchemas)) { error in
            XCTAssertTrue("\(error)".contains("MarkBoxSeen takes a $ref request body"), "\(error)")
        }
        let supported = boxPaths.replacingOccurrences(
            of: markSeen, with: markSeen + ##""requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Box"}}}},"##)
        XCTAssertEqual(try model(supported, boxSchemas).services.first { $0.name == "boxes" }?.operations.first { $0.id == "MarkBoxSeen" }?.body, "Box")
    }

    func testDiscriminatorValuesAreReadTheWayTheOtherGeneratorsReadThem() throws {
        let declaredValues = #""discriminatorValues":{"Calendar::Event":["CalendarEvent","Calendar::Event"]}"#
        let empty = try model(boxPaths, boxSchemas.replacingOccurrences(of: declaredValues, with: #""discriminatorValues":{"Calendar::Event":[]}"#))
        guard case let .structure(_, polymorphic) = empty.schemas.first(where: { $0.name == "Box" })?.shape else { return XCTFail() }
        XCTAssertEqual(polymorphic?.variants[1].values, ["Calendar::Event"], "an empty list means the name alone, never a check that cannot be true")

        let stray = boxSchemas.replacingOccurrences(of: declaredValues, with: #""discriminatorValues":{"Calendar::Todo":["CalendarTodo"]}"#)
        XCTAssertThrowsError(try model(boxPaths, stray)) { error in
            XCTAssertTrue("\(error)".contains("Box: discriminatorValues names Calendar::Todo, which is not a variant"), "\(error)")
        }
    }

    func testIdempotencyComesFromTheOverrideThenTheModelThenTheVerb() throws {
        let markSeen = #""operationId":"MarkBoxSeen","tags":["Boxes"],"x-hey-idempotent":{"natural":true},"#
        let patched = boxPaths.replacingOccurrences(of: markSeen, with: #""operationId":"MarkBoxSeen","tags":["Boxes"],"#)
            .replacingOccurrences(of: #""/boxes/{boxId}/observation.json": {"post":"#, with: #""/boxes/{boxId}/observation.json": {"patch":"#)
        let fromModel = try model(patched, boxSchemas, behavior: ["MarkBoxSeen": #"{"readonly":false,"idempotent":true}"#])
        XCTAssertEqual(fromModel.services.first { $0.name == "boxes" }?.operations.first { $0.id == "MarkBoxSeen" }?.idempotent, true)
        let fromVerb = try model(patched, boxSchemas, behavior: ["MarkBoxSeen": #"{"readonly":false}"#])
        XCTAssertEqual(fromVerb.services.first { $0.name == "boxes" }?.operations.first { $0.id == "MarkBoxSeen" }?.idempotent, false)

        let put = boxPaths.replacingOccurrences(of: markSeen, with: #""operationId":"MarkBoxSeen","tags":["Boxes"],"x-hey-idempotent":{"natural":false},"#)
            .replacingOccurrences(of: #""/boxes/{boxId}/observation.json": {"post":"#, with: #""/boxes/{boxId}/observation.json": {"put":"#)
        let overridden = try model(put, boxSchemas, behavior: ["MarkBoxSeen": #"{"readonly":false,"idempotent":true}"#])
        XCTAssertEqual(overridden.services.first { $0.name == "boxes" }?.operations.first { $0.id == "MarkBoxSeen" }?.idempotent, false)
    }

    func testAMethodCollisionIsRefused() {
        let paths = boxPaths.replacingOccurrences(of: #""operationId":"MarkBoxSeen""#, with: #""operationId":"ListBox""#)
        XCTAssertThrowsError(try model(paths, boxSchemas)) { error in
            XCTAssertTrue("\(error)".contains("both become BoxesService.list"), "\(error)")
        }
    }

    func testRenderedCodeHasTheExpectedShape() throws {
        let model = try model(boxPaths, boxSchemas, behavior: [
            "ListBoxes": #"{"readonly":true,"pagination":{"style":"link","pageParameter":"page"},"retry":{"max":3,"base_delay_ms":1000,"retry_on":[429,503]}}"#,
        ])
        let files = Dictionary(uniqueKeysWithValues: render(model))
        let box = try XCTUnwrap(files["Models/Box.swift"])
        XCTAssertTrue(box.contains("public struct Box: Codable, Sendable, Equatable {"), box)
        XCTAssertTrue(box.contains("    public var id: Int\n    public var kind: String\n"), box)
        XCTAssertTrue(box.contains("    public var owner: Owner?\n"), box)
        XCTAssertTrue(box.contains("    public var emailAddress: SensitiveString?\n"), box)
        XCTAssertTrue(box.contains("    public var counts: [String: Int32]?\n"), box)
        XCTAssertTrue(box.contains("    public var extra: JSONValue?\n"), box)
        XCTAssertTrue(box.contains("    public var `default`: Bool?\n"), box)
        XCTAssertTrue(box.contains("        self.`default` = `default`\n"), box)
        XCTAssertTrue(box.contains("        case `default` = \"default\"\n"), box)
        XCTAssertTrue(box.contains("        case emailAddress = \"email_address\"\n"), box)
        XCTAssertTrue(box.contains("    private var _parent: Indirect<Box>?\n"), box)
        XCTAssertTrue(box.contains("        get { _parent?.value }\n"), box)
        XCTAssertTrue(box.contains("        case _parent = \"parent\"\n"), box)
        XCTAssertTrue(box.contains("    public var children: [Box]?\n"), box)
        XCTAssertTrue(box.contains("        parent: Box? = nil,\n"), "a member that is its own type is optional whatever the model says")
        XCTAssertTrue(box.contains("    public var isTopic: Bool { kind == \"topic\" }\n"), box)
        XCTAssertTrue(box.contains("    public var isCalendarEvent: Bool { [\"CalendarEvent\", \"Calendar::Event\"].contains(kind) }\n"), box)
        XCTAssertEqual(files["Models/Empty.swift"]?.contains("public struct Empty: Codable, Sendable, Equatable {\n    public init() {}\n}"), true)
        XCTAssertEqual(files["Models/ListBoxesResponseContent.swift"]?.contains("public typealias ListBoxesResponseContent = [Box]"), true)
        XCTAssertEqual(files["Models/StagePage.swift"]?.contains("public typealias StagePage = String"), true)

        let service = try XCTUnwrap(files["Services/BoxesService.swift"])
        XCTAssertTrue(service.contains("public final class BoxesService: BaseService, @unchecked Sendable {"), service)
        XCTAssertTrue(service.contains("public struct GetBoxOptions: Sendable, Equatable {\n    public var page: String?\n"), service)
        XCTAssertTrue(service.contains("public func list() async throws -> Page<ListBoxesResponseContent> {"), service)
        XCTAssertTrue(service.contains("        let operation = try client.operation(Routes.listBoxes, [])\n"), service)
        XCTAssertTrue(service.contains("public func get(boxId: Int, since: String, options: GetBoxOptions? = nil) async throws -> GetBoxResponseContent? {"), service)
        XCTAssertTrue(service.contains("        var operation = try client.operation(Routes.getBox, [boxId])\n"), service)
        XCTAssertTrue(service.contains("operation.resourceId(boxId)"), service)
        XCTAssertTrue(service.contains("operation.query(\"since\", since)"), service)
        XCTAssertTrue(service.contains("operation.queryOptional(\"page\", options?.page)"), service)
        XCTAssertTrue(service.contains("return try await client.sendOptional(operation)"), service)
        XCTAssertTrue(service.contains("public func markSeen(boxId: Int) async throws {"), service)
        XCTAssertTrue(service.contains("return try await client.sendVoid(operation)"), service)
        let workflows = try XCTUnwrap(files["Services/WorkflowsService.swift"])
        XCTAssertTrue(workflows.contains("public func getStage(workflowId: Int, stageId: Int32) async throws -> String {"), workflows)
        XCTAssertTrue(workflows.contains("operation.resourceId(Int(stageId))"), workflows)

        let routes = try XCTUnwrap(files["Routes.swift"])
        XCTAssertTrue(routes.contains("public static let listBoxes = Route("), routes)
        XCTAssertTrue(routes.contains("pagination: .link,"), routes)
        XCTAssertTrue(routes.contains("pagination: .unpaged,"), routes)
        XCTAssertTrue(routes.contains("pageParameter: \"page\","), routes)
        XCTAssertTrue(routes.contains("pageParameter: nil,"), routes)
        XCTAssertTrue(routes.contains("retry: RetryPolicy(max: 3, baseDelayMs: 1000, retryOn: [429, 503])"), routes)
        XCTAssertTrue(routes.contains("RouteParam(name: \"boxId\", role: .recording, kind: .int64),"), routes)
        XCTAssertTrue(routes.contains("html: true,"), routes)

        let accessors = try XCTUnwrap(files["ServiceAccessors.swift"])
        XCTAssertTrue(accessors.contains("public var boxes: BoxesService { BoxesService(client: self) }"), accessors)
        XCTAssertTrue(accessors.contains("public var workflows: WorkflowsService { WorkflowsService(client: self) }"), accessors)
    }

    func testTheCheckedInTreeIsWhatTheGeneratorWrites() throws {
        // The drift check, run against the repository this test is built from.
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let files = try generate(root: root)
        let problems = stale(files, at: root.appendingPathComponent(generatedDirectory))
        XCTAssertEqual(problems, [], "run `make swift-generate`")
    }
}
