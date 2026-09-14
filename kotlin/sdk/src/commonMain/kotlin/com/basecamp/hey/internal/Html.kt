package com.basecamp.hey.internal

/**
 * A small, tolerant HTML reader for the pages HEY serves where it serves no JSON. It builds
 * a tree of elements and text, closes what the page leaves open, and reads nothing it does
 * not need: no scripts, no styles, no entities beyond the common ones.
 */
internal sealed class HtmlNode {
    class Text(val text: String) : HtmlNode()

    class Element(val tag: String, val attributes: Map<String, String>) : HtmlNode() {
        val children: MutableList<HtmlNode> = mutableListOf()

        fun attribute(name: String): String? = attributes[name]

        /** The element's classes, split on any whitespace. */
        val classes: List<String> get() = attribute("class")?.split(Regex("\\s+"))?.filter { it.isNotEmpty() }.orEmpty()

        /**
         * Every element under this one, in document order, this one excluded. Walked without
         * recursion: the page is the server's and its nesting is not bounded.
         */
        fun descendants(): Sequence<Element> = sequence {
            val pending = ArrayDeque<HtmlNode>()
            children.asReversed().forEach(pending::addLast)
            while (pending.isNotEmpty()) {
                val node = pending.removeLast()
                if (node is Element) {
                    yield(node)
                    node.children.asReversed().forEach(pending::addLast)
                }
            }
        }

        /** The first element at or under this one, in document order, that [matches]. */
        fun firstAtOrUnder(matches: (Element) -> Boolean): Element? =
            if (matches(this)) this else descendants().firstOrNull(matches)

        /**
         * The text a reader sees under this element, whitespace collapsed: text meant for
         * screen readers only is left out.
         */
        fun visibleText(): String {
            val text = StringBuilder()
            val pending = ArrayDeque<HtmlNode>()
            children.asReversed().forEach(pending::addLast)
            while (pending.isNotEmpty()) {
                when (val node = pending.removeLast()) {
                    is Text -> text.append(node.text)
                    is Element -> if (!node.isVisuallyHidden()) node.children.asReversed().forEach(pending::addLast)
                }
            }
            return text.split(Regex("\\s+")).filter { it.isNotEmpty() }.joinToString(" ")
        }

        private fun isVisuallyHidden(): Boolean =
            classes.any { it == "sr-only" || it == "screen-reader-only" || it == "u-for-screen-reader" || it == "visually-hidden" }
    }
}

private val VOID_ELEMENTS = setOf(
    "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr",
)
private val RAW_TEXT_ELEMENTS = setOf("script", "style")

/** Reads a page into a tree under a synthetic root. */
internal fun parseHtml(html: String): HtmlNode.Element {
    val root = HtmlNode.Element("#root", emptyMap())
    val stack = ArrayDeque<HtmlNode.Element>().apply { addLast(root) }
    var index = 0
    while (index < html.length) {
        val open = html.indexOf('<', index)
        if (open < 0) {
            stack.last().children += HtmlNode.Text(decodeEntities(html.substring(index)))
            break
        }
        if (open > index) stack.last().children += HtmlNode.Text(decodeEntities(html.substring(index, open)))
        when {
            html.startsWith("<!--", open) -> {
                val end = html.indexOf("-->", open + 4)
                index = if (end < 0) html.length else end + 3
            }
            html.startsWith("<!", open) || html.startsWith("<?", open) -> {
                val end = html.indexOf('>', open)
                index = if (end < 0) html.length else end + 1
            }
            html.startsWith("</", open) -> {
                val end = html.indexOf('>', open)
                val name = (if (end < 0) html.substring(open + 2) else html.substring(open + 2, end)).trim().lowercase()
                val depth = stack.indexOfLast { it.tag == name }
                if (depth > 0) while (stack.size > depth) stack.removeLast()
                index = if (end < 0) html.length else end + 1
            }
            else -> {
                val end = findTagEnd(html, open)
                val inside = html.substring(open + 1, end).trim()
                val selfClosing = inside.endsWith("/")
                val content = inside.removeSuffix("/").trim()
                val nameEnd = content.indexOfFirst { it.isWhitespace() }.let { if (it < 0) content.length else it }
                val name = content.substring(0, nameEnd).lowercase()
                val element = HtmlNode.Element(name, parseAttributes(content.substring(nameEnd)))
                stack.last().children += element
                index = end + 1
                when {
                    name.isEmpty() -> {}
                    name in RAW_TEXT_ELEMENTS -> {
                        val close = html.indexOf("</$name", index, ignoreCase = true)
                        val closeEnd = if (close < 0) html.length else html.indexOf('>', close).let { if (it < 0) html.length else it + 1 }
                        index = closeEnd
                    }
                    selfClosing || name in VOID_ELEMENTS -> {}
                    else -> stack.addLast(element)
                }
            }
        }
    }
    return root
}

private fun findTagEnd(html: String, open: Int): Int {
    var quote: Char? = null
    var index = open + 1
    while (index < html.length) {
        val character = html[index]
        when {
            quote != null -> if (character == quote) quote = null
            character == '"' || character == '\'' -> quote = character
            character == '>' -> return index
        }
        index++
    }
    return html.length
}

private fun parseAttributes(source: String): Map<String, String> {
    val attributes = linkedMapOf<String, String>()
    var index = 0
    while (index < source.length) {
        while (index < source.length && source[index].isWhitespace()) index++
        if (index >= source.length) break
        val nameStart = index
        while (index < source.length && !source[index].isWhitespace() && source[index] != '=') index++
        val name = source.substring(nameStart, index).lowercase()
        while (index < source.length && source[index].isWhitespace()) index++
        var value = ""
        if (index < source.length && source[index] == '=') {
            index++
            while (index < source.length && source[index].isWhitespace()) index++
            if (index < source.length && (source[index] == '"' || source[index] == '\'')) {
                val quote = source[index]
                val end = source.indexOf(quote, index + 1).let { if (it < 0) source.length else it }
                value = source.substring(index + 1, end)
                index = end + 1
            } else {
                val end = source.indexOfAny(charArrayOf(' ', '\t', '\n', '\r'), index).let { if (it < 0) source.length else it }
                value = source.substring(index, end)
                index = end
            }
        }
        if (name.isNotEmpty()) attributes[name] = decodeEntities(value)
    }
    return attributes
}

private fun decodeEntities(text: String): String {
    if (!text.contains('&')) return text
    return Regex("&(#x[0-9a-fA-F]+|#[0-9]+|[a-zA-Z]+);").replace(text) { match ->
        val entity = match.groupValues[1]
        when {
            entity.startsWith("#x") -> entity.substring(2).toIntOrNull(16)?.let(::codePointToString) ?: match.value
            entity.startsWith("#") -> entity.substring(1).toIntOrNull()?.let(::codePointToString) ?: match.value
            else -> when (entity) {
                "amp" -> "&"
                "lt" -> "<"
                "gt" -> ">"
                "quot" -> "\""
                "apos" -> "'"
                "nbsp" -> " "
                else -> match.value
            }
        }
    }
}

private fun codePointToString(codePoint: Int): String = when {
    codePoint < 0 || codePoint > 0x10FFFF -> ""
    codePoint < 0x10000 -> codePoint.toChar().toString()
    else -> {
        val offset = codePoint - 0x10000
        charArrayOf((0xD800 + (offset shr 10)).toChar(), (0xDC00 + (offset and 0x3FF)).toChar()).concatToString()
    }
}
