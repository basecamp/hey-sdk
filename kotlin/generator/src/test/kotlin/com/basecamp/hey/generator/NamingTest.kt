package com.basecamp.hey.generator

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class NamingTest {
    private val naming = Naming()

    @Test
    fun methodsDropTheServiceNoun() {
        assertEquals("list", naming.methodFor("ListBoxes", "boxes"))
        assertEquals("getBoxChanges", naming.methodFor("GetBoxPostingChanges", "postings"))
        assertEquals("getEntries", naming.methodFor("GetTopicEntries", "topics"))
        assertEquals("listCategories", naming.methodFor("ListTimeTrackCategories", "time_tracks"))
        assertEquals("getOngoing", naming.methodFor("GetOngoingTimeTrack", "time_tracks"))
        assertEquals("create", naming.methodFor("CreateSticky", "stickies"))
        assertEquals("updateClearance", naming.methodFor("UpdateContactClearance", "contacts"))
        assertEquals("updateMy", naming.methodFor("UpdateMyClearance", "clearances"))
    }

    @Test
    fun aKeywordOrAnEmptyNameIsRefused() {
        assertFailsWith<GeneratorException> { naming.methodFor("Boxes", "boxes") }
        assertFailsWith<GeneratorException> { naming.methodFor("InTopic", "topics") }
    }

    @Test
    fun overridesWin() {
        val naming = Naming.parse(
            """
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
            """.trimIndent(),
        )
        assertEquals("bulk_replies", naming.serviceFor("NewBulkReply", "Bulk Reply"))
        assertEquals("clearances", naming.serviceFor("GetClearances", "Contacts"))
        assertEquals("calendar_periods", naming.serviceFor("ListCalendarDays", "Calendar Periods"))
        assertEquals("moveTopic", naming.methodFor("MoveTopic", "topics"))
        assertEquals("Mailbox", naming.typeFor("Box"))
        assertEquals("BoxGroup", naming.typeFor("BoxGroup"))
        assertEquals("box", naming.resourceTypeFor("ListBoxes", "boxes"))
        assertEquals("box_group", naming.resourceTypeFor("CreateBoxGroup", "boxes"))
        assertFailsWith<GeneratorException> { naming.resourceTypeFor("ListTopics", "topics") }
    }

    @Test
    fun identifiersEscapeKeywordsAndBrackets() {
        assertEquals("`object`", fieldIdent("object"))
        assertEquals("boxId", fieldIdent("box_id"))
        assertEquals("boxId", fieldIdent("boxId"))
        assertEquals("refineFrom", fieldIdent("refine[from]"))
        assertEquals("LIST_BOXES", constantName("ListBoxes"))
        assertEquals("GET_BOX_POSTING_CHANGES", constantName("GetBoxPostingChanges"))
        assertEquals("TimeTracksService", serviceClassName("time_tracks"))
        assertEquals("timeTracks", serviceAccessorName("time_tracks"))
        assertEquals("isCalendarEvent", variantProperty("Calendar::Event"))
        assertEquals("isBundle", variantProperty("bundle"))
    }

    @Test
    fun caseConversions() {
        assertEquals("calendar_periods", "Calendar Periods".toSnakeCase())
        assertEquals("calendar_periods", "CalendarPeriods".toSnakeCase())
        assertEquals("time_tracks", "time_tracks".toSnakeCase())
        assertEquals("TimeTracks", "time_tracks".toPascalCase())
        assertEquals("entry", singular("entries"))
        assertEquals("address", singular("addresses"))
        assertEquals("box", singular("boxes"))
    }
}
