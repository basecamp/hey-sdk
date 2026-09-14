package com.basecamp.hey

import com.basecamp.hey.services.WorkflowStageView
import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class WorkflowStageTest {
    private val page = """<section id="container_workflow_stage_5512"><h2>Applied<span class="u-for-screen-reader"> stage</span></h2>""" +
        """<div class="workflow__card" id="topic_4471829" data-identifier="91"><h3>Application<span class="u-for-screen-reader">Thread: Application</span></h3>""" +
        """<p class="card__detail"><span class="u-for-screen-reader">,</span>3 emails</p></div>""" +
        """<div class="workflow__card" id="topic_4471830" data-identifier="92"><div id="topic_1" data-identifier="93"><h3>Inner</h3></div></div>""" +
        """<div id="topic_not-a-number" data-identifier="93"></div>""" +
        """<div id="topic_5" data-identifier="94"><p class="card__detail">many emails</p></div></section>"""

    @Test
    fun theStageIsReadOutOfThePage() {
        val view = WorkflowStageView.parse(page, 5512)
        assertEquals(5512L, view.id)
        assertEquals("Applied", view.name)
        assertEquals(2, view.topics.size)
        assertEquals(4471829L, view.topics[0].topicId)
        assertEquals(91L, view.topics[0].stagingId)
        assertEquals("Application", view.topics[0].subject)
        assertEquals(3L, view.topics[0].entryCount)
        assertEquals(4471830L, view.topics[1].topicId)
        assertEquals("Inner", view.topics[1].subject, "a card inside a card is that card's content")
        assertEquals(0L, view.topics[1].entryCount)
    }

    @Test
    fun aPageWithoutTheStageIsNotFound() {
        assertFailsWith<HeyException.NotFound> { WorkflowStageView.parse("<html></html>", 1) }
    }

    @Test
    fun theServiceReadsThePageAsHtml() = runTest {
        val hey = mockHey(ok(page, mapOf("Content-Type" to "text/html; charset=utf-8")))
        val view = hey.client().workflows.stage(8801, 5512)
        assertEquals("Applied", view.name)
        assertEquals("/workflows/8801/stages/5512", hey.requests.single().path)
    }

    @Test
    fun aPageNestedBeyondAnyStackIsStillRead() {
        val depth = 200_000
        val page = buildString {
            append("<section id=\"container_workflow_stage_5\"><h2>Applied</h2>")
            repeat(depth) { append("<div>") }
            append("<div id=\"topic_9\" data-identifier=\"77\"><h3>Deep</h3><p class=\"card__detail\">2 messages</p></div>")
            repeat(depth) { append("</div>") }
            append("</section>")
        }
        val view = WorkflowStageView.parse(page, 5)
        assertEquals("Applied", view.name)
        assertEquals(listOf(9L), view.topics.map { it.topicId })
        assertEquals("Deep", view.topics.single().subject)
    }
}
