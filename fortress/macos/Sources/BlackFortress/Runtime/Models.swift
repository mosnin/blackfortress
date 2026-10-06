import Foundation

// Models for the bfd control API. Primary field names follow the Go structs
// in fortress/bfd/internal/runtime (daemon.go, posture.go, events.go) and
// fortress/bfd/internal/guard/ledger.go. Every model is still built from a
// `JSONValue` and tolerates missing fields, alternate spellings and extra
// keys, so minor API drift degrades gracefully instead of failing to decode.

/// SSE frames from /v1/events are `{"type", "time", "data": <payload>}`.
/// Returns the payload (and envelope time) when `json` is such an envelope.
enum EventEnvelope {
    static func unwrap(_ json: JSONValue) -> (type: String?, time: Date?, payload: JSONValue) {
        guard let obj = json.object, let data = obj["data"], data.object != nil else {
            return (json["type"]?.string, nil, json)
        }
        return (obj["type"]?.string, obj["time"]?.date, data)
    }
}

// MARK: - Status

enum RuntimeState: Equatable {
    case starting
    case provisioning
    case running
    case degraded
    case stopping
    case error
    case other(String)

    init(_ raw: String?) {
        switch (raw ?? "").lowercased() {
        case "starting", "booting", "initializing": self = .starting
        case "provisioning": self = .provisioning
        case "stopping": self = .stopping
        case "running", "ok", "ready", "up", "healthy": self = .running
        case "degraded", "warning", "partial": self = .degraded
        case "error", "failed", "down", "stopped": self = .error
        case "": self = .starting
        default: self = .other(raw ?? "")
        }
    }

    var label: String {
        switch self {
        case .starting: return "Starting"
        case .provisioning: return "Provisioning"
        case .running: return "Running"
        case .degraded: return "Degraded"
        case .stopping: return "Stopping"
        case .error: return "Error"
        case .other(let s): return s.capitalized
        }
    }
}

struct ServiceHealth: Identifiable, Equatable {
    var name: String
    var state: String
    var detail: String?

    var id: String { name }

    var isHealthy: Bool {
        ["up", "running", "ok", "healthy", "ready", "enabled"].contains(state.lowercased())
    }

    var isStarting: Bool {
        ["starting", "booting", "pending", "initializing", "waiting"].contains(state.lowercased())
    }

    /// Optional services (e.g. Chrome for PDF export) that may legitimately be absent.
    var isOptionalAbsent: Bool {
        ["absent", "disabled", "not_found", "missing", "unavailable", "skipped", "optional"]
            .contains(state.lowercased())
    }

    var displayName: String {
        switch name.lowercased() {
        case "postgres", "postgresql": return "PostgreSQL"
        case "probod", "probo": return "Probo server"
        case "storage", "objects", "s3", "object_storage": return "Object storage"
        case "mail", "smtp", "mailsink", "mail_sink": return "Mail sink"
        case "chrome", "chromium": return "Chrome (PDF export)"
        case "api", "control", "bfd": return "Control API"
        default: return name.replacingOccurrences(of: "_", with: " ").capitalized
        }
    }
}

struct BFDStatus: Equatable {
    var state: RuntimeState
    var services: [ServiceHealth]
    var consoleURL: URL?
    var mcpURL: URL?
    var controlURL: URL?
    var organizationID: String?
    var dataDir: String?
    var version: String?
    /// `error` from bfd (set when state is degraded/error).
    var message: String?

    init(json: JSONValue) {
        state = RuntimeState(json.first("state", "status")?.string)
        consoleURL = json.first("console_url", "consoleUrl", "consoleURL").flatMap { URL(string: $0.string ?? "") }
        mcpURL = json.first("mcp_url", "mcpUrl", "mcpURL").flatMap { URL(string: $0.string ?? "") }
        controlURL = json.first("control_url", "controlUrl").flatMap { URL(string: $0.string ?? "") }
        organizationID = json.first("organization_id", "organizationId", "org_id")?.string
        dataDir = json.first("data_dir", "dataDir")?.string
        version = json.first("version", "bfd_version")?.string
        message = json.first("error", "message", "detail")?.string

        var list: [ServiceHealth] = []
        if let dict = json["services"]?.object {
            for (name, value) in dict {
                list.append(ServiceHealth.from(name: name, value: value))
            }
        } else if let items = json["services"]?.array {
            for item in items {
                let name = item.first("name", "id", "service")?.string ?? "service"
                list.append(ServiceHealth.from(name: name, value: item))
            }
        }
        services = list.sorted { ServiceHealth.order($0.name) < ServiceHealth.order($1.name) }
    }

    var unhealthyServices: [ServiceHealth] {
        services.filter { !$0.isHealthy && !$0.isOptionalAbsent && !$0.isStarting }
    }
}

extension ServiceHealth {
    static func from(name: String, value: JSONValue) -> ServiceHealth {
        if let s = value.string, value.object == nil {
            return ServiceHealth(name: name, state: s, detail: nil)
        }
        let state = value.first("state", "status", "health")?.string
            ?? (value.first("up", "healthy", "ok")?.bool == true ? "up" : "unknown")
        let detail = value.first("detail", "message", "error", "addr", "address", "url")?.string
        return ServiceHealth(name: name, state: state, detail: detail)
    }

    static func order(_ name: String) -> Int {
        let known = ["postgres", "storage", "mail", "probod", "chrome"]
        return known.firstIndex(of: name.lowercased()) ?? known.count
    }
}

// MARK: - Posture

/// Counts of items (controls or measures) by state.
struct StateCounts: Equatable {
    var byState: [String: Int] = [:]
    var explicitTotal: Int?

    var total: Int {
        if let t = explicitTotal { return t }
        return byState.values.reduce(0, +)
    }

    static let doneStates: Set<String> = [
        "implemented", "done", "compliant", "passing", "passed", "effective",
        "complete", "completed", "met", "satisfied", "ok", "covered",
    ]
    static let excludedStates: Set<String> = ["not_applicable", "na", "n/a", "excluded"]
    static let metaKeys: Set<String> = ["total", "count", "by_state", "byState", "states"]

    var done: Int {
        byState.reduce(0) { acc, kv in
            StateCounts.doneStates.contains(kv.key.lowercased()) ? acc + kv.value : acc
        }
    }

    var excluded: Int {
        byState.reduce(0) { acc, kv in
            StateCounts.excludedStates.contains(kv.key.lowercased()) ? acc + kv.value : acc
        }
    }

    /// Fraction of applicable items in a done state, or nil when empty.
    var ratio: Double? {
        let applicable = total - excluded
        guard applicable > 0 else { return nil }
        return min(1, max(0, Double(done) / Double(applicable)))
    }

    init() {}

    init?(json: JSONValue?) {
        guard let json else { return nil }
        if let n = json.int, json.object == nil {
            explicitTotal = n
            return
        }
        guard let dict = json.object else { return nil }
        explicitTotal = dict["total"]?.int ?? dict["count"]?.int
        let nested = (dict["by_state"] ?? dict["byState"] ?? dict["states"])?.object
        let source = nested ?? dict
        for (key, value) in source where !StateCounts.metaKeys.contains(key) {
            if let n = value.int { byState[key] = n }
        }
    }

    /// Builds counts from flat keys such as `controls_total`, `controls_implemented`.
    init?(prefix: String, in object: [String: JSONValue]) {
        let p = prefix + "_"
        var found = false
        for (key, value) in object where key.hasPrefix(p) {
            guard let n = value.int else { continue }
            let state = String(key.dropFirst(p.count))
            found = true
            if state == "total" || state == "count" {
                explicitTotal = n
            } else {
                byState[state] = n
            }
        }
        if !found { return nil }
    }
}

struct FrameworkPosture: Identifiable, Equatable {
    var id: String
    var name: String
    var controls: StateCounts
    var measures: StateCounts
    var controlsWithoutMeasure: Int?
    var explicitScore: Double?

    init(json: JSONValue, fallbackID: String) {
        let obj = json.object ?? [:]
        id = json.first("id", "key", "slug", "framework_id", "code")?.string ?? fallbackID
        name = json.first("name", "title", "framework", "label")?.string ?? id
        // bfd: "controls": 62 (total) plus flat "controls_implemented",
        // "controls_in_progress", "controls_not_started",
        // "controls_without_measure". "controls_without_measure" overlaps
        // with not_started, so it is kept apart from the state breakdown.
        var c = StateCounts(json: json["controls"]) ?? StateCounts()
        if let flat = StateCounts(prefix: "controls", in: obj) {
            for (state, n) in flat.byState where state != "without_measure" {
                c.byState[state] = n
            }
            if c.explicitTotal == nil { c.explicitTotal = flat.explicitTotal }
        }
        controls = c
        controlsWithoutMeasure = json.first("controls_without_measure")?.int
        measures = StateCounts(json: json["measures"])
            ?? StateCounts(prefix: "measures", in: obj)
            ?? StateCounts()
        if let raw = json.first("score", "percent", "compliance", "coverage", "progress")?.double {
            explicitScore = raw > 1 ? raw / 100 : raw
        }
    }

    /// 0…1 readiness score: explicit score if bfd provides one, else the
    /// share of implemented controls (falling back to measures).
    var score: Double? {
        if let s = explicitScore { return min(1, max(0, s)) }
        return controls.ratio ?? measures.ratio
    }
}

/// `guardrails` block of /v1/posture: counters over the last 24 hours.
struct GuardrailSummary: Equatable {
    var last24h: Int
    var blocked24h: Int
    var asked24h: Int
    var recorded24h: Int

    init(json: JSONValue?) {
        last24h = json?.first("last_24h", "last24h", "total")?.int ?? 0
        blocked24h = json?.first("blocked_24h", "blocked24h", "blocked")?.int ?? 0
        asked24h = json?.first("asked_24h", "asked24h", "asked")?.int ?? 0
        recorded24h = json?.first("recorded_24h", "recorded24h", "recorded")?.int ?? 0
    }
}

struct Posture: Equatable {
    var frameworks: [FrameworkPosture]
    var guardrails: GuardrailSummary
    var updatedAt: Date?

    init(json: JSONValue) {
        var list: [FrameworkPosture] = []
        let source = json.first("frameworks", "framework_posture", "summary")
        if let items = source?.array {
            for (index, item) in items.enumerated() {
                list.append(FrameworkPosture(json: item, fallbackID: "framework-\(index)"))
            }
        } else if let dict = source?.object {
            for (key, value) in dict {
                var fw = FrameworkPosture(json: value, fallbackID: key)
                if value.first("name", "title", "framework", "label") == nil { fw.name = key }
                list.append(fw)
            }
            list.sort { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        }
        frameworks = list

        guardrails = GuardrailSummary(json: json.first("guardrails", "guardrail_summary"))
        updatedAt = json.first("updated_at", "generated_at", "updatedAt", "time")?.date
    }
}

// MARK: - Ledger / guardrail events

enum Decision: Equatable, Hashable {
    case block
    case ask
    case allow
    case record
    case flag
    case other(String)

    init(_ raw: String?) {
        switch (raw ?? "").lowercased() {
        case "block", "blocked", "deny", "denied": self = .block
        case "ask", "prompt", "confirm": self = .ask
        case "allow", "allowed", "approve", "approved", "pass": self = .allow
        case "record", "recorded", "log", "logged", "": self = .record
        case "flag", "flagged", "warn", "warning": self = .flag
        default: self = .other(raw ?? "")
        }
    }

    var label: String {
        switch self {
        case .block: return "Block"
        case .ask: return "Ask"
        case .allow: return "Allow"
        case .record: return "Record"
        case .flag: return "Flag"
        case .other(let s): return s.capitalized
        }
    }

    /// True for decisions that should draw attention (guardrail hits).
    var isHit: Bool {
        switch self {
        case .block, .ask, .flag: return true
        default: return false
        }
    }
}

enum EntryKind: String, Equatable {
    case guardrail
    case evidence
}

struct LedgerEntry: Identifiable, Equatable {
    var id: String
    var kind: EntryKind
    var time: Date?
    var sessionID: String?
    var agent: String?
    var tool: String?
    var target: String?
    var decision: Decision
    var controls: [String]
    /// Guardrail rule ids that matched (`rules`).
    var rules: [String]
    var reason: String?
    /// PreToolUse | PostToolUse | SessionStart | SessionEnd | MCPToolCall
    var event: String?
    /// blocked | awaiting-approval | allowed | executed | approved-and-executed
    var outcome: String?
    var cwd: String?
    var repo: String?
    var hash: String?
    var prevHash: String?

    var ruleID: String? { rules.first }

    init?(json raw: JSONValue, kind defaultKind: EntryKind) {
        // Unwrap common envelopes: {"entry": {...}}, {"data": {...}}, {"event": {...}}.
        var json = raw
        for key in ["entry", "data", "event", "payload"] {
            if let inner = raw[key], inner.object != nil { json = inner; break }
        }
        guard json.object != nil else { return nil }

        time = json.first("time", "ts", "timestamp", "at", "created_at", "recorded_at")?.date
        sessionID = json.first("session_id", "sessionId", "session")?.string
        agent = json.first("agent", "agent_name", "client")?.string
        tool = json.first("tool", "tool_name", "toolName")?.string
        decision = Decision(json.first("decision", "action")?.string)
        rules = json.first("rules", "rule_ids", "rule_id", "rule")?.stringList ?? []
        reason = json.first("reason", "message", "summary", "description")?.string
        event = json.first("event", "hook_event", "hook_event_name")?.string
        outcome = json.first("outcome", "result")?.string
        cwd = json.first("cwd")?.string
        repo = json.first("repo")?.string
        hash = json.first("hash")?.string
        prevHash = json.first("prev_hash", "prevHash")?.string
        controls = json.first("controls", "control_refs", "controlRefs", "control_references", "refs")?.stringList ?? []

        var tgt = json.first("target", "path", "file_path", "command", "url")?.string
        if tgt == nil, let input = json.first("tool_input", "toolInput", "input") {
            tgt = input.first("file_path", "path", "command", "url", "pattern")?.string
        }
        target = tgt

        if let k = json.first("kind", "type")?.string, let parsed = EntryKind(rawValue: k.lowercased()) {
            kind = parsed
        } else {
            kind = defaultKind
        }

        // The ledger hash is unique and stable, and the same entry arrives
        // both via SSE and /v1/ledger, so it is the natural identity.
        if let explicit = json.first("hash", "id", "seq", "entry_id", "uuid")?.string, !explicit.isEmpty {
            id = explicit
        } else {
            let t = time.map { String($0.timeIntervalSince1970) } ?? "-"
            id = [t, sessionID ?? "", event ?? "", tool ?? "", target ?? "", decision.label].joined(separator: "|")
        }
    }

    var isGuardrailHit: Bool { kind == .guardrail || decision.isHit }

    var outcomeLabel: String? {
        guard let outcome, !outcome.isEmpty else { return nil }
        return outcome.replacingOccurrences(of: "-", with: " ").capitalized
    }
}

/// GET /v1/ledger/verify → {"valid": bool, "entries": n, "error"?: string}
struct LedgerVerification: Equatable {
    var valid: Bool
    var entries: Int
    var error: String?

    init(json: JSONValue) {
        valid = json["valid"]?.bool ?? false
        entries = json.first("entries", "count")?.int ?? 0
        error = json["error"]?.string
    }
}

/// Parses `/v1/ledger` responses: a bare array, or an object wrapping it.
enum LedgerResponse {
    static func entries(from json: JSONValue) -> [LedgerEntry] {
        let items = json.first("entries", "items", "ledger", "events", "data")?.array
            ?? json.array
            ?? []
        return items.compactMap { LedgerEntry(json: $0, kind: .evidence) }
    }
}
