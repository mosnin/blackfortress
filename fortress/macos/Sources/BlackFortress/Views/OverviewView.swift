import SwiftUI

struct OverviewView: View {
    @EnvironmentObject private var model: AppModel

    private let cardColumns = [GridItem(.adaptive(minimum: 260, maximum: 420), spacing: Theme.spacing)]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 28) {
                PageHeader(title: "Overview", subtitle: subtitle) {
                    LiveIndicator(live: model.streamOpen)
                }

                statusBanner

                VStack(alignment: .leading, spacing: 12) {
                    SectionLabel("Guardrails · last 24 hours")
                    guardrailCounters
                }

                VStack(alignment: .leading, spacing: 12) {
                    SectionLabel("Compliance posture")
                    postureGrid
                }

                HStack(alignment: .top, spacing: Theme.spacing) {
                    servicesCard
                        .frame(maxWidth: 360)
                    eventsCard
                }
            }
            .padding(.horizontal, 32)
            .padding(.top, 40)
            .padding(.bottom, 32)
        }
        .background(Theme.background)
    }

    private var subtitle: String {
        if let updated = model.lastUpdated {
            return "Updated \(RelativeTime.clock(updated))"
        }
        return "Waiting for the local runtime"
    }

    // MARK: Banner

    private var statusBanner: some View {
        HStack(spacing: 16) {
            Image(systemName: model.health.symbol)
                .font(.system(size: 30, weight: .semibold))
                .foregroundStyle(model.health.color)
            VStack(alignment: .leading, spacing: 4) {
                Text(model.healthTitle)
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundStyle(Theme.textPrimary)
                Text(bannerDetail)
                    .font(Theme.body)
                    .foregroundStyle(Theme.textSecondary)
            }
            Spacer()
            HStack(spacing: 28) {
                Metric(value: model.posture?.frameworks.count, label: "Frameworks")
                Metric(value: averageScorePercent, label: "Avg. readiness %")
                Metric(
                    value: model.posture?.guardrails.last24h,
                    label: "Agent actions 24h"
                )
            }
        }
        .card(padding: 22)
    }

    private var bannerDetail: String {
        if model.connection != .connected {
            return model.supervisor.mode.label
        }
        if let status = model.status {
            let bad = status.unhealthyServices
            if !bad.isEmpty {
                return "Needs attention: " + bad.map { $0.displayName }.joined(separator: ", ")
            }
            if let message = status.message { return message }
        }
        return "Agents are guarded and evidence is being recorded locally."
    }

    private var averageScorePercent: Int? {
        let scores = (model.posture?.frameworks ?? []).compactMap { $0.score }
        guard !scores.isEmpty else { return nil }
        return Int((scores.reduce(0, +) / Double(scores.count) * 100).rounded())
    }

    // MARK: Guardrail counters

    private var guardrailCounters: some View {
        let g = model.posture?.guardrails
        return HStack(spacing: Theme.spacing) {
            CounterTile(value: g?.last24h, label: "Evaluated", symbol: "bolt.horizontal", tint: Theme.textPrimary)
            CounterTile(value: g?.blocked24h, label: "Blocked", symbol: "hand.raised.fill",
                        tint: (g?.blocked24h ?? 0) > 0 ? Theme.danger : Theme.textPrimary)
            CounterTile(value: g?.asked24h, label: "Asked for approval", symbol: "questionmark.circle",
                        tint: (g?.asked24h ?? 0) > 0 ? Theme.warning : Theme.textPrimary)
            CounterTile(value: g?.recorded24h, label: "Recorded as evidence", symbol: "checkmark.seal",
                        tint: Theme.accent)
        }
    }

    // MARK: Posture

    @ViewBuilder
    private var postureGrid: some View {
        if let frameworks = model.posture?.frameworks, !frameworks.isEmpty {
            LazyVGrid(columns: cardColumns, alignment: .leading, spacing: Theme.spacing) {
                ForEach(frameworks) { fw in
                    PostureCard(framework: fw)
                }
            }
        } else {
            EmptyStateView(
                symbol: "checklist",
                title: "No posture yet",
                message: "Framework posture appears once the runtime has provisioned your organization."
            )
            .card()
        }
    }

    // MARK: Services

    private var servicesCard: some View {
        VStack(alignment: .leading, spacing: 14) {
            SectionLabel("Service health")
            if let services = model.status?.services, !services.isEmpty {
                ForEach(services) { service in
                    HStack(spacing: 10) {
                        StatusDot(service: service)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(service.displayName)
                                .font(Theme.body.weight(.medium))
                                .foregroundStyle(Theme.textPrimary)
                            if let detail = service.detail {
                                Text(detail)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundStyle(Theme.textTertiary)
                                    .lineLimit(1)
                            }
                        }
                        Spacer()
                        Text(service.state)
                            .font(Theme.caption)
                            .foregroundStyle(Theme.textSecondary)
                    }
                }
            } else {
                Text(model.supervisor.mode.label)
                    .font(Theme.body)
                    .foregroundStyle(Theme.textSecondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .card()
    }

    // MARK: Events

    private var eventsCard: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                SectionLabel("Recent agent activity")
                Spacer()
                Button("View all") { model.section = .activity }
                    .buttonStyle(.plain)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.accent)
            }
            let recent = Array(model.activity.prefix(8))
            if recent.isEmpty {
                Text("No agent activity recorded yet. Install the Claude Code hooks from Settings to start collecting evidence.")
                    .font(Theme.body)
                    .foregroundStyle(Theme.textTertiary)
            } else {
                ForEach(recent) { entry in
                    ActivityRow(entry: entry)
                    if entry.id != recent.last?.id {
                        Rectangle().fill(Theme.border).frame(height: 1)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .card()
    }
}

struct PostureCard: View {
    let framework: FrameworkPosture
    @State private var hovering = false

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(framework.name)
                        .font(.system(size: 17, weight: .semibold))
                        .foregroundStyle(Theme.textPrimary)
                        .lineLimit(2)
                    Text("\(framework.controls.total) controls")
                        .font(Theme.caption.monospacedDigit())
                        .foregroundStyle(Theme.textSecondary)
                }
                Spacer()
                ScoreRing(score: framework.score)
                    .frame(width: 64, height: 64)
            }
            ScoreBar(score: framework.score)
            HStack(spacing: 0) {
                Metric(value: framework.controls.byState["implemented"] ?? framework.controls.done, label: "Implemented", size: 20)
                Spacer()
                Metric(value: framework.controls.byState["in_progress"], label: "In progress", size: 20)
                Spacer()
                Metric(value: framework.controls.byState["not_started"], label: "Not started", size: 20)
            }
        }
        .card(padding: 20)
        .overlay(
            RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                .strokeBorder(hovering ? Theme.accent.opacity(0.5) : Color.clear, lineWidth: 1)
        )
        .onHover { hovering = $0 }
    }
}

struct CounterTile: View {
    let value: Int?
    let label: String
    let symbol: String
    var tint: Color = Theme.textPrimary

    var body: some View {
        HStack(alignment: .center, spacing: 14) {
            Image(systemName: symbol)
                .font(.system(size: 16, weight: .semibold))
                .foregroundStyle(tint)
                .frame(width: 22)
            VStack(alignment: .leading, spacing: 2) {
                Text(value.map { String($0) } ?? "—")
                    .font(Theme.metric(24))
                    .foregroundStyle(tint)
                Text(label)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.textSecondary)
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .card(padding: 16)
    }
}

struct Metric: View {
    let value: Int?
    let label: String
    var tint: Color = Theme.textPrimary
    var size: CGFloat = 24

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(value.map { String($0) } ?? "—")
                .font(Theme.metric(size))
                .foregroundStyle(tint)
            Text(label)
                .font(Theme.caption)
                .foregroundStyle(Theme.textSecondary)
        }
    }
}

struct LiveIndicator: View {
    let live: Bool

    var body: some View {
        HStack(spacing: 6) {
            StatusDot(color: live ? Theme.accent : Theme.textTertiary)
            Text(live ? "LIVE" : "OFFLINE")
                .font(.system(size: 10, weight: .bold))
                .tracking(1)
                .foregroundStyle(live ? Theme.accent : Theme.textTertiary)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 5)
        .background(Capsule().fill(Theme.surface))
        .overlay(Capsule().strokeBorder(Theme.border, lineWidth: 1))
    }
}

struct ActivityRow: View {
    let entry: LedgerEntry

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            DecisionBadge(decision: entry.decision)
                .frame(width: 60, alignment: .leading)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(entry.tool ?? "Agent action")
                        .font(Theme.body.weight(.medium))
                        .foregroundStyle(Theme.textPrimary)
                    if let target = entry.target {
                        Text(target)
                            .font(Theme.mono)
                            .foregroundStyle(Theme.textSecondary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                    }
                }
                if !entry.controls.isEmpty {
                    ControlChips(refs: entry.controls)
                }
            }
            Spacer(minLength: 8)
            Text(RelativeTime.clock(entry.time))
                .font(Theme.caption.monospacedDigit())
                .foregroundStyle(Theme.textTertiary)
        }
        .padding(.vertical, 2)
    }
}
