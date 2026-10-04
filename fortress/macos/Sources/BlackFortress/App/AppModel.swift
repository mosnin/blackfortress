import SwiftUI
import AppKit
import Combine

enum SidebarItem: String, CaseIterable, Identifiable {
    case overview
    case activity
    case frameworks
    case checks
    case console
    case settings

    var id: String { rawValue }

    var title: String {
        switch self {
        case .overview: return "Overview"
        case .activity: return "Agent Activity"
        case .frameworks: return "Frameworks"
        case .checks: return "Automated Checks"
        case .console: return "Console"
        case .settings: return "Settings"
        }
    }

    var symbol: String {
        switch self {
        case .overview: return "square.grid.2x2"
        case .activity: return "bolt.horizontal.circle"
        case .frameworks: return "checklist"
        case .checks: return "checkmark.shield"
        case .console: return "globe"
        case .settings: return "gearshape"
        }
    }
}

/// Central app state. Owns the daemon supervisor, the API client, the SSE
/// stream, and everything the views render.
@MainActor
final class AppModel: ObservableObject {
    static let shared = AppModel()

    enum Connection: Equatable {
        case connecting
        case connected
        case unreachable
    }

    @Published var section: SidebarItem = .overview
    @Published private(set) var status: BFDStatus?
    @Published private(set) var posture: Posture?
    @Published private(set) var checks: ChecksSummary?
    @Published private(set) var runningChecks = false
    @Published private(set) var activity: [LedgerEntry] = []
    @Published private(set) var connection: Connection = .connecting
    @Published private(set) var streamOpen = false
    @Published private(set) var lastUpdated: Date?
    @Published var transientMessage: String?
    @Published private(set) var verification: LedgerVerification?
    @Published private(set) var verifyingLedger = false
    @Published private(set) var verificationError: String?
    @Published private(set) var installOutput: AgentConfig.CommandOutput?
    @Published private(set) var installingClaude = false

    let supervisor: DaemonSupervisor
    let client: BFDClient
    let paths: RuntimePaths
    let consoleBrowser = ConsoleBrowser()

    private var sse: SSEClient?
    private var pollTask: Task<Void, Never>?
    private var supervisorCancellable: AnyCancellable?
    private var consecutiveFailures = 0
    private var started = false
    private let maxActivity = 1000

    init(paths: RuntimePaths = .current) {
        self.paths = paths
        self.client = BFDClient()
        self.supervisor = DaemonSupervisor(paths: paths, client: client)
        // Re-publish supervisor changes so views observing AppModel refresh.
        supervisorCancellable = supervisor.objectWillChange.sink { [weak self] _ in
            self?.objectWillChange.send()
        }
    }

    // MARK: - Derived state

    var health: HealthLevel {
        guard connection == .connected, let status else {
            switch supervisor.mode {
            case .failed: return .error
            case .restarting: return .warning
            case .binaryMissing: return connection == .unreachable ? .error : .starting
            default: return .starting
            }
        }
        switch status.state {
        case .running:
            return status.unhealthyServices.isEmpty ? .good : .warning
        case .starting, .provisioning: return .starting
        case .degraded, .stopping, .other: return .warning
        case .error: return .error
        }
    }

    var healthTitle: String {
        if connection != .connected {
            switch supervisor.mode {
            case .binaryMissing: return "Runtime not running"
            case .restarting: return "Restarting runtime"
            case .failed: return "Runtime failed"
            default: return "Starting runtime"
            }
        }
        guard let status else { return "Connecting" }
        switch status.state {
        case .running:
            return status.unhealthyServices.isEmpty ? "All systems protected" : "Running with issues"
        default:
            return status.state.label
        }
    }

    var guardrailHits: [LedgerEntry] {
        activity.filter { $0.isGuardrailHit }
    }

    // MARK: - Lifecycle

    func start() {
        guard !started else { return }
        started = true
        supervisor.start()
        startStream()
        startPolling()
    }

    func shutdown() async {
        pollTask?.cancel()
        sse?.stop()
        await supervisor.stop()
    }

    private func startStream() {
        let stream = SSEClient(url: client.eventsURL)
        sse = stream
        stream.start(
            onMessage: { [weak self] message in
                self?.handle(message)
            },
            onState: { [weak self] state in
                guard let self else { return }
                let wasOpen = self.streamOpen
                self.streamOpen = (state == .open)
                if state == .open && !wasOpen {
                    Task { await self.refreshAll() }
                }
            }
        )
    }

    /// Polls /v1/status: quickly while not connected, slowly as a safety net
    /// while the SSE stream is open.
    private func startPolling() {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { return }
                await self.refreshStatus()
                let interval: UInt64 = (self.connection == .connected && self.streamOpen) ? 15 : 2
                try? await Task.sleep(nanoseconds: interval * 1_000_000_000)
            }
        }
    }

    func refreshAll() async {
        await refreshStatus()
        await refreshPosture()
        await refreshLedger()
        await refreshChecks()
    }

    func refreshChecks() async {
        if let c = try? await client.checks() {
            checks = c
        }
    }

    /// Runs every provider that has local credentials, now.
    func runChecksNow() {
        guard !runningChecks else { return }
        guard let token = AgentConfig.personalAPIKey(dataDir: paths.dataDir) else {
            flash("Runtime not set up yet")
            return
        }
        runningChecks = true
        Task {
            do {
                checks = try await client.runChecks(token: token)
                flash("Checks finished")
            } catch {
                flash("Checks failed: \(error.localizedDescription)")
            }
            runningChecks = false
            await refreshPosture()
        }
    }

    func refreshStatus() async {
        do {
            let newStatus = try await client.status()
            let wasConnected = connection == .connected
            status = newStatus
            connection = .connected
            consecutiveFailures = 0
            lastUpdated = Date()
            supervisor.noteExternalRuntime()
            if !wasConnected {
                await refreshPosture()
                await refreshLedger()
            }
        } catch {
            consecutiveFailures += 1
            if consecutiveFailures >= 3 {
                connection = .unreachable
                supervisor.externalRuntimeLost()
            } else if connection != .connected {
                connection = .connecting
            }
        }
    }

    func refreshPosture() async {
        if let p = try? await client.posture() {
            posture = p
        }
    }

    func refreshLedger() async {
        if let entries = try? await client.ledger(limit: 300) {
            mergeActivity(entries)
        }
    }

    // MARK: - SSE handling

    private func handle(_ message: SSEMessage) {
        guard let frame = message.json else { return }
        // bfd frames: `event: <type>` + `data: {"type","time","data": <payload>}`.
        let envelope = EventEnvelope.unwrap(frame)
        let json = envelope.payload
        var type = message.event.lowercased()
        if type == "message", let t = envelope.type {
            type = t.lowercased()
        }
        lastUpdated = Date()

        switch type {
        case "status":
            status = BFDStatus(json: json)
            connection = .connected
            consecutiveFailures = 0
        case "guardrail":
            if let entry = LedgerEntry(json: json, kind: .guardrail) {
                let isNew = !activity.contains { $0.id == entry.id }
                mergeActivity([entry])
                if isNew && entry.decision == .block {
                    NotificationManager.shared.notifyBlocked(entry)
                }
            }
        case "evidence":
            if let entry = LedgerEntry(json: json, kind: .evidence) {
                mergeActivity([entry])
            }
        case "checks":
            checks = ChecksSummary(json: json)
        case "posture":
            let p = Posture(json: json)
            if p.frameworks.isEmpty {
                // Event may only signal a change; fetch the full summary.
                Task { await refreshPosture() }
            } else {
                posture = p
            }
        default:
            break // unknown event types are ignored
        }
    }

    private func mergeActivity(_ entries: [LedgerEntry]) {
        guard !entries.isEmpty else { return }
        var byID: [String: LedgerEntry] = [:]
        for e in activity { byID[e.id] = e }
        for e in entries {
            if let existing = byID[e.id], existing.kind == .guardrail, e.kind == .evidence {
                // Keep the guardrail classification when the same action is
                // later recorded as evidence.
                var merged = e
                merged.kind = .guardrail
                byID[e.id] = merged
            } else {
                byID[e.id] = e
            }
        }
        var list = Array(byID.values)
        list.sort { ($0.time ?? .distantPast) > ($1.time ?? .distantPast) }
        if list.count > maxActivity { list = Array(list.prefix(maxActivity)) }
        activity = list
    }

    // MARK: - Actions

    func copyMCPConfig() {
        Task {
            if let result = await AgentConfig.claudeConfig(paths: paths, status: status) {
                let pb = NSPasteboard.general
                pb.clearContents()
                pb.setString(result.text, forType: .string)
                flash(result.fromCLI ? "MCP config copied" : "MCP config copied (built from status)")
            } else {
                flash("MCP config unavailable — runtime not ready")
            }
        }
    }

    func copyToPasteboard(_ text: String, message: String = "Copied") {
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(text, forType: .string)
        flash(message)
    }

    func flash(_ text: String) {
        transientMessage = text
        Task { [weak self] in
            try? await Task.sleep(nanoseconds: 2_500_000_000)
            if self?.transientMessage == text { self?.transientMessage = nil }
        }
    }

    func revealDataDirectory() {
        let url = paths.dataDir
        if FileManager.default.fileExists(atPath: url.path) {
            NSWorkspace.shared.activateFileViewerSelecting([url])
        } else {
            flash("Data directory does not exist yet")
        }
    }

    func revealLogs() {
        // bfd writes $BF_HOME/logs/{bfd,probod,postgres}.log; bfd-macos.log
        // holds the stdout/stderr this app captured from the child process.
        let bfdLog = paths.logsDir.appendingPathComponent("bfd.log")
        let log = FileManager.default.fileExists(atPath: bfdLog.path) ? bfdLog : supervisor.logFileURL
        if FileManager.default.fileExists(atPath: log.path) {
            NSWorkspace.shared.activateFileViewerSelecting([log])
        } else if FileManager.default.fileExists(atPath: paths.logsDir.path) {
            NSWorkspace.shared.activateFileViewerSelecting([paths.logsDir])
        } else {
            flash("No logs yet")
        }
    }

    func verifyLedger() {
        guard !verifyingLedger else { return }
        verifyingLedger = true
        verificationError = nil
        Task {
            do {
                verification = try await client.verifyLedger()
            } catch {
                verification = nil
                verificationError = "Could not verify ledger: \(error.localizedDescription)"
            }
            verifyingLedger = false
        }
    }

    func installIntoClaudeCode() {
        guard !installingClaude else { return }
        installingClaude = true
        installOutput = nil
        Task {
            let result = await AgentConfig.installClaude(paths: paths)
            installOutput = result
            installingClaude = false
            flash(result.exitCode == 0 ? "Installed into Claude Code" : "bf install-claude failed")
        }
    }

    func restartRuntime() {
        Task { await supervisor.restart() }
    }
}
