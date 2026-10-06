import SwiftUI

/// Frameworks from /v1/posture with per-state breakdowns.
struct FrameworksView: View {
    @EnvironmentObject private var model: AppModel
    @State private var selectedID: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                PageHeader(
                    title: "Frameworks",
                    subtitle: "Readiness per framework, computed from controls and measures in your local Probo organization."
                ) {
                    Button {
                        model.section = .console
                    } label: {
                        Label("Manage in Console", systemImage: "arrow.up.right.square")
                    }
                    .buttonStyle(PrimaryButtonStyle())
                }

                if let frameworks = model.posture?.frameworks, !frameworks.isEmpty {
                    VStack(spacing: 12) {
                        ForEach(frameworks) { fw in
                            FrameworkRow(
                                framework: fw,
                                expanded: selectedID == fw.id
                            )
                            .onTapGesture {
                                withAnimation(.easeOut(duration: 0.15)) {
                                    selectedID = (selectedID == fw.id) ? nil : fw.id
                                }
                            }
                        }
                    }
                } else {
                    EmptyStateView(
                        symbol: "checklist",
                        title: "No frameworks loaded",
                        message: "Once the runtime is up, frameworks imported into your organization (SOC 2, ISO 27001, …) appear here."
                    )
                    .card()
                }
            }
            .padding(.horizontal, 32)
            .padding(.top, 40)
            .padding(.bottom, 32)
        }
        .background(Theme.background)
    }
}

private struct FrameworkRow: View {
    let framework: FrameworkPosture
    let expanded: Bool
    @State private var hovering = false

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(spacing: 18) {
                ScoreRing(score: framework.score, lineWidth: 6)
                    .frame(width: 52, height: 52)
                VStack(alignment: .leading, spacing: 4) {
                    Text(framework.name)
                        .font(Theme.headline)
                        .foregroundStyle(Theme.textPrimary)
                    Text(framework.id)
                        .font(Theme.mono)
                        .foregroundStyle(Theme.textTertiary)
                }
                Spacer()
                stat(framework.controls.done, of: framework.controls.total, label: "Controls")
                stat(framework.measures.done, of: framework.measures.total, label: "Measures")
                VStack(alignment: .trailing, spacing: 2) {
                    Text(framework.score.map { "\(Int(($0 * 100).rounded()))%" } ?? "—")
                        .font(Theme.metric(18))
                        .foregroundStyle(Theme.accent)
                    Text("Readiness")
                        .font(Theme.caption)
                        .foregroundStyle(Theme.textSecondary)
                }
                .frame(width: 90, alignment: .trailing)
                Image(systemName: expanded ? "chevron.up" : "chevron.down")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(Theme.textTertiary)
            }
            ScoreBar(score: framework.score)
            if expanded {
                if let missing = framework.controlsWithoutMeasure, missing > 0 {
                    Text("\(missing) of \(framework.controls.total) controls have no measure yet — add measures in the Console to make progress count.")
                        .font(Theme.caption)
                        .foregroundStyle(Theme.warning)
                }
                HStack(alignment: .top, spacing: 24) {
                    breakdown("Controls by state", framework.controls)
                    breakdown("Measures by state", framework.measures)
                }
            }
        }
        .card(padding: 18)
        .overlay(
            RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                .strokeBorder(hovering ? Theme.accent.opacity(0.5) : Color.clear, lineWidth: 1)
        )
        .contentShape(Rectangle())
        .onHover { hovering = $0 }
    }

    private func stat(_ done: Int, of total: Int, label: String) -> some View {
        VStack(alignment: .trailing, spacing: 2) {
            Text("\(done)/\(total)")
                .font(Theme.metric(18))
                .foregroundStyle(Theme.textPrimary)
            Text(label)
                .font(Theme.caption)
                .foregroundStyle(Theme.textSecondary)
        }
        .frame(width: 90, alignment: .trailing)
    }

    private func breakdown(_ title: String, _ counts: StateCounts) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            SectionLabel(title)
            let keys = counts.byState.keys.sorted { (counts.byState[$0] ?? 0) > (counts.byState[$1] ?? 0) }
            if keys.isEmpty {
                Text("No breakdown provided")
                    .font(Theme.body)
                    .foregroundStyle(Theme.textTertiary)
            } else {
                ForEach(keys, id: \.self) { key in
                    HStack {
                        Text(key.replacingOccurrences(of: "_", with: " ").capitalized)
                            .font(Theme.body)
                            .foregroundStyle(Theme.textSecondary)
                        Spacer()
                        Text("\(counts.byState[key] ?? 0)")
                            .font(Theme.body.monospacedDigit())
                            .foregroundStyle(
                                StateCounts.doneStates.contains(key.lowercased()) ? Theme.accent : Theme.textPrimary
                            )
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .card(padding: 14, raised: true)
    }
}
