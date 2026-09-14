package com.basecamp.hey.services

import com.basecamp.hey.HeyClient
import com.basecamp.hey.HeyException
import com.basecamp.hey.internal.HtmlNode
import com.basecamp.hey.internal.parseHtml
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

/** Workflows service with the stage page reader on top of the generated surface (`get`, `getStage`, `createStaging`, `moveStaging`). */
class WorkflowsService(client: HeyClient) : GeneratedWorkflowsService(client) {
    /** Reads a stage's page and the cards on it. The generated [getStage] answers the page itself. */
    suspend fun stage(workflowId: Long, stageId: Long): WorkflowStageView =
        WorkflowStageView.parse(getStage(workflowId, stageId), stageId)
}
