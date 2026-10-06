import Foundation

/// Resolves where the bundled binaries and the data directory live.
///
/// Lookup order for the binaries directory:
///   1. `BF_BIN_DIR` environment variable (dev override)
///   2. `BlackFortress.app/Contents/Resources/bin`
///   3. `../bfd/dist/darwin-<arch>` next to the package (when run via `swift run`
///      from fortress/macos)
struct RuntimePaths {
    let binDir: URL?
    let postgresDir: URL?
    let dataDir: URL
    /// When true the app never spawns bfd; it only attaches to a running one.
    let attachOnly: Bool

    static let current = RuntimePaths()

    init(environment: [String: String] = ProcessInfo.processInfo.environment, bundle: Bundle = .main) {
        let fm = FileManager.default

        func existingDir(_ url: URL?) -> URL? {
            guard let url else { return nil }
            var isDir: ObjCBool = false
            if fm.fileExists(atPath: url.path, isDirectory: &isDir), isDir.boolValue { return url }
            return nil
        }

        let resources = bundle.resourceURL

        var candidates: [URL?] = []
        if let env = environment["BF_BIN_DIR"], !env.isEmpty {
            candidates.append(URL(fileURLWithPath: env, isDirectory: true))
        }
        candidates.append(resources?.appendingPathComponent("bin", isDirectory: true))
        candidates.append(
            URL(fileURLWithPath: fm.currentDirectoryPath, isDirectory: true)
                .appendingPathComponent("../bfd/dist/darwin-\(RuntimePaths.archName)", isDirectory: true)
                .standardizedFileURL
        )
        binDir = candidates.lazy.compactMap { existingDir($0) }.first { dir in
            fm.isExecutableFile(atPath: dir.appendingPathComponent("bfd").path)
        }

        var pgCandidates: [URL?] = []
        if let env = environment["BF_PG_DIR"], !env.isEmpty {
            pgCandidates.append(URL(fileURLWithPath: env, isDirectory: true))
        }
        pgCandidates.append(resources?.appendingPathComponent("postgres", isDirectory: true))
        pgCandidates.append(
            URL(fileURLWithPath: fm.currentDirectoryPath, isDirectory: true)
                .appendingPathComponent("Resources/postgres", isDirectory: true)
        )
        postgresDir = pgCandidates.lazy.compactMap { existingDir($0) }.first

        if let env = environment["BF_HOME"], !env.isEmpty {
            dataDir = URL(fileURLWithPath: (env as NSString).expandingTildeInPath, isDirectory: true)
        } else {
            dataDir = RuntimePaths.defaultDataDir()
        }

        let attachEnv = environment["BF_ATTACH_ONLY"].map { ["1", "true", "yes"].contains($0.lowercased()) } ?? false
        attachOnly = attachEnv || UserDefaults.standard.bool(forKey: "attachOnly")
    }

    static func defaultDataDir() -> URL {
        #if os(macOS)
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent("Library/Application Support")
        return base.appendingPathComponent("BlackFortress", isDirectory: true)
        #else
        let env = ProcessInfo.processInfo.environment
        if let xdg = env["XDG_DATA_HOME"], !xdg.isEmpty {
            return URL(fileURLWithPath: xdg).appendingPathComponent("blackfortress", isDirectory: true)
        }
        return URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent(".local/share/blackfortress", isDirectory: true)
        #endif
    }

    static var archName: String {
        #if arch(arm64)
        return "arm64"
        #else
        return "amd64"
        #endif
    }

    var bfdExecutable: URL? { binDir?.appendingPathComponent("bfd") }
    var bfExecutable: URL? { binDir?.appendingPathComponent("bf") }
    var logsDir: URL { dataDir.appendingPathComponent("logs", isDirectory: true) }

    /// Display path for the `bf` CLI in setup instructions.
    var bfCommandPath: String {
        bfExecutable?.path ?? "bf"
    }

    /// Environment for child processes (bfd, bf).
    func childEnvironment() -> [String: String] {
        var env = ProcessInfo.processInfo.environment
        env["BF_HOME"] = dataDir.path
        if let binDir {
            env["BF_BIN_DIR"] = binDir.path
            let path = env["PATH"] ?? "/usr/bin:/bin:/usr/sbin:/sbin"
            env["PATH"] = binDir.path + ":" + path
        }
        if let postgresDir {
            env["BF_PG_DIR"] = postgresDir.path
        }
        return env
    }
}
