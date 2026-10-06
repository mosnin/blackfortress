import Foundation

/// GET /v1/checks → automated check results per provider.
/// {"updated_at","providers":[{"provider","name","ran_at","source","error",
///  "checks_passing","checks_failing","checks_inconclusive","checks_errored",
///  "findings"}],"skipped":{"aws":"no local credentials"}}
struct ChecksSummary: Equatable {
    struct Provider: Equatable, Identifiable {
        var id: String
        var name: String
        var ranAt: Date?
        var source: String?
        var error: String?
        var passing: Int
        var failing: Int
        var inconclusive: Int
        var errored: Int
        var findings: Int

        var total: Int { passing + failing + inconclusive + errored }

        init(json: JSONValue) {
            id = json.first("provider", "id")?.string ?? UUID().uuidString
            name = json.first("name", "provider")?.string ?? id
            ranAt = json.first("ran_at", "ranAt")?.date
            source = json["source"]?.string
            error = json["error"]?.string.flatMap { $0.isEmpty ? nil : $0 }
            passing = json["checks_passing"]?.int ?? 0
            failing = json["checks_failing"]?.int ?? 0
            inconclusive = json["checks_inconclusive"]?.int ?? 0
            errored = json["checks_errored"]?.int ?? 0
            findings = json["findings"]?.int ?? 0
        }
    }

    struct Skipped: Equatable, Identifiable {
        var id: String
        var reason: String
    }

    var updatedAt: Date?
    var providers: [Provider]
    var skipped: [Skipped]

    init(json: JSONValue) {
        updatedAt = json.first("updated_at", "updatedAt")?.date
        providers = (json["providers"]?.array ?? []).map(Provider.init(json:))
        skipped = (json["skipped"]?.object ?? [:])
            .map { Skipped(id: $0.key, reason: $0.value.string ?? "") }
            .sorted { $0.id < $1.id }
    }

    var totalFindings: Int { providers.reduce(0) { $0 + $1.findings } }
}
