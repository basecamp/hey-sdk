import Foundation

/// One thread on a workflow stage.
public struct WorkflowStageTopic: Codable, Sendable, Equatable {
    /// The staging record that puts the thread on this stage.
    public var stagingId: Int
    /// The thread.
    public var topicId: Int
    /// The thread's subject; empty when HEY names none.
    public var subject: String
    /// How many emails the thread holds (HEY counts its notes separately); zero when HEY does
    /// not say.
    public var entryCount: Int

    public init(stagingId: Int, topicId: Int, subject: String, entryCount: Int) {
        self.stagingId = stagingId
        self.topicId = topicId
        self.subject = subject
        self.entryCount = entryCount
    }

    enum CodingKeys: String, CodingKey {
        case stagingId = "staging_id"
        case topicId = "topic_id"
        case subject
        case entryCount = "entry_count"
    }
}

/// A workflow stage and the threads on it.
public struct WorkflowStageView: Codable, Sendable, Equatable {
    /// The stage.
    public var id: Int
    /// The stage's name; empty when HEY names none.
    public var name: String
    /// The threads on the stage, in the order the stage shows them.
    public var topics: [WorkflowStageTopic]

    public init(id: Int, name: String, topics: [WorkflowStageTopic]) {
        self.id = id
        self.name = name
        self.topics = topics
    }

    /// The stage as the generated ``WorkflowsService/getStage(workflowId:stageId:)`` answers
    /// it, reduced to the threads' ids, subjects and email counts. A negative count, which HEY
    /// does not send, reads as zero.
    public init(_ stage: WorkflowStageDetail) {
        self.init(
            id: stage.id,
            name: stage.name ?? "",
            topics: (stage.topics ?? []).map { topic in
                WorkflowStageTopic(
                    stagingId: topic.stagingId, topicId: topic.id, subject: topic.name ?? "",
                    entryCount: max(0, Int(topic.emailsCount ?? 0)))
            })
    }
}

/// A workflow as the autocomplete endpoint names it.
public struct WorkflowSummary: Codable, Sendable, Equatable {
    /// The workflow's id.
    public var id: Int
    /// What the workflow is called.
    public var name: String
    /// The account the workflow belongs to, empty when the row names none.
    public var accountName: String

    public init(id: Int, name: String, accountName: String) {
        self.id = id
        self.name = name
        self.accountName = accountName
    }

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case accountName = "account_name"
    }
}

/// The stage view and the form-backed writes on top of the generated surface (`get`,
/// `getStage`, `createStaging`, `moveStaging`). Workflows, their stages and the threads on a
/// stage are read as JSON; every write is a browser form post.
extension WorkflowsService {
    /// The workflows on an account. The autocomplete endpoint answers bare
    /// `[id, name, account name]` rows under its bare path, and answers 304 to a conditional
    /// request — the SDK sends none here, so this always comes back populated.
    public func list(accountId: Int) async throws -> [WorkflowSummary] {
        var operation = client.request(.get, "/autocompletable/accounts/\(accountId)/workflows")
        operation.info = OperationInfo(
            service: "Workflows", operation: "ListWorkflows", resourceType: "workflow", isMutation: false, resourceId: accountId)
        operation.withoutJSONSuffix()
        let rows: [[String]] = try await client.send(operation)
        return rows.compactMap(workflowSummary)
    }

    /// A workflow's stages, in position order.
    public func stages(workflowId: Int) async throws -> [WorkflowStage] {
        try await get(workflowId: workflowId).stages ?? []
    }

    /// One stage and the threads on it: what the generated ``getStage(workflowId:stageId:)``
    /// answers, as a ``WorkflowStageView``.
    public func stage(workflowId: Int, stageId: Int) async throws -> WorkflowStageView {
        WorkflowStageView(try await getStage(workflowId: workflowId, stageId: stageId))
    }

    /// Adds a workflow. No account — nil or a zero id — leaves HEY to pick your first.
    public func create(name: String, accountId: Int? = nil) async throws {
        var fields = [("workflow[name]", name)]
        if let accountId, accountId != 0 { fields.append(("account_id", String(accountId))) }
        var operation = client.form(.post, "/workflows")
        operation.info = writeInfo(service: "Workflows", operation: "CreateWorkflow", resourceType: "workflow")
        operation.form(fields)
        try await client.sendVoid(operation)
    }

    /// Renames a workflow.
    public func update(workflowId: Int, name: String) async throws {
        var operation = client.form(.patch, "/workflows/\(workflowId)")
        operation.info = writeInfo(service: "Workflows", operation: "UpdateWorkflow", resourceType: "workflow", resourceId: workflowId)
        operation.form([("workflow[name]", name)])
        try await client.sendVoid(operation)
    }

    /// Throws a workflow away.
    public func delete(workflowId: Int) async throws {
        var operation = client.form(.delete, "/workflows/\(workflowId)")
        operation.info = writeInfo(service: "Workflows", operation: "DeleteWorkflow", resourceType: "workflow", resourceId: workflowId)
        try await client.sendVoid(operation)
    }

    /// Adds a column to a workflow. HEY names it "Untitled"; rename it with
    /// ``updateStage(workflowId:stageId:name:)``.
    public func createStage(workflowId: Int) async throws {
        var operation = client.form(.post, "/workflows/\(workflowId)/stages")
        operation.info = writeInfo(
            service: "Workflows", operation: "CreateWorkflowStage", resourceType: "workflow_stage", resourceId: workflowId)
        operation.form([])
        try await client.sendVoid(operation)
    }

    /// Renames a workflow column.
    public func updateStage(workflowId: Int, stageId: Int, name: String) async throws {
        var operation = client.form(.patch, "/workflows/\(workflowId)/stages/\(stageId)")
        operation.info = writeInfo(
            service: "Workflows", operation: "UpdateWorkflowStage", resourceType: "workflow_stage", resourceId: stageId)
        operation.form([("workflow_stage[name]", name)])
        try await client.sendVoid(operation)
    }

    /// Removes a workflow column.
    public func deleteStage(workflowId: Int, stageId: Int) async throws {
        var operation = client.form(.delete, "/workflows/\(workflowId)/stages/\(stageId)")
        operation.info = writeInfo(
            service: "Workflows", operation: "DeleteWorkflowStage", resourceType: "workflow_stage", resourceId: stageId)
        try await client.sendVoid(operation)
    }

    /// Adds a topic to a workflow in the stage named. HEY creates the workflow membership before
    /// selecting the stage, so a failure to select it leaves the topic in the workflow's first
    /// stage; the generated ``createStaging(topicId:workflowId:)`` is the first of those two
    /// requests on its own. Both requests go quiet inside one operation, so the hooks hear
    /// `Workflows.CreateWorkflowStaging` once, as they do in Go and Rust, and hear it end only
    /// once the stage is selected — with the failure, when selecting it fails.
    public func stageTopic(topicId: Int, workflowId: Int, stageId: Int) async throws {
        let info = writeInfo(service: "Workflows", operation: "CreateWorkflowStaging", resourceType: "workflow_staging", resourceId: topicId)
        try await client.asOperation(info) {
            var operation = try client.operation(Routes.createWorkflowStaging, [topicId, workflowId])
            operation.formRepresentation()
            operation.quiet()
            try await client.sendVoid(operation)
            try await moveToStage(topicId: topicId, workflowId: workflowId, stageId: stageId, info: nil)
        }
    }

    /// Moves a staged topic to another stage of its workflow. The generated
    /// ``moveStaging(topicId:workflowId:body:)`` sends the same request as JSON, which HEY's own
    /// apps do not; this one sends the form they do.
    public func moveTopicToStage(topicId: Int, workflowId: Int, stageId: Int) async throws {
        let info = writeInfo(service: "Workflows", operation: "MoveWorkflowStaging", resourceType: "workflow_staging", resourceId: topicId)
        try await moveToStage(topicId: topicId, workflowId: workflowId, stageId: stageId, info: info)
    }

    /// Takes a topic back off a workflow.
    public func unstageTopic(topicId: Int, workflowId: Int) async throws {
        var operation = client.form(.delete, "/topics/\(topicId)/workflows/\(workflowId)/stagings")
        operation.info = writeInfo(
            service: "Workflows", operation: "DeleteWorkflowStaging", resourceType: "workflow_staging", resourceId: topicId)
        try await client.sendVoid(operation)
    }

    /// The stage selection ``stageTopic(topicId:workflowId:stageId:)`` and
    /// ``moveTopicToStage(topicId:workflowId:stageId:)`` share. What it announces itself as is
    /// the only difference, and no announcement at all is the staging case: there it is one
    /// request inside an operation already running.
    private func moveToStage(topicId: Int, workflowId: Int, stageId: Int, info: OperationInfo?) async throws {
        var operation = try client.operation(Routes.moveWorkflowStaging, [topicId, workflowId])
        operation.formRepresentation()
        operation.form([("workflow_staging[workflow_stage_id]", String(stageId))])
        if let info { operation.info = info } else { operation.quiet() }
        try await client.sendVoid(operation)
    }
}

/// The workflow a row names. A row too short to carry a name, or whose first column is no id, is
/// one the autocomplete list has nothing to say about.
private func workflowSummary(_ row: [String]) -> WorkflowSummary? {
    guard row.count >= 2, let id = Int(row[0]) else { return nil }
    return WorkflowSummary(id: id, name: row[1], accountName: row.count > 2 ? row[2] : "")
}
