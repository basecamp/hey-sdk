package com.basecamp.hey.generator

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs
import kotlin.test.assertTrue

class ModelTest {
    private val names = """
        [resource_types]
        boxes = "box"
        workflows = "workflow"
        [hand_written_services]
        boxes = "com.basecamp.hey.services.BoxesService"
    """.trimIndent()

    private fun model(paths: String, schemas: String, behavior: Map<String, String> = emptyMap()): Model {
        val openapi = Json.parseToJsonElement(
            """{"openapi":"3.1.0","info":{"version":"2026-01-01"},"paths":$paths,"components":{"schemas":$schemas}}""",
        ) as JsonObject
        val operations = buildJsonObject {
            for (item in (openapi["paths"] as JsonObject).values) {
                for (operation in (item as JsonObject).values) {
                    val id = (operation as JsonObject).string("operationId")!!
                    put(id, Json.parseToJsonElement(behavior[id] ?: """{"readonly": false}"""))
                }
            }
        }
        val behaviorModel = buildJsonObject { put("operations", operations) }
        return Model.build(openapi, behaviorModel, Naming.parse(names))
    }

    private val boxSchemas = """{
        "Box": {"type":"object","properties":{"id":{"type":"integer","format":"int64"},"kind":{"type":"string"},"owner":{"${'$'}ref":"#/components/schemas/Owner"},"created_at":{"type":"string"},"trial_ends_on":{"type":"string"},"email_address":{"type":"string","x-hey-sensitive":{"category":"pii"}},"labels":{"type":"array","items":{"type":"string"}},"extra":{"type":"object"},"counts":{"type":"object","additionalProperties":{"type":"integer","format":"int32"}}},"required":["id","kind"],"x-hey-polymorphic":{"discriminator":"kind","variants":{"topic":["name"],"Calendar::Event":[]},"discriminatorValues":{"Calendar::Event":["CalendarEvent","Calendar::Event"]}}},
        "Owner": {"type":"object","properties":{"name":{"type":"string"}}},
        "GetBoxResponseContent": {"${'$'}ref":"#/components/schemas/Box"},
        "ListBoxesResponseContent": {"type":"array","items":{"${'$'}ref":"#/components/schemas/Box"}},
        "StagePage": {"type":"string"}
    }"""

    private val boxPaths = """{
        "/boxes.json": {"get": {"operationId":"ListBoxes","tags":["Boxes"],"description":"List the boxes","responses":{"200":{"content":{"application/json":{"schema":{"${'$'}ref":"#/components/schemas/ListBoxesResponseContent"}}}}}}},
        "/boxes/{boxId}": {"get": {"operationId":"GetBox","tags":["Boxes"],"parameters":[{"name":"boxId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}},{"name":"page","in":"query","schema":{"type":"string"}},{"name":"since","in":"query","required":true,"schema":{"type":"string"}}],"x-hey-empty-on":{"statusCodes":[404]},"responses":{"200":{"content":{"application/json":{"schema":{"${'$'}ref":"#/components/schemas/GetBoxResponseContent"}}}},"404":{"description":"gone"}}}},
        "/boxes/{boxId}/observation.json": {"post": {"operationId":"MarkBoxSeen","tags":["Boxes"],"x-hey-idempotent":{"natural":true},"parameters":[{"name":"boxId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}}],"responses":{"200":{"description":"ok"}}}},
        "/workflows/{workflowId}/stages/{stageId}": {"get": {"operationId":"GetWorkflowStage","tags":["Workflows"],"parameters":[{"name":"workflowId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}},{"name":"stageId","in":"path","required":true,"schema":{"type":"integer","format":"int64"}}],"responses":{"200":{"content":{"text/html":{"schema":{"${'$'}ref":"#/components/schemas/StagePage"}}}}}}}
    }"""

    @Test
    fun schemasBecomeStructsAndAliases() {
        val model = model(boxPaths, boxSchemas)
        assertEquals("2026-01-01", model.apiVersion)
        val box = model.schemas.first { it.name == "Box" }
        val shape = assertIs<Shape.Struct>(box.shape)
        val byName = shape.fields.associateBy { it.wireName }
        assertEquals(FieldType.Int64, byName.getValue("id").kind)
        assertTrue(byName.getValue("id").required)
        assertEquals(FieldType.Named("Owner"), byName.getValue("owner").kind)
        assertEquals(FieldType.DateTime, byName.getValue("created_at").kind)
        assertEquals(FieldType.Date, byName.getValue("trial_ends_on").kind)
        assertEquals(FieldType.SensitiveStr, byName.getValue("email_address").kind)
        assertEquals(FieldType.ListOf(FieldType.Str), byName.getValue("labels").kind)
        assertEquals(FieldType.Json, byName.getValue("extra").kind)
        assertEquals(FieldType.MapOf(FieldType.Int32), byName.getValue("counts").kind)
        assertEquals(listOf("topic", "Calendar::Event"), shape.polymorphic?.variants?.map { it.name })
        assertEquals(listOf("topic"), shape.polymorphic?.variants?.get(0)?.values, "a variant the model gives no aliases is matched by its name")
        assertEquals(listOf("CalendarEvent", "Calendar::Event"), shape.polymorphic?.variants?.get(1)?.values)
        assertIs<Shape.Alias>(model.schemas.first { it.name == "GetBoxResponseContent" }.shape)
        assertEquals(
            FieldType.ListOf(FieldType.Named("Box")),
            (model.schemas.first { it.name == "ListBoxesResponseContent" }.shape as Shape.Alias).kind,
        )
    }

    @Test
    fun operationsCarryTheirBehaviour() {
        val model = model(
            boxPaths,
            boxSchemas,
            mapOf(
                "ListBoxes" to """{"readonly":true,"pagination":{"style":"link"},"retry":{"max":3,"base_delay_ms":1000,"retry_on":[429,503]}}""",
            ),
        )
        val boxes = model.services.first { it.name == "boxes" }
        assertEquals(listOf("get", "list", "markSeen"), boxes.operations.map { it.methodName })
        val list = boxes.operations.first { it.id == "ListBoxes" }
        assertEquals(Pagination.LINK, list.pagination)
        assertEquals(3, list.retry.max)
        assertEquals(listOf(429, 503), list.retry.on)
        assertTrue(list.readonly)
        assertTrue(list.idempotent)
        assertEquals(Response.Json("ListBoxesResponseContent"), list.response)

        val get = boxes.operations.first { it.id == "GetBox" }
        assertEquals(listOf(404), get.emptyOn)
        assertEquals(ParamRole.RECORDING, get.pathParams.single().role)
        assertEquals(listOf("page" to false, "since" to true), get.queryParams.map { it.wireName to it.required })
        assertEquals(0, get.retry.max)

        val seen = boxes.operations.first { it.id == "MarkBoxSeen" }
        assertTrue(seen.idempotent, "x-hey-idempotent.natural wins over the method")
        assertEquals(Response.Empty, seen.response)

        val stage = model.services.first { it.name == "workflows" }.operations.single()
        assertEquals(Response.Html("StagePage"), stage.response)
        assertEquals(listOf(ParamRole.PARENT, ParamRole.RECORDING), stage.pathParams.map { it.role })
    }

    @Test
    fun anHtmlResponseThatIsNotAStringIsRefused() {
        val schemas = boxSchemas.replace(""""StagePage": {"type":"string"}""", """"StagePage": {"type":"object","properties":{}}""")
        val error = assertFailsWith<GeneratorException> { model(boxPaths, schemas) }
        assertContains(error.message!!, "GetWorkflowStage answers text/html as StagePage")
    }

    @Test
    fun aMissingReferenceIsRefused() {
        val schemas = boxSchemas.replace(""""Owner": {"type":"object","properties":{"name":{"type":"string"}}},""", "")
        val error = assertFailsWith<GeneratorException> { model(boxPaths, schemas) }
        assertContains(error.message!!, "Box refers to Owner")
    }

    @Test
    fun anUnknownRepresentationIsRefused() {
        val paths = boxPaths.replace("text/html", "image/png")
        val error = assertFailsWith<GeneratorException> { model(paths, boxSchemas) }
        assertContains(error.message!!, "image/png")
    }

    @Test
    fun aRequestBodyTheGeneratorCannotSendIsRefused() {
        val markSeen = """"operationId":"MarkBoxSeen","tags":["Boxes"],"""
        val inline = boxPaths.replace(
            markSeen,
            markSeen + """"requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"seen":{"type":"boolean"}}}}}},""",
        )
        assertContains(assertFailsWith<GeneratorException> { model(inline, boxSchemas) }.message!!, "MarkBoxSeen takes a request body with a schema that is not a \$ref")

        val form = boxPaths.replace(
            markSeen,
            markSeen + """"requestBody":{"content":{"application/x-www-form-urlencoded":{"schema":{"${'$'}ref":"#/components/schemas/Box"}}}},""",
        )
        assertContains(assertFailsWith<GeneratorException> { model(form, boxSchemas) }.message!!, "MarkBoxSeen takes a application/x-www-form-urlencoded request body")

        val referenced = boxPaths.replace(markSeen, markSeen + """"requestBody":{"${'$'}ref":"#/components/requestBodies/Seen"},""")
        assertContains(assertFailsWith<GeneratorException> { model(referenced, boxSchemas) }.message!!, "MarkBoxSeen takes a \$ref request body")

        val supported = boxPaths.replace(
            markSeen,
            markSeen + """"requestBody":{"content":{"application/json":{"schema":{"${'$'}ref":"#/components/schemas/Box"}}}},""",
        )
        assertEquals("Box", model(supported, boxSchemas).services.single { it.name == "boxes" }.operations.single { it.id == "MarkBoxSeen" }.body)
    }

    @Test
    fun aMethodCollisionIsRefused() {
        val paths = boxPaths.replace(""""operationId":"MarkBoxSeen"""", """"operationId":"ListBox"""")
        val error = assertFailsWith<GeneratorException> { model(paths, boxSchemas) }
        assertContains(error.message!!, "both become BoxesService.list")
    }

    @Test
    fun renderedCodeHasTheExpectedShape() {
        val model = model(
            boxPaths,
            boxSchemas,
            mapOf("ListBoxes" to """{"readonly":true,"pagination":{"style":"link"},"retry":{"max":3,"base_delay_ms":1000,"retry_on":[429,503]}}"""),
        )
        val files = render(model)
        val box = files.getValue("models/Box.kt")
        assertContains(box, "package com.basecamp.hey.generated.models")
        assertContains(box, "data class Box(")
        assertContains(box, "    val id: Long,\n    val kind: String,\n")
        assertContains(box, "val owner: Owner? = null,")
        assertContains(box, "@SerialName(\"email_address\")\n    val emailAddress: SensitiveString? = null,")
        assertContains(box, "val counts: Map<String, Int>? = null,")
        assertContains(box, "val isTopic: Boolean get() = kind == \"topic\"")
        assertContains(box, "val isCalendarEvent: Boolean get() = kind in setOf(\"CalendarEvent\", \"Calendar::Event\")")
        assertContains(files.getValue("models/ListBoxesResponseContent.kt"), "typealias ListBoxesResponseContent = List<Box>")
        assertContains(files.getValue("models/StagePage.kt"), "typealias StagePage = String")

        val service = files.getValue("services/BoxesService.kt")
        assertContains(service, "package com.basecamp.hey.generated.services")
        assertContains(service, "open class BoxesService(client: HeyClient) : BaseService(client) {")
        assertContains(service, "data class GetBoxOptions(\n    val page: String? = null,\n)")
        assertContains(service, "suspend fun list(): Page<ListBoxesResponseContent> {")
        assertContains(service, "suspend fun get(boxId: Long, since: String, options: GetBoxOptions? = null): GetBoxResponseContent? {")
        assertContains(service, "operation.resourceId(boxId)")
        assertContains(service, "operation.query(\"since\", since)")
        assertContains(service, "operation.queryOptional(\"page\", options?.page)")
        assertContains(service, "return client.sendOptional(operation)")
        assertContains(service, "suspend fun markSeen(boxId: Long): Unit {")
        val workflows = files.getValue("services/WorkflowsService.kt")
        assertContains(workflows, "class WorkflowsService(client: HeyClient) : BaseService(client) {")
        assertContains(workflows, "suspend fun getStage(workflowId: Long, stageId: Long): String {")
        assertContains(workflows, "operation.resourceId(stageId)")

        val routes = files.getValue("Routes.kt")
        assertContains(routes, "package com.basecamp.hey.generated")
        assertContains(routes, "val LIST_BOXES: Route = Route(")
        assertContains(routes, "pagination = Pagination.LINK,")
        assertContains(routes, "retry = RetryPolicy(max = 3, baseDelayMs = 1000L, retryOn = listOf(429, 503)),")
        assertContains(routes, "RouteParam(\"boxId\", ParamRole.RECORDING, ParamKind.INT64),")
        assertContains(routes, "html = true,")

        val accessors = files.getValue("ServiceAccessors.kt")
        assertContains(accessors, "val HeyClient.boxes: com.basecamp.hey.services.BoxesService")
        assertContains(accessors, "get() = service(\"Boxes\") { com.basecamp.hey.services.BoxesService(this) }")
        assertContains(accessors, "val HeyClient.workflows: WorkflowsService")
        assertEquals(false, files.containsKey("ApiVersion.kt"))
    }
}
