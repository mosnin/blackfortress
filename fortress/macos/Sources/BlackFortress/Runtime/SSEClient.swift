import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// One dispatched Server-Sent Event.
struct SSEMessage: Equatable {
    var event: String
    var data: String
    var id: String?

    var json: JSONValue? { JSONValue.parse(data) }
}

/// Incremental, byte-oriented SSE parser (WHATWG event-stream rules).
///
/// We deliberately parse bytes ourselves instead of using
/// `URLSession.AsyncBytes.lines`, because `.lines` drops empty lines — and
/// empty lines are what terminate an SSE event.
struct SSEParser {
    private var line: [UInt8] = []
    private var lastWasCR = false
    private var eventType = ""
    private var dataLines: [String] = []
    private(set) var lastEventID: String?
    private(set) var retryMilliseconds: Int?

    init() {}

    /// Feeds one byte; returns a message when a blank line completes an event.
    mutating func feed(_ byte: UInt8) -> SSEMessage? {
        switch byte {
        case 0x0A: // \n
            if lastWasCR {
                lastWasCR = false
                return nil
            }
            return endLine()
        case 0x0D: // \r
            lastWasCR = true
            return endLine()
        default:
            lastWasCR = false
            line.append(byte)
            return nil
        }
    }

    /// Convenience for feeding a chunk (used by tests and non-streaming paths).
    mutating func feed<S: Sequence>(_ bytes: S) -> [SSEMessage] where S.Element == UInt8 {
        var out: [SSEMessage] = []
        for b in bytes {
            if let m = feed(b) { out.append(m) }
        }
        return out
    }

    /// Resets per-connection state (keeps last event id for reconnects).
    mutating func reset() {
        line.removeAll()
        lastWasCR = false
        eventType = ""
        dataLines.removeAll()
    }

    private mutating func endLine() -> SSEMessage? {
        let text = String(decoding: line, as: UTF8.self)
        line.removeAll(keepingCapacity: true)
        return process(line: text)
    }

    private mutating func process(line text: String) -> SSEMessage? {
        if text.isEmpty {
            defer {
                eventType = ""
                dataLines.removeAll()
            }
            guard !dataLines.isEmpty else { return nil }
            return SSEMessage(
                event: eventType.isEmpty ? "message" : eventType,
                data: dataLines.joined(separator: "\n"),
                id: lastEventID
            )
        }
        if text.hasPrefix(":") { return nil } // comment / keep-alive

        let field: String
        var value: String
        if let colon = text.firstIndex(of: ":") {
            field = String(text[text.startIndex..<colon])
            value = String(text[text.index(after: colon)...])
            if value.hasPrefix(" ") { value.removeFirst() }
        } else {
            field = text
            value = ""
        }

        switch field {
        case "event": eventType = value
        case "data": dataLines.append(value)
        case "id": if !value.contains("\u{0}") { lastEventID = value }
        case "retry": if let ms = Int(value) { retryMilliseconds = ms }
        default: break
        }
        return nil
    }
}

#if canImport(Darwin)
/// Long-lived SSE connection with automatic reconnect and backoff.
final class SSEClient {
    enum ConnectionState: Equatable {
        case connecting
        case open
        case closed
    }

    private let url: URL
    private let session: URLSession
    private var task: Task<Void, Never>?

    init(url: URL) {
        self.url = url
        let config = URLSessionConfiguration.ephemeral
        // timeoutIntervalForRequest is the *idle* timeout between bytes. bfd
        // may be quiet for a while; reconnecting after a long idle is fine.
        config.timeoutIntervalForRequest = 300
        config.timeoutIntervalForResource = 60 * 60 * 24 * 7
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        config.urlCache = nil
        self.session = URLSession(configuration: config)
    }

    deinit {
        task?.cancel()
    }

    /// Starts the read loop. Callbacks are delivered on the main actor.
    func start(
        onMessage: @escaping @MainActor (SSEMessage) -> Void,
        onState: @escaping @MainActor (ConnectionState) -> Void
    ) {
        task?.cancel()
        let url = self.url
        let session = self.session
        task = Task.detached(priority: .utility) {
            var parser = SSEParser()
            var attempt = 0
            while !Task.isCancelled {
                await onState(.connecting)
                var request = URLRequest(url: url)
                request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
                request.setValue("no-cache", forHTTPHeaderField: "Cache-Control")
                if let last = parser.lastEventID {
                    request.setValue(last, forHTTPHeaderField: "Last-Event-ID")
                }
                parser.reset()
                do {
                    let (bytes, response) = try await session.bytes(for: request)
                    if let http = response as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
                        throw URLError(.badServerResponse)
                    }
                    attempt = 0
                    await onState(.open)
                    for try await byte in bytes {
                        if let message = parser.feed(byte) {
                            await onMessage(message)
                        }
                    }
                } catch {
                    // Fall through to reconnect.
                }
                if Task.isCancelled { break }
                await onState(.closed)
                attempt += 1
                let base = Double(parser.retryMilliseconds ?? 1000) / 1000
                let delay = min(15, base * pow(2, Double(min(attempt - 1, 4))))
                try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
            }
        }
    }

    func stop() {
        task?.cancel()
        task = nil
    }
}
#endif
