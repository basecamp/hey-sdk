package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.Method
import com.basecamp.hey.OperationInfo
import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.WorkflowStage
import com.basecamp.hey.generated.models.WorkflowStageDetail
import com.basecamp.hey.writeInfo
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import com.basecamp.hey.generated.services.WorkflowsService as GeneratedWorkflowsService

/** One thread on a workflow stage. */
@Serializable
data class WorkflowStageTopic(
    /** The staging record that puts the thread on this stage. */
    @SerialName("staging_id") val stagingId: Long,
    /** The thread. */
    @SerialName("topic_id") val topicId: Long,
    /** The thread's subject; empty when HEY names none. */
    val subject: String,
    /** How many emails the thread holds (HEY counts its notes separately); zero when HEY does not say. */
    @SerialName("entry_count") val entryCount: Long,
)

/** A workflow stage and the threads on it. */
@Serializable
data class WorkflowStageView(
    /** The stage. */
    val id: Long,
    /** The stage's name; empty when HEY names none. */
    val name: String,
    /** The threads on the stage, in the order the stage shows them. */
    val topics: List<WorkflowStageTopic>,
) {
    companion object {
        /**
         * The stage as the generated [GeneratedWorkflowsService.getStage] answers it, reduced to
         * the threads' ids, subjects and email counts. A negative count, which HEY does not
         * send, reads as zero.
         */
        fun from(stage: WorkflowStageDetail): WorkflowStageView = WorkflowStageView(
            id = stage.id,
            name = stage.name.orEmpty(),
            topics = stage.topics.orEmpty().map { topic ->
                WorkflowStageTopic(
                    stagingId = topic.stagingId,
                    topicId = topic.id,
                    subject = topic.name.orEmpty(),
                    entryCount = (topic.emailsCount ?: 0).coerceAtLeast(0).toLong(),
                )
            },
        )
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
 * Workflows service with the stage view and the form-backed writes on top of the generated
 * surface (`get`, `getStage`, `createStaging`, `moveStaging`). Workflows, their stages and the
 * threads on a stage are read as JSON; every write is a browser form post.
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

    /** One stage and the threads on it: what the generated [getStage] answers, as a [WorkflowStageView]. */
    suspend fun stage(workflowId: Long, stageId: Long): WorkflowStageView = WorkflowStageView.from(getStage(workflowId, stageId))

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
     * requests on its own. Both requests go quiet inside one operation, so the hooks hear
     * `Workflows.CreateWorkflowStaging` once, as they do in Go and Rust, and hear it end
     * only once the stage is selected — with the failure, when selecting it fails.
     */
    suspend fun stageTopic(topicId: Long, workflowId: Long, stageId: Long) {
        client.asOperation(writeInfo("Workflows", "CreateWorkflowStaging", "workflow_staging", topicId)) {
            val operation = client.operation(Routes.CREATE_WORKFLOW_STAGING, listOf(topicId, workflowId))
            operation.formRepresentation()
            operation.quiet()
            client.sendUnit(operation)
            moveToStage(topicId, workflowId, stageId, null)
        }
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
