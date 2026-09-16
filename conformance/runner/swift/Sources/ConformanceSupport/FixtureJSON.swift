import Foundation

/// A JSON document as the runner reads a fixture: objects keep their members in the order the
/// file lists them, and numbers keep the text they were written as, so what the generator
/// emits follows the model's own order and never depends on how a platform bridges a number.
public enum FixtureJSON: Equatable, Sendable {
    case null
    case bool(Bool)
    case number(String)
    case string(String)
    case array([FixtureJSON])
    case object([(String, FixtureJSON)])

    public static func == (lhs: FixtureJSON, rhs: FixtureJSON) -> Bool {
        switch (lhs, rhs) {
        case (.null, .null): return true
        case let (.bool(a), .bool(b)): return a == b
        case let (.number(a), .number(b)): return a == b
        case let (.string(a), .string(b)): return a == b
        case let (.array(a), .array(b)): return a == b
        case let (.object(a), .object(b)):
            return a.count == b.count && zip(a, b).allSatisfy { $0.0 == $1.0 && $0.1 == $1.1 }
        default: return false
        }
    }

    /// A member of an object, or nil for anything else or a member it does not have.
    public subscript(key: String) -> FixtureJSON? {
        guard case let .object(members) = self else { return nil }
        return members.first { $0.0 == key }?.1
    }

    public var object: [(String, FixtureJSON)]? {
        if case let .object(members) = self { return members }
        return nil
    }

    public var array: [FixtureJSON]? {
        if case let .array(elements) = self { return elements }
        return nil
    }

    public var string: String? {
        if case let .string(value) = self { return value }
        return nil
    }

    public var bool: Bool? {
        if case let .bool(value) = self { return value }
        return nil
    }

    public var int: Int? {
        if case let .number(text) = self { return Int(text) }
        return nil
    }

    /// The member keys, in order.
    public var keys: [String] { object?.map(\.0) ?? [] }

    public static func parse(_ text: String) throws -> FixtureJSON {
        var parser = Parser(scalars: Array(text.unicodeScalars))
        parser.skipWhitespace()
        let value = try parser.value()
        parser.skipWhitespace()
        guard parser.index == parser.scalars.count else {
            throw FixtureError("FixtureJSON has trailing content at offset \(parser.index)")
        }
        return value
    }

    private struct Parser {
        let scalars: [Unicode.Scalar]
        var index = 0

        mutating func skipWhitespace() {
            while index < scalars.count, [" ", "\n", "\r", "\t"].contains(scalars[index]) { index += 1 }
        }

        mutating func value() throws -> FixtureJSON {
            guard index < scalars.count else { throw FixtureError("FixtureJSON ends early") }
            switch scalars[index] {
            case "{": return try objectValue()
            case "[": return try arrayValue()
            case "\"": return .string(try stringValue())
            case "t": try literal("true"); return .bool(true)
            case "f": try literal("false"); return .bool(false)
            case "n": try literal("null"); return .null
            default: return try numberValue()
            }
        }

        mutating func literal(_ word: String) throws {
            for scalar in word.unicodeScalars {
                guard index < scalars.count, scalars[index] == scalar else {
                    throw FixtureError("FixtureJSON has an unexpected token at offset \(index)")
                }
                index += 1
            }
        }

        mutating func objectValue() throws -> FixtureJSON {
            index += 1
            var members: [(String, FixtureJSON)] = []
            skipWhitespace()
            if index < scalars.count, scalars[index] == "}" { index += 1; return .object(members) }
            while true {
                skipWhitespace()
                guard index < scalars.count, scalars[index] == "\"" else {
                    throw FixtureError("FixtureJSON object key expected at offset \(index)")
                }
                let key = try stringValue()
                skipWhitespace()
                guard index < scalars.count, scalars[index] == ":" else {
                    throw FixtureError("FixtureJSON ':' expected at offset \(index)")
                }
                index += 1
                skipWhitespace()
                members.append((key, try value()))
                skipWhitespace()
                guard index < scalars.count else { throw FixtureError("FixtureJSON object not closed") }
                if scalars[index] == "," { index += 1; continue }
                if scalars[index] == "}" { index += 1; return .object(members) }
                throw FixtureError("FixtureJSON ',' or '}' expected at offset \(index)")
            }
        }

        mutating func arrayValue() throws -> FixtureJSON {
            index += 1
            var elements: [FixtureJSON] = []
            skipWhitespace()
            if index < scalars.count, scalars[index] == "]" { index += 1; return .array(elements) }
            while true {
                skipWhitespace()
                elements.append(try value())
                skipWhitespace()
                guard index < scalars.count else { throw FixtureError("FixtureJSON array not closed") }
                if scalars[index] == "," { index += 1; continue }
                if scalars[index] == "]" { index += 1; return .array(elements) }
                throw FixtureError("FixtureJSON ',' or ']' expected at offset \(index)")
            }
        }

        mutating func stringValue() throws -> String {
            index += 1
            var out = String.UnicodeScalarView()
            while index < scalars.count {
                let scalar = scalars[index]
                index += 1
                switch scalar {
                case "\"":
                    return String(out)
                case "\\":
                    guard index < scalars.count else { throw FixtureError("FixtureJSON escape not finished") }
                    let escaped = scalars[index]
                    index += 1
                    switch escaped {
                    case "\"": out.append("\"")
                    case "\\": out.append("\\")
                    case "/": out.append("/")
                    case "b": out.append("\u{08}")
                    case "f": out.append("\u{0C}")
                    case "n": out.append("\n")
                    case "r": out.append("\r")
                    case "t": out.append("\t")
                    case "u":
                        var code = try hex4()
                        if (0xD800...0xDBFF).contains(code), index + 1 < scalars.count,
                           scalars[index] == "\\", scalars[index + 1] == "u" {
                            index += 2
                            let low = try hex4()
                            code = 0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00)
                        }
                        guard let decoded = Unicode.Scalar(code) else {
                            throw FixtureError("FixtureJSON has an invalid \\u escape")
                        }
                        out.append(decoded)
                    default:
                        throw FixtureError("FixtureJSON has an invalid escape at offset \(index)")
                    }
                default:
                    out.append(scalar)
                }
            }
            throw FixtureError("FixtureJSON string not closed")
        }

        mutating func hex4() throws -> UInt32 {
            guard index + 4 <= scalars.count else { throw FixtureError("FixtureJSON \\u escape too short") }
            var code: UInt32 = 0
            for _ in 0..<4 {
                guard let digit = UInt32(String(scalars[index]), radix: 16) else {
                    throw FixtureError("FixtureJSON \\u escape is not hex")
                }
                code = code * 16 + digit
                index += 1
            }
            return code
        }

        mutating func numberValue() throws -> FixtureJSON {
            let start = index
            while index < scalars.count, "+-0123456789.eE".unicodeScalars.contains(scalars[index]) { index += 1 }
            guard index > start else { throw FixtureError("FixtureJSON has an unexpected character at offset \(index)") }
            return .number(String(String.UnicodeScalarView(scalars[start..<index])))
        }
    }
}

extension FixtureJSON {
    /// The value written back out as compact JSON, numbers as they were read.
    public var text: String {
        switch self {
        case .null: return "null"
        case let .bool(value): return value ? "true" : "false"
        case let .number(text): return text
        case let .string(value): return Self.quoted(value)
        case let .array(elements): return "[" + elements.map(\.text).joined(separator: ",") + "]"
        case let .object(members): return "{" + members.map { Self.quoted($0.0) + ":" + $0.1.text }.joined(separator: ",") + "}"
        }
    }

    static func quoted(_ value: String) -> String {
        var out = "\""
        for scalar in value.unicodeScalars {
            switch scalar {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            case "\t": out += "\\t"
            case _ where scalar.value < 0x20: out += String(format: "\\u%04x", scalar.value)
            default: out.unicodeScalars.append(scalar)
            }
        }
        return out + "\""
    }
}

/// A fixture that will not parse.
public struct FixtureError: Error, CustomStringConvertible {
    public let description: String
    public init(_ description: String) { self.description = description }
}
