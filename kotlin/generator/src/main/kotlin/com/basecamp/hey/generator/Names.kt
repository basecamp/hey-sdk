package com.basecamp.hey.generator

/**
 * The naming overrides read from `kotlin/generator/names.toml`: which service an
 * operation files under, what a method is called when the derivation gets it wrong, what a
 * schema is called when its Smithy name collides with Kotlin's, and the noun each service's
 * operations act on.
 */
class Naming(
    private val services: Map<String, String> = emptyMap(),
    private val operationServices: Map<String, String> = emptyMap(),
    private val operationMethods: Map<String, String> = emptyMap(),
    private val typeNames: Map<String, String> = emptyMap(),
    private val resourceTypes: Map<String, String> = emptyMap(),
    private val operationResourceTypes: Map<String, String> = emptyMap(),
    private val handWrittenServices: Map<String, String> = emptyMap(),
) {
    /** The hand-written subclass the client's accessor constructs for a service, when there is one. */
    fun handWrittenServiceFor(service: String): String? = handWrittenServices[service]

    /** The service an operation files under, in `snake_case`: `boxes`, `time_tracks`. */
    fun serviceFor(operationId: String, tag: String): String =
        operationServices[operationId] ?: services[tag] ?: tag.toSnakeCase()

    /**
     * The method an operation becomes on its service: the operation id's words with the
     * service's own noun dropped, camelCased. `ListBoxes` on `boxes` is `list`;
     * `GetBoxPostingChanges` on `postings` is `getBoxChanges`. An empty name or a Kotlin
     * keyword is refused, and settled with an `[operation_methods]` override.
     */
    fun methodFor(operationId: String, service: String): String {
        val method = operationMethods[operationId] ?: run {
            val serviceWords = service.split('_').map(::singular)
            camelWords(operationId)
                .map { it.lowercase() }
                .filter { singular(it) !in serviceWords }
                .let(::camelCase)
        }
        if (method.isEmpty() || method in KEYWORDS) {
            throw GeneratorException(
                "$operationId becomes `$method` in $service; add an [operation_methods] override to names.toml",
            )
        }
        return method
    }

    /** What a schema is called in Kotlin: its own name unless `[type_names]` renames it. */
    fun typeFor(schema: String): String = typeNames[schema] ?: schema

    /** The noun an operation acts on, as the hooks report it. */
    fun resourceTypeFor(operationId: String, service: String): String =
        operationResourceTypes[operationId]
            ?: resourceTypes[service]
            ?: throw GeneratorException(
                "$service has no resource type; add one to the [resource_types] table in names.toml",
            )

    companion object {
        /** Kotlin's hard keywords: the ones that cannot be an identifier without backticks. */
        val KEYWORDS: Set<String> = setOf(
            "as", "break", "class", "continue", "do", "else", "false", "for", "fun", "if", "in",
            "interface", "is", "null", "object", "package", "return", "super", "this", "throw",
            "true", "try", "typealias", "typeof", "val", "var", "when", "while",
        )

        /**
         * Reads the subset of TOML `names.toml` is written in: `[tables]` of
         * `key = "value"` lines, keys quoted or bare, comments after `#`.
         */
        fun parse(source: String): Naming {
            val tables = mutableMapOf<String, MutableMap<String, String>>()
            var current: MutableMap<String, String>? = null
            for ((index, raw) in source.lines().withIndex()) {
                val line = raw.substringBefore('#').trim()
                if (line.isEmpty()) continue
                if (line.startsWith("[") && line.endsWith("]")) {
                    current = tables.getOrPut(line.substring(1, line.length - 1).trim()) { mutableMapOf() }
                    continue
                }
                val table = current ?: throw GeneratorException("names.toml line ${index + 1}: key outside a table")
                val separator = line.indexOf('=')
                if (separator < 0) throw GeneratorException("names.toml line ${index + 1}: expected key = \"value\"")
                val key = unquote(line.substring(0, separator).trim())
                val value = line.substring(separator + 1).trim()
                if (!value.startsWith("\"") || !value.endsWith("\"") || value.length < 2) {
                    throw GeneratorException("names.toml line ${index + 1}: value must be a quoted string")
                }
                table[key] = unquote(value)
            }
            return Naming(
                services = tables["services"].orEmpty(),
                operationServices = tables["operation_services"].orEmpty(),
                operationMethods = tables["operation_methods"].orEmpty(),
                typeNames = tables["type_names"].orEmpty(),
                resourceTypes = tables["resource_types"].orEmpty(),
                operationResourceTypes = tables["operation_resource_types"].orEmpty(),
                handWrittenServices = tables["hand_written_services"].orEmpty(),
            )
        }

        private fun unquote(value: String): String =
            if (value.length >= 2 && value.startsWith("\"") && value.endsWith("\"")) {
                value.substring(1, value.length - 1)
            } else {
                value
            }
    }
}

class GeneratorException(message: String) : RuntimeException(message)

/** The class a service becomes: `time_tracks` is `TimeTracksService`. */
fun serviceClassName(service: String): String = service.toPascalCase() + "Service"

/** The property a service is reached through on the client: `time_tracks` is `timeTracks`. */
fun serviceAccessorName(service: String): String = service.toCamelCase()

/**
 * A wire name as a Kotlin identifier: `email_address` is `emailAddress`, `refine[from]` is
 * `refineFrom`, and a keyword is backticked.
 */
fun fieldIdent(wireName: String): String {
    val ident = wireName.replace('[', '_').replace(']', '_').trimEnd('_').toCamelCase()
    return if (ident in Naming.KEYWORDS) "`$ident`" else ident
}

/** The constant a route is held under: `ListBoxes` is `LIST_BOXES`. */
fun constantName(operationId: String): String = operationId.toSnakeCase().uppercase()

/** The predicate a polymorphic variant answers to: `Calendar::Event` is `isCalendarEvent`. */
fun variantProperty(variant: String): String =
    "is" + variant.split("::").joinToString("") { it.toPascalCase() }

/** `GetBoxPostingChanges` is `Get`, `Box`, `Posting`, `Changes`. */
fun camelWords(source: String): List<String> {
    val words = mutableListOf<String>()
    val current = StringBuilder()
    for (character in source) {
        if (character.isUpperCase() && current.isNotEmpty()) {
            words += current.toString()
            current.clear()
        }
        current.append(character)
    }
    if (current.isNotEmpty()) words += current.toString()
    return words
}

private fun camelCase(words: List<String>): String =
    words.mapIndexed { index, word -> if (index == 0) word else word.replaceFirstChar { it.uppercase() } }
        .joinToString("")

/** `boxes` is `box`, `entries` is `entry`, `addresses` is `address`, `status` is `status`. */
fun singular(word: String): String = when {
    word.endsWith("ies") -> word.dropLast(3) + "y"
    word.endsWith("ses") || word.endsWith("xes") || word.endsWith("ches") || word.endsWith("shes") ->
        word.dropLast(2)
    word.endsWith("s") && !word.endsWith("ss") -> word.dropLast(1)
    else -> word
}

/** `Calendar Periods` and `CalendarPeriods` are both `calendar_periods`. */
fun String.toSnakeCase(): String {
    val out = StringBuilder()
    var previousLower = false
    for (character in this) {
        when {
            character == ' ' || character == '-' || character == '_' -> {
                if (out.isNotEmpty() && out.last() != '_') out.append('_')
                previousLower = false
            }
            character.isUpperCase() -> {
                if (out.isNotEmpty() && out.last() != '_' && previousLower) out.append('_')
                out.append(character.lowercaseChar())
                previousLower = false
            }
            else -> {
                out.append(character)
                previousLower = character.isLowerCase() || character.isDigit()
            }
        }
    }
    return out.toString().trim('_')
}

/** `time_tracks` is `timeTracks`. */
fun String.toCamelCase(): String {
    val parts = split('_').filter { it.isNotEmpty() }
    return parts.mapIndexed { index, part ->
        if (index == 0) part.replaceFirstChar { it.lowercase() } else part.replaceFirstChar { it.uppercase() }
    }.joinToString("")
}

/** `time_tracks` is `TimeTracks`. */
fun String.toPascalCase(): String =
    split('_').filter { it.isNotEmpty() }.joinToString("") { it.replaceFirstChar { c -> c.uppercase() } }
