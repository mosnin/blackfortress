import SwiftUI

/// Automated evidence checks (Comp's integration checks run locally by
/// bf-checks with the developer's own credentials).
struct ChecksView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                PageHeader(
                    title: "Automated Checks",
                    subtitle: "GitHub, AWS, GCP, Azure and more, verified with the credentials already on this Mac. Results become evidence on the mapped controls."
                ) {
                    Button {
                        model.runChecksNow()
                    } label: {
                        Label(model.runningChecks ? "Running…" : "Run now", systemImage: "play.fill")
                    }
                    .buttonStyle(PrimaryButtonStyle())
                    .disabled(model.runningChecks)
                }

                if let checks = model.checks, !checks.providers.isEmpty {
                    VStack(spacing: 12) {
                        ForEach(checks.providers) { provider in
                            ProviderCard(provider: provider)
                        }
                    }
                } else {
                    EmptyStateView(
                        symbol: "checkmark.shield",
                        title: "No checks have run yet",
                        message: "Checks run shortly after startup and every 6 hours. Log in with gh, aws, gcloud or az, then press Run now."
                    )
                    .card()
                }

                if let skipped = model.checks?.skipped, !skipped.isEmpty {
                    VStack(alignment: .leading, spacing: 10) {
                        SectionLabel("Not running")
                        ForEach(skipped) { item in
                            HStack {
                                Text(item.id)
                                    .font(Theme.mono)
                                    .foregroundStyle(Theme.textSecondary)
                                Spacer()
                                Text(item.reason)
                                    .font(Theme.caption)
                                    .foregroundStyle(Theme.textTertiary)
                            }
                        }
                        Text("Enable a provider by signing in to its CLI, or set variables and disable providers in checks.json in the data directory.")
                            .font(Theme.caption)
                            .foregroundStyle(Theme.textTertiary)
                    }
                    .card(padding: 16)
                }
            }
            .padding(.horizontal, 32)
            .padding(.top, 40)
            .padding(.bottom, 32)
        }
        .background(Theme.background)
        .task { await model.refreshChecks() }
    }
}

private struct ProviderCard: View {
    let provider: ChecksSummary.Provider

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 14) {
                Image(systemName: provider.error == nil ? "checkmark.shield.fill" : "exclamationmark.shield.fill")
                    .font(.system(size: 22))
                    .foregroundStyle(tint)
                VStack(alignment: .leading, spacing: 3) {
                    Text(provider.name)
                        .font(Theme.headline)
                        .foregroundStyle(Theme.textPrimary)
                    Text(subtitle)
                        .font(Theme.caption)
                        .foregroundStyle(Theme.textTertiary)
                }
                Spacer()
                count(provider.passing, "Passing", Theme.accent)
                count(provider.failing, "Failing", provider.failing > 0 ? Theme.danger : Theme.textPrimary)
                count(provider.inconclusive + provider.errored, "No result", Theme.textSecondary)
                count(provider.findings, "Findings", provider.findings > 0 ? Theme.warning : Theme.textPrimary)
            }
            if let error = provider.error {
                Text(error)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.danger)
            } else if provider.total > 0 {
                ScoreBar(score: Double(provider.passing) / Double(provider.total))
            }
        }
        .card(padding: 18)
    }

    private var tint: Color {
        if provider.error != nil || provider.failing > 0 { return Theme.danger }
        return provider.passing > 0 ? Theme.accent : Theme.textSecondary
    }

    private var subtitle: String {
        var parts: [String] = []
        if let ran = provider.ranAt {
            parts.append("Ran " + ran.formatted(.relative(presentation: .named)))
        }
        if let source = provider.source, !source.isEmpty {
            parts.append("via " + source)
        }
        return parts.joined(separator: " · ")
    }

    private func count(_ value: Int, _ label: String, _ color: Color) -> some View {
        VStack(alignment: .trailing, spacing: 2) {
            Text("\(value)")
                .font(Theme.metric(18))
                .foregroundStyle(color)
            Text(label)
                .font(Theme.caption)
                .foregroundStyle(Theme.textSecondary)
        }
        .frame(width: 74, alignment: .trailing)
    }
}
