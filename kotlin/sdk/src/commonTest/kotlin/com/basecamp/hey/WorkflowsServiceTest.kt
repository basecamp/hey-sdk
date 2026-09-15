package com.basecamp.hey

import com.basecamp.hey.generated.workflows
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class WorkflowsServiceTest {
    private val stagings = "/topics/4471829/workflows/8801/stagings"

    @Test
    fun theListReadsTheRowsTheAutocompleteEndpointAnswersFromTheBarePath() = runTest {
        val hey = mockHey(ok("""[["8801","Hiring","Example Co"],["8802","Sales pipeline"],["not-an-id","Ignored","Example Co"],["8804"],[]]"""))
        val log = OperationLog()
        val workflows = hey.client { hooks = log }.workflows.list(77)
        assertEquals(listOf(Triple(8801L, "Hiring", "Example Co"), Triple(8802L, "Sales pipeline", "")), workflows.map { Triple(it.id, it.name, it.accountName) })
        val request = hey.requests.single()
        assertEquals("GET", request.method)
        assertEquals("/autocompletable/accounts/77/workflows", request.path, "the autocomplete endpoint answers JSON only under its bare path")
        assertEquals("application/json", request.header("Accept"))
        assertEquals(listOf("Workflows.ListWorkflows:workflow:false:77"), log.started)
    }

    @Test
    fun theStagesComeOffTheWorkflowItself() = runTest {
        val hey = mockHey(ok("""{"id":8801,"name":"Hiring","stages":[{"id":5512,"name":"Applied"},{"id":5513}]}"""), ok("""{"id":8801}"""))
        val client = hey.client()
        assertEquals(listOf(5512L, 5513L), client.workflows.stages(8801).map { it.id })
        assertEquals("/workflows/8801.json", hey.requests[0].path)
        assertEquals(emptyList(), client.workflows.stages(8801), "a workflow without stages has none rather than a missing list")
    }

    @Test
    fun aWorkflowIsMadeOnTheAccountItNamesOrOnTheOneHeyPicks() = runTest {
        val redirect = status(302, headers = mapOf("Location" to "/workflows"))
        val hey = mockHey(redirect, redirect, redirect)
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.workflows.create("Hiring", 77)
        client.workflows.create("Hiring", 0)
        client.workflows.create("Hiring")
        val request = hey.requests[0]
        assertEquals("POST", request.method)
        assertEquals("/workflows", request.path)
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        assertEquals(BROWSER_ACCEPT_HEADER, request.header("Accept"))
        assertEquals(listOf("workflow[name]" to "Hiring", "account_id" to "77"), formPairs(request.body))
        assertEquals(listOf("workflow[name]" to "Hiring"), formPairs(hey.requests[1].body), "a zero account id is no account")
        assertEquals(listOf("workflow[name]" to "Hiring"), formPairs(hey.requests[2].body))
        assertEquals("Workflows.CreateWorkflow:workflow:true:null", log.started[0])
    }

    @Test
    fun renamingAndThrowingAwayAWorkflow() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/workflows/8801")), status(302, headers = mapOf("Location" to "/workflows")))
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.workflows.update(8801, "Recruiting")
        client.workflows.delete(8801)
        assertEquals("PATCH", hey.requests[0].method)
        assertEquals("/workflows/8801", hey.requests[0].path)
        assertEquals(listOf("workflow[name]" to "Recruiting"), formPairs(hey.requests[0].body))
        assertEquals("DELETE", hey.requests[1].method)
        assertEquals("/workflows/8801", hey.requests[1].path)
        assertEquals("", hey.requests[1].body)
        assertEquals(listOf("Workflows.UpdateWorkflow:workflow:true:8801", "Workflows.DeleteWorkflow:workflow:true:8801"), log.started)
    }

    @Test
    fun aStageIsMadeWithAnEmptyFormThenRenamedAndRemoved() = runTest {
        val redirect = status(302, headers = mapOf("Location" to "/workflows/8801"))
        val hey = mockHey(redirect, redirect, redirect)
        val log = OperationLog()
        val client = hey.client { hooks = log }
        client.workflows.createStage(8801)
        client.workflows.updateStage(8801, 5512, "Phone screen")
        client.workflows.deleteStage(8801, 5512)
        assertEquals("POST", hey.requests[0].method)
        assertEquals("/workflows/8801/stages", hey.requests[0].path)
        assertEquals("application/x-www-form-urlencoded", hey.requests[0].header("Content-Type"), "an empty form is still a form")
        assertEquals("", hey.requests[0].body)
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals("/workflows/8801/stages/5512", hey.requests[1].path)
        assertEquals(listOf("workflow_stage[name]" to "Phone screen"), formPairs(hey.requests[1].body))
        assertEquals("DELETE", hey.requests[2].method)
        assertEquals("/workflows/8801/stages/5512", hey.requests[2].path)
        assertEquals(
            listOf(
                "Workflows.CreateWorkflowStage:workflow_stage:true:8801",
                "Workflows.UpdateWorkflowStage:workflow_stage:true:5512",
                "Workflows.DeleteWorkflowStage:workflow_stage:true:5512",
            ),
            log.started,
        )
    }

    @Test
    fun stagingATopicFilesItThenMovesItToTheStageAskedForAsOneOperation() = runTest {
        val hey = mockHey(status(204), status(204))
        val log = OperationLog()
        hey.client { hooks = log }.workflows.stageTopic(4471829, 8801, 5512)
        assertEquals(listOf("Workflows.CreateWorkflowStaging:workflow_staging:true:4471829"), log.started, "the stage selection is quiet, so this is one operation however many requests it takes")
        assertEquals(2, hey.requests.size)
        assertEquals("POST", hey.requests[0].method)
        assertEquals(stagings, hey.requests[0].path)
        assertEquals(BROWSER_ACCEPT_HEADER, hey.requests[0].header("Accept"))
        assertEquals("", hey.requests[0].body)
        assertEquals("PATCH", hey.requests[1].method)
        assertEquals(stagings, hey.requests[1].path)
        assertEquals(listOf("workflow_staging[workflow_stage_id]" to "5512"), formPairs(hey.requests[1].body))
    }

    @Test
    fun aStageThatWillNotTakeTheTopicSurfacesAfterTheTopicIsFiled() = runTest {
        val hey = mockHey(status(204), status(422, """{"error":"stage is not part of this workflow"}"""))
        val refused = assertFailsWith<HeyException> { hey.client().workflows.stageTopic(4471829, 8801, 5512) }
        assertEquals(422, refused.httpStatus)
        assertEquals(2, hey.requests.size, "the topic was filed before the stage was refused")
    }

    @Test
    fun movingAStagedTopicSendsTheStageAsAForm() = runTest {
        val hey = mockHey(status(204))
        val log = OperationLog()
        hey.client { hooks = log }.workflows.moveTopicToStage(4471829, 8801, 5512)
        val request = hey.requests.single()
        assertEquals("PATCH", request.method)
        assertEquals(stagings, request.path)
        assertEquals(BROWSER_ACCEPT_HEADER, request.header("Accept"))
        assertEquals("application/x-www-form-urlencoded", request.header("Content-Type"))
        assertEquals(listOf("workflow_staging[workflow_stage_id]" to "5512"), formPairs(request.body))
        assertEquals(listOf("Workflows.MoveWorkflowStaging:workflow_staging:true:4471829"), log.started)
    }

    @Test
    fun unstagingATopicTakesItOffTheWorkflow() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/topics/4471829")))
        val log = OperationLog()
        hey.client { hooks = log }.workflows.unstageTopic(4471829, 8801)
        val request = hey.requests.single()
        assertEquals("DELETE", request.method)
        assertEquals(stagings, request.path)
        assertEquals("", request.body)
        assertEquals(listOf("Workflows.DeleteWorkflowStaging:workflow_staging:true:4471829"), log.started)
    }
}
