package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.WorkflowStage
import com.basecamp.hey.internal.HtmlNode
import com.basecamp.hey.internal.parseHtml
import com.basecamp.hey.writeInfo
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import com.basecamp.hey.generated.services.WorkflowsService as GeneratedWorkflowsService

/** One thread on a workflow stage, as the stage page renders its card. */
@Serializable
data class WorkflowStageTopic(
    /** The staging record that puts the thread on this stage, which is what `WorkflowsService.moveStaging` moves. */
    @SerialName("staging_id") val stagingId: Long,
    /** The thread the card is for. */
    @SerialName("topic_id") val topicId: Long,
    /** The card's title; empty when the card renders none. */
    val subject: String,
    /** How many emails the card says the thread holds; zero when the card does not say. */
    @SerialName("entry_count") val entryCount: Long,
)

/**
 * A workflow stage as HEY renders it — the stage page is the only place a stage's threads
 * are listed — with the cards it shows.
 */
@Serializable
data class WorkflowStageView(
    /** The stage, as the caller asked for it. */
    val id: Long,
    /** The stage's name as the page shows it; empty when the page names none. */
    val name: String,
    /** The cards on the stage, in the order the page shows them. */
    val topics: List<WorkflowStageTopic>,
) {
    companion object {
        /**
         * Reads the stage out of the page HEY serves for it, the way Go and Rust do: the
         * element whose id names the stage, the first `h2` at or under it for the name, and
         * every outermost element under it whose id is `topic_<id>` for a card. A card is
         * skipped when its thread or staging id will not parse as a positive number, or when
         * its detail line does not start with a count. Text meant for screen readers is left
         * out of names and subjects.
         */
        fun parse(html: String, stageId: Long): WorkflowStageView {
            val document = parseHtml(html)
            val stage = document.descendants().firstOrNull { it.attribute("id") == "container_workflow_stage_$stageId" }
                ?: throw HeyException.NotFound("workflow stage not found: $stageId")
            val name = stage.firstAtOrUnder { it.tag == "h2" }?.visibleText().orEmpty()
            // Outermost cards only, in document order, found without recursion: the page's
            // nesting is the server's to decide.
            val topics = mutableListOf<WorkflowStageTopic>()
            val pending = ArrayDeque<HtmlNode.Element>()
            stage.children.asReversed().filterIsInstance<HtmlNode.Element>().forEach(pending::addLast)
            while (pending.isNotEmpty()) {
                val element = pending.removeLast()
                if (element.attribute("id")?.startsWith("topic_") == true) {
                    topic(element)?.let { topics += it }
                } else {
                    element.children.asReversed().filterIsInstance<HtmlNode.Element>().forEach(pending::addLast)
                }
            }
            return WorkflowStageView(stageId, name, topics)
        }

        private fun topic(card: HtmlNode.Element): WorkflowStageTopic? {
            val topicId = positive(card.attribute("id")?.removePrefix("topic_")) ?: return null
            val stagingId = positive(card.attribute("data-identifier")) ?: return null
            val subject = card.firstAtOrUnder { it.tag == "h3" }?.visibleText().orEmpty()
            val detail = card.firstAtOrUnder { it.tag == "p" && it.attribute("class")?.contains("card__detail") == true }
            val entryCount = when (detail) {
                null -> 0L
                else -> detail.visibleText().split(' ').firstOrNull()?.toLongOrNull()?.takeIf { it >= 0 } ?: return null
            }
            return WorkflowStageTopic(stagingId = stagingId, topicId = topicId, subject = subject, entryCount = entryCount)
        }

        private fun positive(value: String?): Long? = value?.toLongOrNull()?.takeIf { it > 0 }
    }
}

/** A workflow as the autocomplete endpoint names it. */
@Serializable
data class WorkflowSummary(
    /** The workflow's id. */
    val id: Long,
    /** What the workflow is called. */
    val name: String,
    /** The account the workflow belongs to, empty when the row names none. */
    @SerialName("account_name") val accountName: String,
)

/**
 * Workflows service with the stage page reader and the form-backed writes on top of the
 * generated surface (`get`, `getStage`, `createStaging`, `moveStaging`). A workflow has no
 * JSON surface beyond the page that reads one and the autocomplete endpoint that enumerates
 * them, so every write is a browser form post.
 */
class WorkflowsService(client: HeyClient) : GeneratedWorkflowsService(client) {
    /**
     * The workflows on an account. The autocomplete endpoint answers bare
     * `[id, name, account name]` rows under its bare path, and answers 304 to a conditional
     * request — the SDK sends none here, so this always comes back populated.
     */
    suspend fun list(accountId: Long): List<WorkflowSummary> {
        val operation = client.request(Method.GET, "/autocompletable/accounts/$accountId/workflows")
        operation.info(OperationInfo(service = "Workflows", operation = "ListWorkflows", resourceType = "workflow", isMutation = false, resourceId = accountId))
        operation.withoutJsonSuffix()
        val rows: List<List<String>> = client.send(operation)
        return rows.mapNotNull(::summary)
    }

    /** A workflow's stages, in position order. */
    suspend fun stages(workflowId: Long): List<WorkflowStage> = get(workflowId).stages.orEmpty()

    /**
     * Reads a stage's page and the cards on it. The generated [getStage] answers the page
     * itself; this sends the same route and parses inside the operation, so a page without
     * the stage ends the operation the hooks hear with the error the caller gets.
     */
    suspend fun stage(workflowId: Long, stageId: Long): WorkflowStageView {
        val operation = client.operation(Routes.GET_WORKFLOW_STAGE, listOf(workflowId, stageId))
        operation.resourceId(stageId)
        return client.execute(operation) { WorkflowStageView.parse(it.text(), stageId) }
    }

    /** Adds a workflow. No account — null or a zero id — leaves HEY to pick your first. */
    suspend fun create(name: String, accountId: Long? = null) {
        val fields = mutableListOf("workflow[name]" to name)
        accountId?.takeIf { it != 0L }?.let { fields += "account_id" to it.toString() }
        val operation = client.form(Method.POST, "/workflows")
        operation.info(writeInfo("Workflows", "CreateWorkflow", "workflow"))
        operation.form(fields)
        client.sendUnit(operation)
    }

    /** Renames a workflow. */
    suspend fun update(workflowId: Long, name: String) {
        val operation = client.form(Method.PATCH, "/workflows/$workflowId")
        operation.info(writeInfo("Workflows", "UpdateWorkflow", "workflow", workflowId))
        operation.form(listOf("workflow[name]" to name))
        client.sendUnit(operation)
    }

    /** Throws a workflow away. */
    suspend fun delete(workflowId: Long) {
        val operation = client.form(Method.DELETE, "/workflows/$workflowId")
        operation.info(writeInfo("Workflows", "DeleteWorkflow", "workflow", workflowId))
        client.sendUnit(operation)
    }

    /** Adds a column to a workflow. HEY names it "Untitled"; rename it with [updateStage]. */
    suspend fun createStage(workflowId: Long) {
        val operation = client.form(Method.POST, "/workflows/$workflowId/stages")
        operation.info(writeInfo("Workflows", "CreateWorkflowStage", "workflow_stage", workflowId))
        operation.form(emptyList())
        client.sendUnit(operation)
    }

    /** Renames a workflow column. */
    suspend fun updateStage(workflowId: Long, stageId: Long, name: String) {
        val operation = client.form(Method.PATCH, "/workflows/$workflowId/stages/$stageId")
        operation.info(writeInfo("Workflows", "UpdateWorkflowStage", "workflow_stage", stageId))
        operation.form(listOf("workflow_stage[name]" to name))
        client.sendUnit(operation)
    }

    /** Removes a workflow column. */
    suspend fun deleteStage(workflowId: Long, stageId: Long) {
        val operation = client.form(Method.DELETE, "/workflows/$workflowId/stages/$stageId")
        operation.info(writeInfo("Workflows", "DeleteWorkflowStage", "workflow_stage", stageId))
        client.sendUnit(operation)
    }

    /**
     * Adds a topic to a workflow in the stage named. HEY creates the workflow membership
     * before selecting the stage, so a failure to select it leaves the topic in the
     * workflow's first stage; the generated [createStaging] is the first of those two
     * requests on its own. The stage selection is a quiet send, so the hooks hear
     * `Workflows.CreateWorkflowStaging` once, as they do in Go and Rust.
     */
    suspend fun stageTopic(topicId: Long, workflowId: Long, stageId: Long) {
        val operation = client.operation(Routes.CREATE_WORKFLOW_STAGING, listOf(topicId, workflowId))
        operation.info(writeInfo("Workflows", "CreateWorkflowStaging", "workflow_staging", topicId))
        operation.formRepresentation()
        client.sendUnit(operation)
        moveToStage(topicId, workflowId, stageId, null)
    }

    /**
     * Moves a staged topic to another stage of its workflow. The generated [moveStaging]
     * sends the same request as JSON, which HEY's own apps do not; this one sends the form
     * they do.
     */
    suspend fun moveTopicToStage(topicId: Long, workflowId: Long, stageId: Long) {
        moveToStage(topicId, workflowId, stageId, writeInfo("Workflows", "MoveWorkflowStaging", "workflow_staging", topicId))
    }

    /** Takes a topic back off a workflow. */
    suspend fun unstageTopic(topicId: Long, workflowId: Long) {
        val operation = client.form(Method.DELETE, "/topics/$topicId/workflows/$workflowId/stagings")
        operation.info(writeInfo("Workflows", "DeleteWorkflowStaging", "workflow_staging", topicId))
        client.sendUnit(operation)
    }

    /**
     * The stage selection [stageTopic] and [moveTopicToStage] share. What it announces
     * itself as is the only difference, and no announcement at all is the staging case:
     * there it is one request inside an operation already running.
     */
    private suspend fun moveToStage(topicId: Long, workflowId: Long, stageId: Long, info: OperationInfo?) {
        val operation = client.operation(Routes.MOVE_WORKFLOW_STAGING, listOf(topicId, workflowId))
        operation.formRepresentation()
        operation.form(listOf("workflow_staging[workflow_stage_id]" to stageId.toString()))
        if (info != null) operation.info(info) else operation.quiet()
        client.sendUnit(operation)
    }
}

/**
 * The workflow a row names. A row too short to carry a name, or whose first column is no
 * id, is one the autocomplete list has nothing to say about.
 */
private fun summary(row: List<String>): WorkflowSummary? {
    if (row.size < 2) return null
    val id = row[0].toLongOrNull() ?: return null
    return WorkflowSummary(id = id, name = row[1], accountName = row.getOrElse(2) { "" })
}
