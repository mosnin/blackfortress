import Foundation

/// Produces the MCP configuration snippet for Claude Code.
///
/// Preferred source is `bf agent-config claude` (the CLI knows the API key
/// and hook wiring). If the CLI is unavailable we build a minimal MCP entry
/// from /v1/status and, if readable, the personal API key in secrets.json.
enum AgentConfig {
    struct Result {
        var text: String
        var fromCLI: Bool
    }

    static func claudeConfig(paths: RuntimePaths, status: BFDStatus?) async -> Result? {
        if let exe = paths.bfExecutable, FileManager.default.isExecutableFile(atPath: exe.path) {
            if let out = await run(exe, arguments: ["agent-config", "claude"], environment: paths.childEnvironment()),
               !out.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                return Result(text: out, fromCLI: true)
            }
        }
        return fallback(paths: paths, status: status).map { Result(text: $0, fromCLI: false) }
    }

    static func fallback(paths: RuntimePaths, status: BFDStatus?) -> String? {
        guard let mcp = status?.mcpURL else { return nil }
        var server: [String: Any] = ["type": "http", "url": mcp.absoluteString]
        if let key = personalAPIKey(dataDir: paths.dataDir) {
            server["headers"] = ["Authorization": "Bearer \(key)"]
        }
        let config: [String: Any] = ["mcpServers": ["black-fortress": server]]
        guard let data = try? JSONSerialization.data(withJSONObject: config, options: [.prettyPrinted, .sortedKeys]) else {
            return nil
        }
        return String(decoding: data, as: UTF8.self)
    }

    static func personalAPIKey(dataDir: URL) -> String? {
        let url = dataDir.appendingPathComponent("secrets.json")
        guard let data = try? Data(contentsOf: url), let json = JSONValue.parse(data) else { return nil }
        // bfd stores the agent bearer token for /mcp as "mcp_token".
        return json.first("mcp_token", "mcpToken", "api_key", "personal_api_key")?.string
    }

    struct CommandOutput {
        var exitCode: Int32
        var output: String
    }

    /// Runs `bf install-claude`, returning combined stdout+stderr and exit code.
    static func installClaude(paths: RuntimePaths) async -> CommandOutput {
        guard let exe = paths.bfExecutable, FileManager.default.isExecutableFile(atPath: exe.path) else {
            return CommandOutput(exitCode: -1, output: "bf CLI not found. Build the app with scripts/build-app.sh or set BF_BIN_DIR.")
        }
        return await runCapturing(exe, arguments: ["install-claude"], environment: paths.childEnvironment())
    }

    /// Runs a command and returns its combined output and exit status.
    static func runCapturing(_ executable: URL, arguments: [String], environment: [String: String], timeout: TimeInterval = 60) async -> CommandOutput {
        await withCheckedContinuation { (continuation: CheckedContinuation<CommandOutput, Never>) in
            DispatchQueue.global(qos: .userInitiated).async {
                let proc = Process()
                proc.executableURL = executable
                proc.arguments = arguments
                proc.environment = environment
                let pipe = Pipe()
                proc.standardOutput = pipe
                proc.standardError = pipe
                proc.standardInput = FileHandle.nullDevice
                do {
                    try proc.run()
                } catch {
                    continuation.resume(returning: CommandOutput(exitCode: -1, output: "Could not run \(executable.path): \(error.localizedDescription)"))
                    return
                }
                let pid = proc.processIdentifier
                DispatchQueue.global().asyncAfter(deadline: .now() + timeout) {
                    if proc.isRunning { kill(pid, SIGTERM) }
                }
                let data = pipe.fileHandleForReading.readDataToEndOfFile()
                proc.waitUntilExit()
                continuation.resume(returning: CommandOutput(
                    exitCode: proc.terminationStatus,
                    output: String(decoding: data, as: UTF8.self)
                ))
            }
        }
    }

    /// Runs a command and returns stdout, or nil on failure / non-zero exit.
    static func run(_ executable: URL, arguments: [String], environment: [String: String], timeout: TimeInterval = 15) async -> String? {
        await withCheckedContinuation { (continuation: CheckedContinuation<String?, Never>) in
            DispatchQueue.global(qos: .userInitiated).async {
                let proc = Process()
                proc.executableURL = executable
                proc.arguments = arguments
                proc.environment = environment
                let out = Pipe()
                proc.standardOutput = out
                proc.standardError = FileHandle.nullDevice
                proc.standardInput = FileHandle.nullDevice
                do {
                    try proc.run()
                } catch {
                    continuation.resume(returning: nil)
                    return
                }
                // Watchdog so a hung CLI cannot block forever.
                let pid = proc.processIdentifier
                DispatchQueue.global().asyncAfter(deadline: .now() + timeout) {
                    if proc.isRunning { kill(pid, SIGTERM) }
                }
                let data = out.fileHandleForReading.readDataToEndOfFile()
                proc.waitUntilExit()
                guard proc.terminationStatus == 0 else {
                    continuation.resume(returning: nil)
                    return
                }
                continuation.resume(returning: String(decoding: data, as: UTF8.self))
            }
        }
    }
}
