import Foundation
import XCTest

@testable import Hey

final class WorkflowsServiceTests: XCTestCase {
    private let stagings = "/topics/4471829/workflows/8801/stagings"

    func testTheListReadsTheRowsTheAutocompleteEndpointAnswersFromTheBarePath() async throws {
        let hey = mockHey(ok(#"[["8801","Hiring","Example Co"],["8802","Sales pipeline"],["not-an-id","Ignored","Example Co"],["8804"],[]]"#))
        let log = OperationLog()
        let workflows = try await hey.client(hooks: log).workflows.list(accountId: 77)
        XCTAssertEqual(workflows, [
            WorkflowSummary(id: 8801, name: "Hiring", accountName: "Example Co"),
            WorkflowSummary(id: 8802, name: "Sales pipeline", accountName: ""),
        ])
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "GET")
        XCTAssertEqual(request.path, "/autocompletable/accounts/77/workflows", "the autocomplete endpoint answers JSON only under its bare path")
        XCTAssertEqual(request.header("Accept"), "application/json")
        XCTAssertEqual(log.started, ["Workflows.ListWorkflows:workflow:false:77"])
    }

    func testTheStagesComeOffTheWorkflowItself() async throws {
        let hey = mockHey(ok(#"{"id":8801,"name":"Hiring","stages":[{"id":5512,"name":"Applied"},{"id":5513}]}"#), ok(#"{"id":8801}"#))
        let client = try hey.client()
        let stages = try await client.workflows.stages(workflowId: 8801)
        XCTAssertEqual(stages.map(\.id), [5512, 5513])
        XCTAssertEqual(hey.requests[0].path, "/workflows/8801.json")
        let none = try await client.workflows.stages(workflowId: 8801)
        XCTAssertEqual(none, [], "a workflow without stages has none rather than a missing list")
    }

    func testAWorkflowIsMadeOnTheAccountItNamesOrOnTheOneHeyPicks() async throws {
        let redirect = status(302, nil, [("Location", "/workflows")])
        let hey = mockHey(redirect, redirect, redirect)
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.workflows.create(name: "Hiring", accountId: 77)
        try await client.workflows.create(name: "Hiring", accountId: 0)
        try await client.workflows.create(name: "Hiring")
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "POST")
        XCTAssertEqual(request.path, "/workflows")
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)
        XCTAssertEqual(fields(request.body), ["workflow[name]=Hiring", "account_id=77"])
        XCTAssertEqual(fields(hey.requests[1].body), ["workflow[name]=Hiring"], "a zero account id is no account")
        XCTAssertEqual(fields(hey.requests[2].body), ["workflow[name]=Hiring"])
        XCTAssertEqual(log.started[0], "Workflows.CreateWorkflow:workflow:true:nil")
    }

    func testRenamingAndThrowingAwayAWorkflow() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/workflows/8801")]), status(302, nil, [("Location", "/workflows")]))
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.workflows.update(workflowId: 8801, name: "Recruiting")
        try await client.workflows.delete(workflowId: 8801)
        XCTAssertEqual(hey.requests[0].method, "PATCH")
        XCTAssertEqual(hey.requests[0].path, "/workflows/8801")
        XCTAssertEqual(fields(hey.requests[0].body), ["workflow[name]=Recruiting"])
        XCTAssertEqual(hey.requests[1].method, "DELETE")
        XCTAssertEqual(hey.requests[1].path, "/workflows/8801")
        XCTAssertEqual(hey.requests[1].body, "")
        XCTAssertEqual(log.started, ["Workflows.UpdateWorkflow:workflow:true:8801", "Workflows.DeleteWorkflow:workflow:true:8801"])
    }

    func testAStageIsMadeWithAnEmptyFormThenRenamedAndRemoved() async throws {
        let redirect = status(302, nil, [("Location", "/workflows/8801")])
        let hey = mockHey(redirect, redirect, redirect)
        let log = OperationLog()
        let client = try hey.client(hooks: log)
        try await client.workflows.createStage(workflowId: 8801)
        try await client.workflows.updateStage(workflowId: 8801, stageId: 5512, name: "Phone screen")
        try await client.workflows.deleteStage(workflowId: 8801, stageId: 5512)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, "/workflows/8801/stages")
        XCTAssertEqual(hey.requests[0].header("Content-Type"), "application/x-www-form-urlencoded", "an empty form is still a form")
        XCTAssertEqual(hey.requests[0].body, "")
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, "/workflows/8801/stages/5512")
        XCTAssertEqual(fields(hey.requests[1].body), ["workflow_stage[name]=Phone screen"])
        XCTAssertEqual(hey.requests[2].method, "DELETE")
        XCTAssertEqual(hey.requests[2].path, "/workflows/8801/stages/5512")
        XCTAssertEqual(log.started, [
            "Workflows.CreateWorkflowStage:workflow_stage:true:8801",
            "Workflows.UpdateWorkflowStage:workflow_stage:true:5512",
            "Workflows.DeleteWorkflowStage:workflow_stage:true:5512",
        ])
    }

    func testStagingATopicFilesItThenMovesItToTheStageAskedForAsOneOperation() async throws {
        let hey = mockHey(status(204), status(204))
        let log = OperationLog()
        try await hey.client(hooks: log).workflows.stageTopic(topicId: 4471829, workflowId: 8801, stageId: 5512)
        XCTAssertEqual(
            log.started, ["Workflows.CreateWorkflowStaging:workflow_staging:true:4471829"],
            "the stage selection is quiet, so this is one operation however many requests it takes")
        XCTAssertEqual(hey.requests.count, 2)
        XCTAssertEqual(hey.requests[0].method, "POST")
        XCTAssertEqual(hey.requests[0].path, stagings)
        XCTAssertEqual(hey.requests[0].header("Accept"), browserAcceptHeader)
        XCTAssertEqual(hey.requests[0].body, "")
        XCTAssertEqual(hey.requests[1].method, "PATCH")
        XCTAssertEqual(hey.requests[1].path, stagings)
        XCTAssertEqual(fields(hey.requests[1].body), ["workflow_staging[workflow_stage_id]=5512"])
    }

    func testAStageThatWillNotTakeTheTopicSurfacesAfterTheTopicIsFiled() async throws {
        let hey = mockHey(status(204), status(422, #"{"error":"stage is not part of this workflow"}"#))
        let refused = await assertThrowsHeyError(try await hey.client().workflows.stageTopic(topicId: 4471829, workflowId: 8801, stageId: 5512))
        XCTAssertEqual(refused?.httpStatus, 422)
        XCTAssertEqual(hey.requests.count, 2, "the topic was filed before the stage was refused")
    }

    func testMovingAStagedTopicSendsTheStageAsAForm() async throws {
        let hey = mockHey(status(204))
        let log = OperationLog()
        try await hey.client(hooks: log).workflows.moveTopicToStage(topicId: 4471829, workflowId: 8801, stageId: 5512)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "PATCH")
        XCTAssertEqual(request.path, stagings)
        XCTAssertEqual(request.header("Accept"), browserAcceptHeader)
        XCTAssertEqual(request.header("Content-Type"), "application/x-www-form-urlencoded")
        XCTAssertEqual(fields(request.body), ["workflow_staging[workflow_stage_id]=5512"])
        XCTAssertEqual(log.started, ["Workflows.MoveWorkflowStaging:workflow_staging:true:4471829"])
    }

    func testUnstagingATopicTakesItOffTheWorkflow() async throws {
        let hey = mockHey(status(302, nil, [("Location", "/topics/4471829")]))
        let log = OperationLog()
        try await hey.client(hooks: log).workflows.unstageTopic(topicId: 4471829, workflowId: 8801)
        XCTAssertEqual(hey.requests.count, 1)
        let request = hey.requests[0]
        XCTAssertEqual(request.method, "DELETE")
        XCTAssertEqual(request.path, stagings)
        XCTAssertEqual(request.body, "")
        XCTAssertEqual(log.started, ["Workflows.DeleteWorkflowStaging:workflow_staging:true:4471829"])
    }

    func testAStagePageWithoutTheStageEndsTheOperationWithThatError() async throws {
        let hey = mockHey(ok(#"<section id="container_workflow_stage_1"></section>"#, [("Content-Type", "text/html")]))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        await assertThrows(HeyError.codeNotFound, try await client.workflows.stage(workflowId: 8801, stageId: 5))
        XCTAssertEqual(transcript.log, [
            "start:Workflows.GetWorkflowStage:workflow_stage:false:5",
            "request:GET:1",
            "response:200:nil",
            "end:GetWorkflowStage:not_found",
        ])
    }

    func testAStagingOfTwoRequestsEndsOnceBothAreInWithTheErrorTheCallerGets() async throws {
        let hey = mockHey(ok(""), status(422, #"{"error":"no such stage"}"#))
        let transcript = Transcript()
        let client = try hey.client(hooks: transcript)
        await assertThrows(HeyError.codeValidation, try await client.workflows.stageTopic(topicId: 5, workflowId: 8801, stageId: 99))
        XCTAssertEqual(transcript.log, [
            "start:Workflows.CreateWorkflowStaging:workflow_staging:true:5",
            "request:POST:1",
            "response:200:nil",
            "request:PATCH:1",
            "response:422:validation",
            "end:CreateWorkflowStaging:validation",
        ])
    }
}

/// A form body's pairs as `name=value` lines, in order, so they compare.
private func fields(_ body: String) -> [String] {
    formPairs(body).map { "\($0.0)=\($0.1)" }
}
