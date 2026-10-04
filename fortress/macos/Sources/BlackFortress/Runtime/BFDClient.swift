import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Minimal client for the bfd control API on 127.0.0.1:7811.
struct BFDClient {
    static let defaultPort = 7811
    static let defaultBaseURL = URL(string: "http://127.0.0.1:7811")!
    /// Base used for browser-facing URLs. Must be `localhost` (not 127.0.0.1)
    /// so the session cookie set by /login matches the console at localhost:7810.
    static let browserBaseURL = URL(string: "http://localhost:7811")!

    enum ClientError: Error {
        case badStatus(Int)
        case invalidBody
    }

    let baseURL: URL
    private let session: URLSession

    init(baseURL: URL = BFDClient.defaultBaseURL) {
        self.baseURL = baseURL
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 5
        config.timeoutIntervalForResource = 15
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        config.urlCache = nil
        self.session = URLSession(configuration: config)
    }

    var eventsURL: URL { baseURL.appendingPathComponent("v1/events") }

    /// Every bfd endpoint except /v1/status needs the local token that bfd
    /// writes to secrets.json (readable only by this user).
    static func authToken() -> String? {
        AgentConfig.personalAPIKey(dataDir: RuntimePaths.current.dataDir)
    }

    func getJSON(_ path: String, query: [URLQueryItem] = [], timeout: TimeInterval = 5) async throws -> JSONValue {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)!
        if !query.isEmpty { components.queryItems = query }
        var request = URLRequest(url: components.url!)
        request.timeoutInterval = timeout
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token = BFDClient.authToken() {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        let (data, response) = try await session.data(for: request)
        if let http = response as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
            throw ClientError.badStatus(http.statusCode)
        }
        guard let json = JSONValue.parse(data) else { throw ClientError.invalidBody }
        return json
    }

    func status(timeout: TimeInterval = 3) async throws -> BFDStatus {
        BFDStatus(json: try await getJSON("v1/status", timeout: timeout))
    }

    func posture() async throws -> Posture {
        Posture(json: try await getJSON("v1/posture"))
    }

    func ledger(limit: Int = 200) async throws -> [LedgerEntry] {
        let json = try await getJSON("v1/ledger", query: [URLQueryItem(name: "limit", value: String(limit))])
        return LedgerResponse.entries(from: json)
    }

    /// GET /v1/checks — latest automated check results.
    func checks() async throws -> ChecksSummary {
        ChecksSummary(json: try await getJSON("v1/checks"))
    }

    /// POST /v1/checks/run — run automated checks now. Requires the local
    /// agent token from secrets.json; checks can take several minutes.
    func runChecks(token: String) async throws -> ChecksSummary {
        var request = URLRequest(url: baseURL.appendingPathComponent("v1/checks/run"))
        request.httpMethod = "POST"
        request.timeoutInterval = 30 * 60
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 30 * 60
        config.timeoutIntervalForResource = 30 * 60
        let (data, response) = try await URLSession(configuration: config).data(for: request)
        if let http = response as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
            throw ClientError.badStatus(http.statusCode)
        }
        guard let json = JSONValue.parse(data) else { throw ClientError.invalidBody }
        return ChecksSummary(json: json)
    }

    /// GET /v1/ledger/verify — hash-chain integrity of the evidence ledger.
    func verifyLedger() async throws -> LedgerVerification {
        LedgerVerification(json: try await getJSON("v1/ledger/verify", timeout: 30))
    }

    /// GET /v1/login-link → {"url": "http://localhost:7811/login?nonce=..."}.
    /// The nonce is single-use with a 60 s TTL, so fetch a fresh link for every
    /// console sign-in. The host is forced to `localhost` for cookie scoping.
    func loginLink() async throws -> URL {
        let json = try await getJSON("v1/login-link")
        guard let raw = json.first("url", "login_url")?.string,
              var components = URLComponents(string: raw) else {
            throw ClientError.invalidBody
        }
        if components.host == "127.0.0.1" || components.host == "::1" || components.host == "[::1]" {
            components.host = "localhost"
        }
        if components.host == nil {
            // Relative link: resolve against the browser base.
            guard let resolved = URL(string: raw, relativeTo: BFDClient.browserBaseURL)?.absoluteURL else {
                throw ClientError.invalidBody
            }
            return resolved
        }
        guard let url = components.url else { throw ClientError.invalidBody }
        return url
    }

    /// True when something answers /v1/status with a JSON body.
    func isReachable(timeout: TimeInterval = 1.5) async -> Bool {
        do {
            _ = try await getJSON("v1/status", timeout: timeout)
            return true
        } catch {
            return false
        }
    }
}
