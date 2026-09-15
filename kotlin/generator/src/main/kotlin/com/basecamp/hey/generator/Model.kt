package com.basecamp.hey.generator

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.longOrNull

/** Everything the emitters need, read once out of `openapi.json` and `behavior-model.json`. */
class Model(
    val apiVersion: String,
    val schemas: List<Schema>,
    val services: List<Service>,
    val naming: Naming,
) {
    companion object {
        fun build(openapi: JsonObject, behavior: JsonObject, naming: Naming): Model {
            val apiVersion = openapi.obj("info")?.string("version")
                ?: throw GeneratorException("openapi.json has no info.version")
            val schemas = buildSchemas(openapi, naming)
            val services = buildServices(openapi, behavior, naming)
            checkReferences(schemas, services)
            checkHtmlResponses(schemas, services)
            return Model(apiVersion, schemas, services, naming)
        }
    }
}

class Schema(
    val name: String,
    val description: String?,
    val shape: Shape,
)

sealed class Shape {
    class Struct(val fields: List<Field>, val polymorphic: Polymorphic?) : Shape()
    class Alias(val kind: FieldType) : Shape()
}

/** A schema with a discriminator: each variant is named by the model, and matched by every value the model says HEY may send for it. */
class Polymorphic(val discriminator: String, val variants: List<Variant>) {
    /** One variant and the discriminator values that mean it: the namespaced name and, where the model declares them, its aliases. */
    class Variant(val name: String, val values: List<String>)
}

class Field(
    val wireName: String,
    val description: String?,
    val kind: FieldType,
    val required: Boolean,
    /** The field names the type it belongs to, so it cannot have a default instance. */
    val recursive: Boolean,
)

sealed class FieldType {
    object Str : FieldType()
    object SensitiveStr : FieldType()
    object DateTime : FieldType()
    object Date : FieldType()
    object Bool : FieldType()
    object Int32 : FieldType()
    object Int64 : FieldType()
    object Json : FieldType()
    data class Named(val name: String) : FieldType()
    data class ListOf(val inner: FieldType) : FieldType()
    data class MapOf(val inner: FieldType) : FieldType()

    /** The schema this type names, if it names one. */
    fun named(): String? = when (this) {
        is Named -> name
        is ListOf -> inner.named()
        is MapOf -> inner.named()
        else -> null
    }

    fun mentions(schema: String): Boolean = named() == schema
}

class Service(val name: String, val operations: List<Operation>)

class Operation(
    val id: String,
    /** The service in `snake_case`: `boxes`, `time_tracks`. */
    val service: String,
    val methodName: String,
    val description: String?,
    val httpMethod: String,
    val path: String,
    /** The tag the model files the operation under: `Boxes`, `Calendar Time Tracks`. */
    val resource: String,
    val resourceType: String,
    val pathParams: List<PathParam>,
    val queryParams: List<QueryParam>,
    val body: String?,
    val response: Response,
    val idempotent: Boolean,
    val readonly: Boolean,
    val emptyOn: List<Int>,
    val pagination: Pagination,
    val pageParameter: String?,
    val retry: Retry,
)

class PathParam(val wireName: String, val kind: ParamKind, val role: ParamRole)

class QueryParam(val wireName: String, val kind: ParamKind, val required: Boolean)

enum class ParamKind { STRING, BOOL, INT32, INT64 }

/** Where a path parameter sits: the last segment names the record itself, anything before it a parent. */
enum class ParamRole { PARENT, RECORDING }

sealed class Response {
    object Empty : Response()
    data class Json(val name: String) : Response()
    data class Html(val name: String) : Response()

    fun describe(): String = when (this) {
        Empty -> "no body"
        is Json -> "$name as JSON"
        is Html -> "$name as HTML"
    }
}

enum class Pagination { NONE, LINK, WINDOW }

class Retry(val max: Int, val baseDelayMs: Long, val on: List<Int>)

private const val SCHEMA_REFERENCE = "#/components/schemas/"

/** The representations the generator can emit a method for. Anything else fails generation. */
private val REPRESENTATIONS = listOf("application/json", "text/html")

private fun buildSchemas(openapi: JsonObject, naming: Naming): List<Schema> {
    val components = openapi.obj("components")?.obj("schemas")
        ?: throw GeneratorException("openapi.json has no components.schemas")
    return components.keys.sorted().map { schemaName ->
        val schema = components.getValue(schemaName).jsonObject
        val name = naming.typeFor(schemaName)
        val properties = schema["properties"]
        val shape = if (properties != null) {
            val required = schema["required"]?.let { it as? JsonArray }
                ?.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }?.toSet().orEmpty()
            val members = properties as? JsonObject
                ?: throw GeneratorException("$name.properties is not an object")
            val fields = members.entries.map { (wireName, property) ->
                val kind = fieldType(wireName, property.jsonObject, naming)
                Field(
                    wireName = wireName,
                    description = property.jsonObject.string("description"),
                    kind = kind,
                    required = wireName in required,
                    recursive = kind.mentions(name),
                )
            }
            Shape.Struct(fields, polymorphicOf(schemaName, schema))
        } else {
            Shape.Alias(fieldType(schemaName, schema, naming))
        }
        Schema(name, schema.string("description"), shape)
    }
}

private fun polymorphicOf(name: String, schema: JsonObject): Polymorphic? {
    val extension = schema.obj("x-hey-polymorphic") ?: return null
    val discriminator = extension.string("discriminator") ?: return null
    val variants = extension.obj("variants")?.keys?.toList() ?: return null
    val aliases = extension.obj("discriminatorValues")
    // A value list for a variant the schema does not declare is a model error, as the
    // TypeScript generator treats it; an empty or missing list means the name alone, as the
    // Rust generator treats it. Neither becomes a check that can never be true.
    aliases?.keys?.firstOrNull { it !in variants }?.let { stray ->
        throw GeneratorException("$name: discriminatorValues names $stray, which is not a variant")
    }
    return Polymorphic(
        discriminator,
        variants.map { variant ->
            val declared = (aliases?.get(variant) as? JsonArray)?.map { it.jsonPrimitive.content }?.filter { it.isNotEmpty() }
            Polymorphic.Variant(variant, declared?.takeIf { it.isNotEmpty() } ?: listOf(variant))
        },
    )
}

private fun fieldType(name: String, property: JsonObject, naming: Naming): FieldType {
    property.string("\$ref")?.let { return FieldType.Named(naming.typeFor(referenceName(it))) }
    val format = property.string("format")
    return when (val type = property.string("type")) {
        "string" -> stringType(name, property, format)
        "boolean" -> FieldType.Bool
        "integer" -> if (format == "int32") FieldType.Int32 else FieldType.Int64
        "array" -> FieldType.ListOf(fieldType(name, property.obj("items") ?: JsonObject(emptyMap()), naming))
        "object" -> property.obj("additionalProperties")
            ?.let { FieldType.MapOf(fieldType(name, it, naming)) }
            ?: FieldType.Json
        else -> throw GeneratorException("$name: unsupported schema type $type")
    }
}

private fun stringType(name: String, property: JsonObject, format: String?): FieldType = when {
    property.containsKey("x-hey-sensitive") -> FieldType.SensitiveStr
    format == "date-time" || name.endsWith("_at") -> FieldType.DateTime
    format == "date" || name.endsWith("_on") -> FieldType.Date
    else -> FieldType.Str
}

private fun buildServices(openapi: JsonObject, behavior: JsonObject, naming: Naming): List<Service> {
    val paths = openapi.obj("paths") ?: throw GeneratorException("openapi.json has no paths")
    val behaviors = behavior.obj("operations") ?: throw GeneratorException("behavior-model.json has no operations")
    val services = mutableMapOf<String, MutableList<Operation>>()

    for ((path, item) in paths.entries.sortedBy { it.key }) {
        val itemObject = item as? JsonObject ?: throw GeneratorException("$path is not an object")
        for ((httpMethod, operation) in itemObject) {
            when (httpMethod) {
                "get", "post", "put", "patch", "delete" -> {}
                "head", "options", "trace" ->
                    throw GeneratorException("$path has a $httpMethod operation, which the generator does not emit")
                else -> continue
            }
            val operationObject = operation.jsonObject
            val id = operationObject.string("operationId")
                ?: throw GeneratorException("$httpMethod $path has no operationId")
            val tag = (operationObject["tags"] as? JsonArray)?.firstOrNull()?.let { (it as? JsonPrimitive)?.contentOrNull }
                ?: throw GeneratorException("$id has no tag")
            val service = naming.serviceFor(id, tag)
            val semantics = behaviors.obj(id) ?: throw GeneratorException("$id is missing from behavior-model.json")
            services.getOrPut(service) { mutableListOf() } += Operation(
                id = id,
                service = service,
                methodName = naming.methodFor(id, service),
                description = operationObject.string("description"),
                httpMethod = httpMethod.uppercase(),
                path = path,
                resource = tag,
                resourceType = naming.resourceTypeFor(id, service),
                pathParams = pathParams(operationObject, path),
                queryParams = queryParams(operationObject),
                body = bodyOf(id, operationObject, naming),
                response = responseOf(id, operationObject, naming),
                idempotent = idempotent(httpMethod, operationObject),
                readonly = semantics["readonly"]?.let { (it as? JsonPrimitive)?.booleanOrNull }
                    ?: throw GeneratorException("$id has no readonly in behavior-model.json"),
                emptyOn = statusCodes(operationObject.obj("x-hey-empty-on")?.get("statusCodes")),
                pagination = pagination(semantics),
                pageParameter = semantics.obj("pagination")?.string("pageParameter"),
                retry = retry(semantics),
            )
        }
    }

    return services.entries.sortedBy { it.key }.map { (name, operations) ->
        val sorted = operations.sortedWith(compareBy({ it.methodName }, { it.id }))
        sorted.zipWithNext().forEach { (a, b) ->
            if (a.methodName == b.methodName) {
                throw GeneratorException(
                    "${a.id} and ${b.id} both become ${serviceClassName(name)}.${a.methodName}; add an [operation_methods] override to names.toml",
                )
            }
        }
        Service(name, sorted)
    }
}

private fun pathParams(operation: JsonObject, path: String): List<PathParam> {
    val lastSegment = path.removeSuffix(".json").substringAfterLast('/')
    return parametersIn(operation, "path").map { parameter ->
        val wireName = parameter.string("name") ?: throw GeneratorException("a path parameter of $path has no name")
        PathParam(
            wireName = wireName,
            kind = paramKind(parameter),
            role = if (lastSegment == "{$wireName}") ParamRole.RECORDING else ParamRole.PARENT,
        )
    }
}

private fun queryParams(operation: JsonObject): List<QueryParam> =
    parametersIn(operation, "query").map { parameter ->
        QueryParam(
            wireName = parameter.string("name") ?: throw GeneratorException("a query parameter has no name"),
            kind = paramKind(parameter),
            required = parameter["required"]?.let { (it as? JsonPrimitive)?.booleanOrNull } ?: false,
        )
    }

private fun parametersIn(operation: JsonObject, location: String): List<JsonObject> =
    (operation["parameters"] as? JsonArray).orEmpty()
        .map { it.jsonObject }
        .filter { it.string("in") == location }

private fun paramKind(parameter: JsonObject): ParamKind {
    val schema = parameter.obj("schema") ?: JsonObject(emptyMap())
    return when (val type = schema.string("type")) {
        "string" -> ParamKind.STRING
        "boolean" -> ParamKind.BOOL
        "integer" -> if (schema.string("format") == "int32") ParamKind.INT32 else ParamKind.INT64
        else -> throw GeneratorException("parameter ${parameter.string("name")}: unsupported type $type")
    }
}

/**
 * The type an operation's request body is, or null when it takes none. A body the generator
 * cannot send as the model describes it — in another representation, through a `\$ref`
 * request body, or with a schema that is not a `\$ref` — fails generation naming the
 * operation, rather than becoming a method that quietly sends nothing.
 */
private fun bodyOf(id: String, operation: JsonObject, naming: Naming): String? {
    val requestBody = operation.obj("requestBody") ?: return null
    if (requestBody.containsKey("\$ref")) {
        throw GeneratorException("$id takes a \$ref request body, which the generator does not resolve; write the body inline")
    }
    val content = requestBody.obj("content") ?: throw GeneratorException("$id takes a request body with no content")
    val representations = content.keys.toList()
    val mediaType = when (representations.size) {
        0 -> throw GeneratorException("$id takes a request body with no representation")
        1 -> representations.single()
        else -> throw GeneratorException(
            "$id takes a request body in ${representations.size} representations (${representations.joinToString(", ")}); the generator sends one",
        )
    }
    if (mediaType != "application/json") {
        throw GeneratorException("$id takes a $mediaType request body, which the generator has no representation for; it sends application/json")
    }
    val reference = content.obj(mediaType)?.obj("schema")?.string("\$ref")
        ?: throw GeneratorException("$id takes a request body with a schema that is not a \$ref to components.schemas")
    return naming.typeFor(referenceName(reference))
}

private fun responseOf(id: String, operation: JsonObject, naming: Naming): Response {
    val responses = operation.obj("responses") ?: throw GeneratorException("$id has no responses")
    var agreed: Pair<String, Response>? = null
    for ((status, response) in responses) {
        if (!status.startsWith('2')) continue
        val representation = representationOf(id, status, response.jsonObject, naming)
        val first = agreed
        if (first == null) {
            agreed = status to representation
        } else if (first.second != representation) {
            throw GeneratorException(
                "$id answers ${first.first} and $status differently (${first.second.describe()} and ${representation.describe()}); the generator emits one representation per operation",
            )
        }
    }
    return agreed?.second ?: throw GeneratorException("$id has no 2xx response")
}

private fun representationOf(id: String, status: String, response: JsonObject, naming: Naming): Response {
    if (response.containsKey("\$ref")) {
        throw GeneratorException("$id answers $status through a \$ref response, which the generator does not resolve; write the response inline")
    }
    val content = response.obj("content") ?: return Response.Empty
    val representations = content.keys.toList()
    val mediaType = when (representations.size) {
        0 -> return Response.Empty
        1 -> representations.single()
        else -> throw GeneratorException(
            "$id answers $status in ${representations.size} representations (${representations.joinToString(", ")}); the generator emits one",
        )
    }
    if (mediaType !in REPRESENTATIONS) {
        throw GeneratorException(
            "$id answers $status as $mediaType, which the generator has no representation for; it emits ${REPRESENTATIONS.joinToString(" and ")}",
        )
    }
    val reference = content.obj(mediaType)?.obj("schema")?.string("\$ref")
        ?: throw GeneratorException("$id answers $status as $mediaType with a schema that is not a \$ref to components.schemas")
    val name = naming.typeFor(referenceName(reference))
    return if (mediaType == "text/html") Response.Html(name) else Response.Json(name)
}

private fun idempotent(httpMethod: String, operation: JsonObject): Boolean =
    operation.obj("x-hey-idempotent")?.get("natural")?.let { (it as? JsonPrimitive)?.booleanOrNull }
        ?: (httpMethod in listOf("get", "head", "put", "delete"))

private fun pagination(semantics: JsonObject): Pagination =
    when (val style = semantics.obj("pagination")?.string("style")) {
        null -> Pagination.NONE
        "link" -> Pagination.LINK
        "window" -> Pagination.WINDOW
        else -> throw GeneratorException("unsupported pagination style $style")
    }

private fun retry(semantics: JsonObject): Retry {
    val retry = semantics.obj("retry") ?: JsonObject(emptyMap())
    return Retry(
        max = (retry["max"] as? JsonPrimitive)?.intOrNull ?: 0,
        baseDelayMs = (retry["base_delay_ms"] as? JsonPrimitive)?.longOrNull ?: 1000L,
        on = statusCodes(retry["retry_on"]),
    )
}

private fun statusCodes(codes: kotlinx.serialization.json.JsonElement?): List<Int> =
    (codes as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.intOrNull }.filter { it in 100..999 }

/** The schema a `$ref` names. Only a reference into this document's own schemas can become a type. */
private fun referenceName(reference: String): String {
    val name = reference.removePrefix(SCHEMA_REFERENCE)
    if (name == reference || name.isEmpty() || name.contains('/')) {
        throw GeneratorException("\$ref $reference does not point into $SCHEMA_REFERENCE; the generator resolves nothing else")
    }
    return name
}

private fun checkReferences(schemas: List<Schema>, services: List<Service>) {
    val known = schemas.map { it.name }.toSet()
    for (schema in schemas) {
        val mentioned = when (val shape = schema.shape) {
            is Shape.Struct -> shape.fields.map { it.kind }
            is Shape.Alias -> listOf(shape.kind)
        }
        for (kind in mentioned) {
            val name = kind.named() ?: continue
            if (name !in known) {
                throw GeneratorException("${schema.name} refers to $name, which is not in components.schemas")
            }
        }
    }
    for (operation in services.flatMap { it.operations }) {
        val mentioned = listOfNotNull(
            operation.body,
            when (val response = operation.response) {
                Response.Empty -> null
                is Response.Json -> response.name
                is Response.Html -> response.name
            },
        )
        for (name in mentioned) {
            if (name !in known) {
                throw GeneratorException("${operation.id} refers to $name, which is not in components.schemas")
            }
        }
    }
}

/** A page is handed back as the `String` it arrived as, so the schema an HTML response names has to be a string alias. */
private fun checkHtmlResponses(schemas: List<Schema>, services: List<Service>) {
    for (operation in services.flatMap { it.operations }) {
        val response = operation.response as? Response.Html ?: continue
        if (!resolvesToString(schemas, response.name)) {
            throw GeneratorException(
                "${operation.id} answers text/html as ${response.name}, which is not a string schema; an HTML page is handed back as a String",
            )
        }
    }
}

private fun resolvesToString(schemas: List<Schema>, name: String): Boolean {
    var current = name
    repeat(schemas.size + 1) {
        val shape = schemas.firstOrNull { it.name == current }?.shape ?: return false
        when {
            shape is Shape.Alias && shape.kind == FieldType.Str -> return true
            shape is Shape.Alias && shape.kind is FieldType.Named -> current = (shape.kind as FieldType.Named).name
            else -> return false
        }
    }
    return false
}

internal fun JsonObject.obj(key: String): JsonObject? = this[key] as? JsonObject

internal fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.contentOrNull
