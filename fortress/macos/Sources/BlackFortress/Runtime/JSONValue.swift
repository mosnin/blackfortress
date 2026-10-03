import Foundation

/// A loosely-typed JSON value. The bfd control API is evolving in parallel
/// with this app, so every model is built from `JSONValue` with tolerant
/// accessors instead of strict `Codable` key mappings. Missing keys, extra
/// keys, and minor type differences (number vs. numeric string) never fail
/// decoding.
enum JSONValue: Decodable, Equatable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let value = try? container.decode(Bool.self) {
            // Bool must be tried before Double so `true` is not read as 1.0.
            self = .bool(value)
        } else if let value = try? container.decode(Double.self) {
            self = .number(value)
        } else if let value = try? container.decode(String.self) {
            self = .string(value)
        } else if let value = try? container.decode([JSONValue].self) {
            self = .array(value)
        } else if let value = try? container.decode([String: JSONValue].self) {
            self = .object(value)
        } else {
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "Unsupported JSON value"
            )
        }
    }

    /// Parses raw bytes. Returns nil for empty or invalid input.
    static func parse(_ data: Data) -> JSONValue? {
        guard !data.isEmpty else { return nil }
        return try? JSONDecoder().decode(JSONValue.self, from: data)
    }

    static func parse(_ text: String) -> JSONValue? {
        parse(Data(text.utf8))
    }

    // MARK: - Accessors

    subscript(key: String) -> JSONValue? {
        if case .object(let dict) = self { return dict[key] }
        return nil
    }

    /// Returns the first present, non-null value among `keys`.
    func first(_ keys: String...) -> JSONValue? {
        first(keys)
    }

    func first(_ keys: [String]) -> JSONValue? {
        guard case .object(let dict) = self else { return nil }
        for key in keys {
            if let value = dict[key], value != .null { return value }
        }
        return nil
    }

    var object: [String: JSONValue]? {
        if case .object(let dict) = self { return dict }
        return nil
    }

    var array: [JSONValue]? {
        if case .array(let items) = self { return items }
        return nil
    }

    var string: String? {
        switch self {
        case .string(let s): return s
        case .number(let n):
            if n.rounded() == n, abs(n) < 1e15 { return String(Int64(n)) }
            return String(n)
        case .bool(let b): return b ? "true" : "false"
        default: return nil
        }
    }

    var double: Double? {
        switch self {
        case .number(let n): return n
        case .string(let s): return Double(s.trimmingCharacters(in: .whitespaces))
        case .bool(let b): return b ? 1 : 0
        default: return nil
        }
    }

    var int: Int? {
        guard let d = double, d.isFinite else { return nil }
        return Int(d)
    }

    var bool: Bool? {
        switch self {
        case .bool(let b): return b
        case .number(let n): return n != 0
        case .string(let s):
            switch s.lowercased() {
            case "true", "yes", "1", "on": return true
            case "false", "no", "0", "off": return false
            default: return nil
            }
        default: return nil
        }
    }

    /// A list of strings from an array of strings, an array of objects with
    /// an id-like field, or a comma-separated string.
    var stringList: [String] {
        switch self {
        case .array(let items):
            return items.compactMap { item in
                if let s = item.string { return s }
                return item.first("ref", "id", "control", "name", "key")?.string
            }
        case .string(let s):
            return s.split(separator: ",")
                .map { $0.trimmingCharacters(in: .whitespaces) }
                .filter { !$0.isEmpty }
        default:
            return []
        }
    }

    /// Interprets the value as a timestamp: RFC 3339 / ISO 8601 string
    /// (any number of fractional digits), or Unix seconds / milliseconds.
    var date: Date? {
        switch self {
        case .number(let n):
            return n > 1e12 ? Date(timeIntervalSince1970: n / 1000) : Date(timeIntervalSince1970: n)
        case .string(let s):
            return JSONDates.parse(s)
        default:
            return nil
        }
    }
}

enum JSONDates {
    private static let fractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let plain: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    static func parse(_ raw: String) -> Date? {
        let s = raw.trimmingCharacters(in: .whitespaces)
        if s.isEmpty { return nil }
        if let d = fractional.date(from: s) { return d }
        if let d = plain.date(from: s) { return d }
        // Go's RFC3339Nano emits up to 9 fractional digits, which some
        // formatter versions reject. Drop the fraction and retry.
        if let dot = s.firstIndex(of: "."), let tIndex = s.firstIndex(of: "T"), dot > tIndex {
            var end = s.index(after: dot)
            while end < s.endIndex, s[end].isNumber { end = s.index(after: end) }
            let trimmed = String(s[s.startIndex..<dot]) + String(s[end...])
            if let d = plain.date(from: trimmed) { return d }
        }
        if let seconds = Double(s) {
            return JSONValue.number(seconds).date
        }
        return nil
    }
}
