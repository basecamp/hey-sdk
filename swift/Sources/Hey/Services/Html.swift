import Foundation

/// A small, tolerant HTML reader for the pages HEY serves where it serves no JSON. It builds a
/// tree of elements and text, closes what the page leaves open, and reads nothing it does not
/// need: no scripts, no styles, no entities beyond the common ones.
///
/// The tree is held flat — every element is an index into the document's arrays — so a page
/// nested deeper than any stack is built, walked and freed without recursion: the page is the
/// server's, and its nesting is not bounded.
final class HtmlDocument: @unchecked Sendable {
    /// One child of an element: a run of text, or another element by its index.
    enum Child {
        case text(String)
        case element(Int)
    }

    fileprivate(set) var tags: [String] = []
    fileprivate(set) var attributes: [[String: String]] = []
    fileprivate(set) var children: [[Child]] = []

    /// The synthetic element every top-level node of the page sits under.
    var root: HtmlElement { HtmlElement(document: self, index: 0) }

    fileprivate init() {
        _ = add("#root", [:])
    }

    fileprivate func add(_ tag: String, _ attributes: [String: String]) -> Int {
        tags.append(tag)
        self.attributes.append(attributes)
        children.append([])
        return tags.count - 1
    }

    fileprivate func append(_ child: Child, to parent: Int) {
        children[parent].append(child)
    }
}

/// One element of an ``HtmlDocument``.
struct HtmlElement {
    let document: HtmlDocument
    let index: Int

    /// The element's tag, lowercased.
    var tag: String { document.tags[index] }

    /// An attribute's value, by its lowercased name.
    func attribute(_ name: String) -> String? { document.attributes[index][name] }

    /// The element's classes, split on any whitespace.
    var classes: [String] {
        (attribute("class") ?? "").split(whereSeparator: { isHtmlWhitespace($0) }).map(String.init)
    }

    /// The element's direct children that are elements, in document order.
    var childElements: [HtmlElement] {
        document.children[index].compactMap { child in
            if case let .element(child) = child { return HtmlElement(document: document, index: child) }
            return nil
        }
    }

    /// Every element under this one, in document order, this one excluded. Walked without
    /// recursion: the page is the server's and its nesting is not bounded.
    func descendants() -> [HtmlElement] {
        var found: [HtmlElement] = []
        forEachDescendant { found.append($0); return false }
        return found
    }

    /// The first element at or under this one, in document order, that `matches`.
    func firstAtOrUnder(_ matches: (HtmlElement) -> Bool) -> HtmlElement? {
        if matches(self) { return self }
        var found: HtmlElement?
        forEachDescendant { element in
            if matches(element) { found = element; return true }
            return false
        }
        return found
    }

    /// Visits every element under this one in document order until `visit` answers true.
    private func forEachDescendant(_ visit: (HtmlElement) -> Bool) {
        var pending: [Int] = []
        pushChildren(of: index, onto: &pending)
        while let next = pending.popLast() {
            if visit(HtmlElement(document: document, index: next)) { return }
            pushChildren(of: next, onto: &pending)
        }
    }

    private func pushChildren(of element: Int, onto pending: inout [Int]) {
        for child in document.children[element].reversed() {
            if case let .element(child) = child { pending.append(child) }
        }
    }

    /// The text a reader sees under this element, whitespace collapsed: text meant for screen
    /// readers only is left out.
    func visibleText() -> String {
        if isVisuallyHidden { return "" }
        var text = ""
        var pending: [HtmlDocument.Child] = document.children[index].reversed()
        while let node = pending.popLast() {
            switch node {
            case let .text(run):
                text += run
            case let .element(child):
                let element = HtmlElement(document: document, index: child)
                if !element.isVisuallyHidden { pending.append(contentsOf: document.children[child].reversed()) }
            }
        }
        return text.split(whereSeparator: { isHtmlWhitespace($0) }).joined(separator: " ")
    }

    private var isVisuallyHidden: Bool {
        classes.contains { $0 == "sr-only" || $0 == "screen-reader-only" || $0 == "u-for-screen-reader" || $0 == "visually-hidden" }
    }
}

/// The whitespace `\s` matches: space, tab, line feed, vertical tab, form feed, carriage return.
private func isHtmlWhitespace(_ character: Character) -> Bool {
    character == " " || character == "\t" || character == "\n" || character == "\u{0B}" || character == "\u{0C}"
        || character == "\r" || character == "\r\n"
}

private func isWhitespaceByte(_ byte: UInt8) -> Bool {
    byte == 0x20 || byte == 0x09 || byte == 0x0A || byte == 0x0B || byte == 0x0C || byte == 0x0D
}

private let voidElements: Set<String> = [
    "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr",
]
private let rawTextElements: Set<String> = ["script", "style"]

private let commentOpen = Array("<!--".utf8)
private let commentClose = Array("-->".utf8)
private let declarationOpen = Array("<!".utf8)
private let instructionOpen = Array("<?".utf8)
private let closeOpen = Array("</".utf8)
private let lt = UInt8(ascii: "<")
private let gt = UInt8(ascii: ">")
private let equals = UInt8(ascii: "=")
private let doubleQuote = UInt8(ascii: "\"")
private let singleQuote = UInt8(ascii: "'")

/// Reads a page into a tree under a synthetic root. The page is read as UTF-8 bytes: everything
/// the reader looks for is ASCII, and text and values are decoded back out whole.
func parseHtml(_ html: String) -> HtmlDocument {
    let document = HtmlDocument()
    let bytes = Array(html.utf8)
    let count = bytes.count
    var stack: [Int] = [0]
    // How many of each tag are open, so a close tag for one that is not is answered without
    // walking the stack: a page of stray close tags is the server's to send, and it reads in
    // time linear in its length either way.
    var openCount: [String: Int] = [:]

    func text(_ from: Int, _ to: Int) -> String {
        String(decoding: bytes[from..<to], as: UTF8.self)
    }
    func push(_ element: Int) {
        stack.append(element)
        openCount[document.tags[element], default: 0] += 1
    }
    func pop() {
        let element = stack.removeLast()
        openCount[document.tags[element], default: 1] -= 1
    }
    func indexOf(_ needle: [UInt8], from: Int, ignoringCase: Bool = false) -> Int? {
        guard !needle.isEmpty, needle.count <= count else { return nil }
        var at = from
        while at + needle.count <= count {
            var matched = true
            for offset in 0..<needle.count {
                let byte = bytes[at + offset]
                let wanted = needle[offset]
                if byte != wanted && !(ignoringCase && asciiLowercased(byte) == asciiLowercased(wanted)) {
                    matched = false
                    break
                }
            }
            if matched { return at }
            at += 1
        }
        return nil
    }
    func indexOf(_ byte: UInt8, from: Int) -> Int? {
        var at = from
        while at < count {
            if bytes[at] == byte { return at }
            at += 1
        }
        return nil
    }
    func startsWith(_ prefixBytes: [UInt8], at: Int) -> Bool {
        guard at + prefixBytes.count <= count else { return false }
        for offset in 0..<prefixBytes.count where bytes[at + offset] != prefixBytes[offset] { return false }
        return true
    }

    var index = 0
    while index < count {
        guard let open = indexOf(lt, from: index) else {
            document.append(.text(decodeEntities(text(index, count))), to: stack.last!)
            break
        }
        if open > index { document.append(.text(decodeEntities(text(index, open))), to: stack.last!) }
        if startsWith(commentOpen, at: open) {
            let end = indexOf(commentClose, from: open + 4)
            index = end.map { $0 + 3 } ?? count
        } else if startsWith(declarationOpen, at: open) || startsWith(instructionOpen, at: open) {
            let end = indexOf(gt, from: open)
            index = end.map { $0 + 1 } ?? count
        } else if startsWith(closeOpen, at: open) {
            let end = indexOf(gt, from: open)
            let name = text(open + 2, end ?? count).trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if openCount[name, default: 0] > 0, let depth = stack.lastIndex(where: { document.tags[$0] == name }), depth > 0 {
                while stack.count > depth { pop() }
            }
            index = end.map { $0 + 1 } ?? count
        } else {
            let end = findTagEnd(bytes, open)
            var inside = text(open + 1, end).trimmingCharacters(in: .whitespacesAndNewlines)
            let selfClosing = inside.hasSuffix("/")
            if selfClosing { inside.removeLast() }
            let content = inside.trimmingCharacters(in: .whitespacesAndNewlines)
            let nameEnd = content.firstIndex(where: { $0.isWhitespace }) ?? content.endIndex
            let name = content[..<nameEnd].lowercased()
            let element = document.add(name, parseAttributes(String(content[nameEnd...])))
            document.append(.element(element), to: stack.last!)
            index = end + 1
            if name.isEmpty {
                // Nothing to open.
            } else if rawTextElements.contains(name) {
                if let close = indexOf(Array("</\(name)".utf8), from: index, ignoringCase: true) {
                    index = indexOf(gt, from: close).map { $0 + 1 } ?? count
                } else {
                    index = count
                }
            } else if !selfClosing && !voidElements.contains(name) {
                push(element)
            }
        }
    }
    return document
}

private func asciiLowercased(_ byte: UInt8) -> UInt8 {
    (0x41...0x5A).contains(byte) ? byte + 0x20 : byte
}

/// Where the tag opened at `open` ends: the first `>` outside a quoted value, or the end of the
/// page.
private func findTagEnd(_ bytes: [UInt8], _ open: Int) -> Int {
    var quote: UInt8?
    var index = open + 1
    while index < bytes.count {
        let byte = bytes[index]
        if let current = quote {
            if byte == current { quote = nil }
        } else if byte == doubleQuote || byte == singleQuote {
            quote = byte
        } else if byte == gt {
            return index
        }
        index += 1
    }
    return bytes.count
}

private func parseAttributes(_ source: String) -> [String: String] {
    let bytes = Array(source.utf8)
    let count = bytes.count
    func text(_ from: Int, _ to: Int) -> String { String(decoding: bytes[from..<to], as: UTF8.self) }
    var attributes: [String: String] = [:]
    var index = 0
    while index < count {
        while index < count, isWhitespaceByte(bytes[index]) { index += 1 }
        if index >= count { break }
        let nameStart = index
        while index < count, !isWhitespaceByte(bytes[index]), bytes[index] != equals { index += 1 }
        let name = text(nameStart, index).lowercased()
        while index < count, isWhitespaceByte(bytes[index]) { index += 1 }
        var value = ""
        if index < count, bytes[index] == equals {
            index += 1
            while index < count, isWhitespaceByte(bytes[index]) { index += 1 }
            if index < count, bytes[index] == doubleQuote || bytes[index] == singleQuote {
                let quote = bytes[index]
                var end = index + 1
                while end < count, bytes[end] != quote { end += 1 }
                value = text(index + 1, end)
                index = end + 1
            } else {
                var end = index
                while end < count, !(bytes[end] == 0x20 || bytes[end] == 0x09 || bytes[end] == 0x0A || bytes[end] == 0x0D) { end += 1 }
                value = text(index, end)
                index = end
            }
        }
        if !name.isEmpty { attributes[name] = decodeEntities(value) }
    }
    return attributes
}

private let entityPattern = try! NSRegularExpression(pattern: "&(#x[0-9a-fA-F]+|#[0-9]+|[a-zA-Z]+);")

private func decodeEntities(_ text: String) -> String {
    guard text.contains("&") else { return text }
    let source = text as NSString
    var decoded = ""
    var last = 0
    for match in entityPattern.matches(in: text, range: NSRange(location: 0, length: source.length)) {
        decoded += source.substring(with: NSRange(location: last, length: match.range.location - last))
        let whole = source.substring(with: match.range)
        let entity = source.substring(with: match.range(at: 1))
        decoded += replacement(entity) ?? whole
        last = match.range.location + match.range.length
    }
    decoded += source.substring(from: last)
    return decoded
}

/// What an entity stands for, or nil to leave it as written: a numeric one too large to read is
/// left as written, as Kotlin's reader leaves it.
private func replacement(_ entity: String) -> String? {
    if entity.hasPrefix("#x") {
        return Int32(entity.dropFirst(2), radix: 16).map { codePointToString(Int($0)) }
    }
    if entity.hasPrefix("#") {
        return Int32(entity.dropFirst()).map { codePointToString(Int($0)) }
    }
    switch entity {
    case "amp": return "&"
    case "lt": return "<"
    case "gt": return ">"
    case "quot": return "\""
    case "apos": return "'"
    case "nbsp": return " "
    default: return nil
    }
}

/// A code point as text; one outside Unicode reads as nothing, and a lone surrogate as the
/// replacement character, since a Swift string cannot hold one.
private func codePointToString(_ codePoint: Int) -> String {
    guard codePoint >= 0, codePoint <= 0x10FFFF else { return "" }
    guard let scalar = Unicode.Scalar(UInt32(codePoint)) else { return "\u{FFFD}" }
    return String(Character(scalar))
}
