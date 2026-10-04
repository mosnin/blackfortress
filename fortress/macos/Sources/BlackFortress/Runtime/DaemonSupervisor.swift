import Foundation
#if canImport(Combine)
import Combine
#endif
#if canImport(Darwin)
import Darwin
#endif

/// Runs the bundled `bfd run` as a child process, restarts it with backoff
/// if it crashes, and stops it cleanly (SIGTERM, then wait) on quit.
///
/// If something already answers /v1/status on 127.0.0.1:7811 at launch (for
/// example a bfd you started by hand), the supervisor attaches to it instead
/// of spawning, and never stops it.
@MainActor
final class DaemonSupervisor: ObservableObject {
    enum Mode: Equatable {
        case idle
        case probing
        /// An externally started bfd is answering; we do not own it.
        case attached
        case launching
        case running(pid: Int32)
        case restarting(attempt: Int, delay: Double)
        case stopping
        case stopped
        /// No bfd binary found; waiting for one to appear on :7811.
        case binaryMissing
        case failed(String)

        var label: String {
            switch self {
            case .idle: return "Idle"
            case .probing: return "Looking for runtime"
            case .attached: return "Attached to running bfd"
            case .launching: return "Launching bfd"
            case .running(let pid): return "Managed (pid \(pid))"
            case .restarting(let attempt, let delay):
                return "Restarting in \(Int(delay.rounded()))s (attempt \(attempt))"
            case .stopping: return "Stopping"
            case .stopped: return "Stopped"
            case .binaryMissing: return "bfd binary not found — waiting for an external bfd"
            case .failed(let msg): return "Failed: \(msg)"
            }
        }
    }

    @Published private(set) var mode: Mode = .idle
    @Published private(set) var lastExitDescription: String?

    let paths: RuntimePaths
    private let client: BFDClient
    private var process: Process?
    private var logHandle: FileHandle?
    private var startedAt: Date?
    private var attempt = 0
    private var isStopping = false
    private var restartTask: Task<Void, Never>?

    init(paths: RuntimePaths = .current, client: BFDClient = BFDClient()) {
        self.paths = paths
        self.client = client
    }

    /// True when this app spawned bfd and is responsible for stopping it.
    var ownsProcess: Bool { process?.isRunning == true }

    var canRestart: Bool {
        switch mode {
        case .attached, .binaryMissing, .probing: return false
        default: return paths.bfdExecutable != nil && !paths.attachOnly
        }
    }

    // MARK: - Lifecycle

    func start() {
        guard mode == .idle || mode == .stopped || mode == .binaryMissing else { return }
        isStopping = false
        mode = .probing
        Task {
            if await client.isReachable() {
                mode = .attached
                return
            }
            if paths.attachOnly {
                mode = .binaryMissing
                return
            }
            guard paths.bfdExecutable != nil else {
                mode = .binaryMissing
                return
            }
            spawn()
        }
    }

    /// Called by the app's health loop when the API stops answering while we
    /// are attached, so we can take over if the external bfd went away.
    func externalRuntimeLost() {
        guard mode == .attached || mode == .binaryMissing else { return }
        guard !paths.attachOnly, paths.bfdExecutable != nil else {
            mode = .binaryMissing
            return
        }
        mode = .idle
        start()
    }

    /// Called when /v1/status answers while we have no process of our own
    /// (e.g. a bfd started by hand after the app launched).
    func noteExternalRuntime() {
        if mode == .binaryMissing || mode == .idle {
            mode = .attached
        }
    }

    /// Manually restarts a managed bfd (Settings → Restart runtime).
    func restart() async {
        await stop()
        mode = .idle
        attempt = 0
        start()
    }

    /// Sends SIGTERM and waits (up to `timeout`) for exit, then SIGKILL.
    func stop(timeout: TimeInterval = 20) async {
        isStopping = true
        restartTask?.cancel()
        restartTask = nil
        guard let proc = process, proc.isRunning else {
            process = nil
            if mode != .attached { mode = .stopped }
            return
        }
        mode = .stopping
        let pid = proc.processIdentifier
        AppDelegate.log("stopping bfd (pid \(pid))")
        proc.terminate() // SIGTERM
        let deadline = Date().addingTimeInterval(timeout)
        while proc.isRunning && Date() < deadline {
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        if proc.isRunning {
            AppDelegate.log("bfd did not stop in \(Int(timeout))s, killing it")
            kill(pid, SIGKILL)
            try? await Task.sleep(nanoseconds: 200_000_000)
        }
        process = nil
        closeLog()
        mode = .stopped
    }

    // MARK: - Spawning

    private func spawn() {
        guard !isStopping, let exe = paths.bfdExecutable else { return }
        mode = .launching

        let proc = Process()
        proc.executableURL = exe
        proc.arguments = ["run"]
        proc.environment = paths.childEnvironment()

        do {
            try FileManager.default.createDirectory(at: paths.logsDir, withIntermediateDirectories: true)
        } catch {
            // bfd creates its own directories; logging is best-effort.
        }
        if FileManager.default.fileExists(atPath: paths.dataDir.path) {
            proc.currentDirectoryURL = paths.dataDir
        }
        if let handle = openLog() {
            proc.standardOutput = handle
            proc.standardError = handle
        } else {
            proc.standardOutput = FileHandle.nullDevice
            proc.standardError = FileHandle.nullDevice
        }

        proc.terminationHandler = { [weak self] finished in
            let status = finished.terminationStatus
            let reason = finished.terminationReason
            Task { @MainActor in
                self?.handleExit(of: finished, status: status, reason: reason)
            }
        }

        do {
            try proc.run()
            process = proc
            startedAt = Date()
            mode = .running(pid: proc.processIdentifier)
        } catch {
            process = nil
            lastExitDescription = "Could not launch bfd: \(error.localizedDescription)"
            scheduleRestart()
        }
    }

    private func handleExit(of finished: Process, status: Int32, reason: Process.TerminationReason) {
        // Only react to the process we currently own. stop() clears
        // `process` itself, so late callbacks for a deliberately stopped (or
        // replaced) process are ignored and cannot trigger a second spawn.
        guard let current = process, finished === current else { return }
        process = nil
        closeLog()

        let how = reason == .uncaughtSignal ? "signal \(status)" : "exit code \(status)"
        lastExitDescription = "bfd stopped (\(how)) at \(Date().formatted(date: .omitted, time: .standard))"

        if isStopping {
            mode = .stopped
            return
        }
        // Reset backoff if the previous run was healthy for a while.
        if let started = startedAt, Date().timeIntervalSince(started) > 60 {
            attempt = 0
        }
        scheduleRestart()
    }

    private func scheduleRestart() {
        guard !isStopping else { return }
        attempt += 1
        let delay = min(30, pow(2, Double(min(attempt - 1, 5))))
        mode = .restarting(attempt: attempt, delay: delay)
        restartTask?.cancel()
        restartTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
            guard let self, !Task.isCancelled, !self.isStopping else { return }
            // If another bfd took the port meanwhile, attach instead.
            if await self.client.isReachable() {
                self.mode = .attached
                return
            }
            self.spawn()
        }
    }

    // MARK: - Logging

    private func openLog() -> FileHandle? {
        let url = paths.logsDir.appendingPathComponent("bfd-macos.log")
        let fm = FileManager.default
        if !fm.fileExists(atPath: url.path) {
            _ = fm.createFile(atPath: url.path, contents: nil)
        }
        guard let handle = try? FileHandle(forWritingTo: url) else { return nil }
        _ = try? handle.seekToEnd()
        let banner = "\n--- bfd launched by Black Fortress.app at \(Date()) ---\n"
        try? handle.write(contentsOf: Data(banner.utf8))
        logHandle = handle
        return handle
    }

    private func closeLog() {
        try? logHandle?.close()
        logHandle = nil
    }

    var logFileURL: URL { paths.logsDir.appendingPathComponent("bfd-macos.log") }
}
