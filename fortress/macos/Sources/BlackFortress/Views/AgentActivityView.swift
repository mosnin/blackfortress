import SwiftUI

/// Live table of ledger entries and guardrail events.
///
/// Built from stacks rather than `Table` so header, rows and selection all
/// follow the black theme exactly.
struct AgentActivityView: View {
    @EnvironmentObject private var model: AppModel
    @State private var query = ""
    @State private var decisionFilter: DecisionFilter = .all
    @State private var expandedID: String?

    enum DecisionFilter: String, CaseIterable, Identifiable {
        case all = "All"
        case hits = "Hits"
        case block = "Block"
        case ask = "Ask"
        case record = "Record"

        var id: String { rawValue }

        func matches(_ entry: LedgerEntry) -> Bool {
            switch self {
            case .all: return true
            case .hits: return entry.isGuardrailHit
            case .block: return entry.decision == .block
            case .ask: return entry.decision == .ask || entry.decision == .flag
            case .record: return entry.decision == .record || entry.decision == .allow
            }
        }
    }

    private var filtered: [LedgerEntry] {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        return model.activity.filter { entry in
            guard decisionFilter.matches(entry) else { return false }
            guard !q.isEmpty else { return true }
            var fields: [String] = [entry.decision.label]
            let optionals: [String?] = [entry.tool, entry.target, entry.sessionID, entry.agent, entry.reason, entry.event, entry.outcome, entry.repo]
            for value in optionals {
                if let value { fields.append(value) }
            }
            fields.append(contentsOf: entry.controls)
            fields.append(contentsOf: entry.rules)
            let haystack = fields.joined(separator: " ").lowercased()
            return haystack.contains(q)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            PageHeader(
                title: "Agent Activity",
                subtitle: "Every evaluated agent action, with its decision and the controls it evidences."
            ) {
                LiveIndicator(live: model.streamOpen)
            }

            toolbar
            verificationBanner

            VStack(spacing: 0) {
                ActivityHeaderRow()
                Rectangle().fill(Theme.border).frame(height: 1)
                let rows = filtered
                if rows.isEmpty {
                    EmptyStateView(
                        symbol: "bolt.horizontal.circle",
                        title: model.activity.isEmpty ? "No agent activity yet" : "No matching events",
                        message: model.activity.isEmpty
                            ? "Run `bf install-claude` (see Settings) and start a Claude Code session. Actions appear here in real time."
                            : "Try a different filter or search term."
                    )
                    .frame(maxHeight: .infinity)
                } else {
                    ScrollView {
                        LazyVStack(spacing: 0) {
                            ForEach(rows) { entry in
                                ActivityTableRow(entry: entry, expanded: expandedID == entry.id)
                                    .onTapGesture {
                                        expandedID = (expandedID == entry.id) ? nil : entry.id
                                    }
                                Rectangle().fill(Theme.border.opacity(0.6)).frame(height: 1)
                            }
                        }
                    }
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(
                RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).fill(Theme.surface)
            )
            .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                    .strokeBorder(Theme.border, lineWidth: 1)
            )
        }
        .padding(.horizontal, 32)
        .padding(.top, 40)
        .padding(.bottom, 24)
        .background(Theme.background)
    }

    private var toolbar: some View {
        HStack(spacing: 12) {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass")
                    .foregroundStyle(Theme.textTertiary)
                TextField("Filter by tool, target, session, control…", text: $query)
                    .textFieldStyle(.plain)
                    .font(Theme.body)
                    .foregroundStyle(Theme.textPrimary)
                if !query.isEmpty {
                    Button {
                        query = ""
                    } label: {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundStyle(Theme.textTertiary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(RoundedRectangle(cornerRadius: Theme.radiusSmall).fill(Theme.surface))
            .overlay(RoundedRectangle(cornerRadius: Theme.radiusSmall).strokeBorder(Theme.border, lineWidth: 1))
            .frame(maxWidth: 320)

            Picker("Decision", selection: $decisionFilter) {
                ForEach(DecisionFilter.allCases) { f in
                    Text(f.rawValue).tag(f)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .frame(width: 280)

            Spacer()

            Text("\(filtered.count) of \(model.activity.count)")
                .font(Theme.caption.monospacedDigit())
                .foregroundStyle(Theme.textSecondary)

            Button {
                model.verifyLedger()
            } label: {
                Label(model.verifyingLedger ? "Verifying…" : "Verify ledger integrity", systemImage: "lock.shield")
            }
            .buttonStyle(SecondaryButtonStyle())
            .disabled(model.verifyingLedger)
            .help("Recompute the ledger hash chain (GET /v1/ledger/verify)")

            Button {
                Task { await model.refreshLedger() }
            } label: {
                Image(systemName: "arrow.clockwise")
            }
            .buttonStyle(SecondaryButtonStyle())
            .help("Reload ledger")
        }
    }

    @ViewBuilder
    private var verificationBanner: some View {
        if let v = model.verification {
            HStack(spacing: 10) {
                Image(systemName: v.valid ? "checkmark.shield.fill" : "xmark.shield.fill")
                    .foregroundStyle(v.valid ? Theme.accent : Theme.danger)
                Text(v.valid
                     ? "Ledger intact — \(v.entries) entries, hash chain verified."
                     : "Ledger integrity check FAILED after \(v.entries) entries: \(v.error ?? "unknown error")")
                    .font(Theme.body)
                    .foregroundStyle(Theme.textPrimary)
                    .textSelection(.enabled)
                Spacer(minLength: 0)
            }
            .card(padding: 12, raised: true)
        } else if let err = model.verificationError {
            Text(err)
                .font(Theme.body)
                .foregroundStyle(Theme.warning)
                .card(padding: 12, raised: true)
        }
    }
}

private enum ActivityColumns {
    static let time: CGFloat = 86
    static let session: CGFloat = 120
    static let tool: CGFloat = 110
    static let decision: CGFloat = 76
    static let controls: CGFloat = 220
}

private struct ActivityHeaderRow: View {
    var body: some View {
        HStack(spacing: 12) {
            header("Time").frame(width: ActivityColumns.time, alignment: .leading)
            header("Session").frame(width: ActivityColumns.session, alignment: .leading)
            header("Tool").frame(width: ActivityColumns.tool, alignment: .leading)
            header("Target").frame(maxWidth: .infinity, alignment: .leading)
            header("Decision").frame(width: ActivityColumns.decision, alignment: .leading)
            header("Controls").frame(width: ActivityColumns.controls, alignment: .leading)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Theme.surfaceRaised)
    }

    private func header(_ text: String) -> some View {
        Text(text.uppercased())
            .font(.system(size: 10, weight: .semibold))
            .tracking(0.6)
            .foregroundStyle(Theme.textTertiary)
    }
}

private struct ActivityTableRow: View {
    let entry: LedgerEntry
    let expanded: Bool
    @State private var hovering = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 12) {
                Text(RelativeTime.clock(entry.time))
                    .font(Theme.mono.monospacedDigit())
                    .foregroundStyle(Theme.textSecondary)
                    .frame(width: ActivityColumns.time, alignment: .leading)
                Text(shortSession)
                    .font(Theme.mono)
                    .foregroundStyle(Theme.textSecondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .frame(width: ActivityColumns.session, alignment: .leading)
                Text(entry.tool ?? "—")
                    .font(Theme.body.weight(.medium))
                    .foregroundStyle(Theme.textPrimary)
                    .lineLimit(1)
                    .frame(width: ActivityColumns.tool, alignment: .leading)
                Text(entry.target ?? "—")
                    .font(Theme.mono)
                    .foregroundStyle(Theme.textPrimary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .frame(maxWidth: .infinity, alignment: .leading)
                DecisionBadge(decision: entry.decision)
                    .frame(width: ActivityColumns.decision, alignment: .leading)
                ControlChips(refs: Array(entry.controls.prefix(2)))
                    .frame(width: ActivityColumns.controls, alignment: .leading)
                    .clipped()
            }
            if expanded {
                VStack(alignment: .leading, spacing: 6) {
                    KeyValueRow(key: "Recorded", value: RelativeTime.full(entry.time))
                    KeyValueRow(key: "Kind", value: entry.kind.rawValue.capitalized)
                    if let session = entry.sessionID { KeyValueRow(key: "Session", value: session, mono: true) }
                    if let agent = entry.agent { KeyValueRow(key: "Agent", value: agent) }
                    if let event = entry.event { KeyValueRow(key: "Hook event", value: event) }
                    if let outcome = entry.outcomeLabel { KeyValueRow(key: "Outcome", value: outcome) }
                    if !entry.rules.isEmpty { KeyValueRow(key: "Rules", value: entry.rules.joined(separator: ", "), mono: true) }
                    if let repo = entry.repo { KeyValueRow(key: "Repository", value: repo, mono: true) }
                    if let cwd = entry.cwd { KeyValueRow(key: "Working dir", value: cwd, mono: true) }
                    if let hash = entry.hash { KeyValueRow(key: "Hash", value: hash, mono: true) }
                    if let target = entry.target { KeyValueRow(key: "Target", value: target, mono: true) }
                    if let reason = entry.reason { KeyValueRow(key: "Reason", value: reason) }
                    if !entry.controls.isEmpty {
                        KeyValueRow(key: "Controls", value: entry.controls.joined(separator: ", "), mono: true)
                    }
                }
                .padding(12)
                .background(RoundedRectangle(cornerRadius: Theme.radiusSmall).fill(Theme.background))
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(hovering || expanded ? Theme.surfaceRaised : Color.clear)
        .contentShape(Rectangle())
        .onHover { hovering = $0 }
    }

    private var shortSession: String {
        guard let s = entry.sessionID, !s.isEmpty else { return "—" }
        return s.count > 12 ? String(s.prefix(8)) + "…" : s
    }
}
