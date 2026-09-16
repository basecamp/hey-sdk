import Foundation

/// A string the model marks as sensitive — an email address, a token — which prints as
/// `[REDACTED]`, so that printing anything holding one cannot put it in a log. The value is a
/// call to ``expose()`` away, and goes over the wire as the plain string it is.
public struct SensitiveString: Codable, Sendable, Hashable, CustomStringConvertible, CustomDebugStringConvertible,
    ExpressibleByStringLiteral
{
    private let value: String

    public init(_ value: String) {
        self.value = value
    }

    public init(stringLiteral value: String) {
        self.value = value
    }

    public init(from decoder: any Decoder) throws {
        value = try String(from: decoder)
    }

    public func encode(to encoder: any Encoder) throws {
        try value.encode(to: encoder)
    }

    /// The string itself.
    public func expose() -> String { value }

    /// Whether there is nothing behind the redaction.
    public var isEmpty: Bool { value.isEmpty }

    public var description: String { "[REDACTED]" }
    public var debugDescription: String { "[REDACTED]" }
}

/// Any JSON value, for a member the model leaves free-form.
public enum JSONValue: Codable, Sendable, Equatable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    public init(from decoder: any Decoder) throws {
        if let container = try? decoder.container(keyedBy: AnyKey.self) {
            var object: [String: JSONValue] = [:]
            for key in container.allKeys {
                object[key.stringValue] = try container.decode(JSONValue.self, forKey: key)
            }
            self = .object(object)
        } else if var container = try? decoder.unkeyedContainer() {
            var array: [JSONValue] = []
            while !container.isAtEnd {
                array.append(try container.decode(JSONValue.self))
            }
            self = .array(array)
        } else {
            let container = try decoder.singleValueContainer()
            if container.decodeNil() {
                self = .null
            } else if let bool = try? container.decode(Bool.self) {
                self = .bool(bool)
            } else if let number = try? container.decode(Double.self) {
                self = .number(number)
            } else {
                self = .string(try container.decode(String.self))
            }
        }
    }

    public func encode(to encoder: any Encoder) throws {
        switch self {
        case .null:
            var container = encoder.singleValueContainer()
            try container.encodeNil()
        case let .bool(value):
            var container = encoder.singleValueContainer()
            try container.encode(value)
        case let .number(value):
            var container = encoder.singleValueContainer()
            try container.encode(value)
        case let .string(value):
            var container = encoder.singleValueContainer()
            try container.encode(value)
        case let .array(values):
            var container = encoder.unkeyedContainer()
            for value in values { try container.encode(value) }
        case let .object(members):
            var container = encoder.container(keyedBy: AnyKey.self)
            for (key, value) in members { try container.encode(value, forKey: AnyKey(key)) }
        }
    }

    private struct AnyKey: CodingKey {
        let stringValue: String
        var intValue: Int? { nil }
        init(_ string: String) { stringValue = string }
        init?(stringValue: String) { self.stringValue = stringValue }
        init?(intValue: Int) { stringValue = String(intValue) }
    }
}

/// Holds a model member whose type is the model itself — a recording's parent recording —
/// which a struct cannot contain directly. It decodes and encodes as the value it holds.
public final class Indirect<Value: Codable & Sendable & Equatable>: Codable, Sendable, Equatable {
    public let value: Value

    public init(_ value: Value) {
        self.value = value
    }

    public required init(from decoder: any Decoder) throws {
        value = try Value(from: decoder)
    }

    public func encode(to encoder: any Encoder) throws {
        try value.encode(to: encoder)
    }

    public static func == (lhs: Indirect, rhs: Indirect) -> Bool { lhs.value == rhs.value }
}
