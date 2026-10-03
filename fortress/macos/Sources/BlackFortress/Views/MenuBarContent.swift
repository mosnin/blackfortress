import SwiftUI
import AppKit

/// Menu bar icon: an SF Symbol shield, tinted by health.
///
/// MenuBarExtra renders SwiftUI images as templates (monochrome), which would
/// drop the tint, so we build a non-template NSImage with a palette color.
struct MenuBarIcon: View {
    let level: HealthLevel

    var body: some View {
        Image(nsImage: Self.image(for: level))
            .accessibilityLabel("Black Fortress")
    }

    static func image(for level: HealthLevel) -> NSImage {
        let base = NSImage(systemSymbolName: level.symbol, accessibilityDescription: "Black Fortress")
            ?? NSImage(systemSymbolName: "shield.lefthalf.filled", accessibilityDescription: "Black Fortress")
            ?? NSImage()
        let config = NSImage.SymbolConfiguration(pointSize: 15, weight: .semibold)
            .applying(NSImage.SymbolConfiguration(paletteColors: [level.nsColor]))
        let image = base.withSymbolConfiguration(config) ?? base
        image.isTemplate = false
        return image
    }
}

/// Content of the menu bar popover (MenuBarExtra with `.window` style).
struct MenuBarContent: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header
            MenuDivider()
            servicesSection
            if let frameworks = model.posture?.frameworks, !frameworks.isEmpty {
                MenuDivider()
                postureSection(frameworks)
            }
            MenuDivider()
            eventsSection
            MenuDivider()
            actions
            if let message = model.transientMessage {
                Text(message)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.accent)
                    .transition(.opacity)
            }
        }
        .padding(16)
        .frame(width: 340)
        .background(Theme.background)
    }

    // MARK: Sections

    private var header: some View {
        HStack(spacing: 12) {
            Image(systemName: model.health.symbol)
                .font(.system(size: 22, weight: .semibold))
                .foregroundStyle(model.health.color)
            VStack(alignment: .leading, spacing: 2) {
                Text("Black Fortress")
                    .font(Theme.headline)
                    .foregroundStyle(Theme.textPrimary)
                Text(model.healthTitle)
                    .font(Theme.caption)
                    .foregroundStyle(model.health.color)
            }
            Spacer()
            if let version = model.status?.version {
                Text("v\(version)")
                    .font(Theme.caption.monospacedDigit())
                    .foregroundStyle(Theme.textTertiary)
            }
        }
    }

    private var servicesSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            SectionLabel("Runtime")
            if let services = model.status?.services, !services.isEmpty {
                ForEach(services) { service in
                    HStack(spacing: 8) {
                        StatusDot(service: service)
                        Text(service.displayName)
                            .font(Theme.body)
                            .foregroundStyle(Theme.textPrimary)
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
    }

    private func postureSection(_ frameworks: [FrameworkPosture]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            SectionLabel("Posture")
            ForEach(frameworks.prefix(6)) { fw in
                HStack(spacing: 10) {
                    Text(fw.name)
                        .font(Theme.body)
                        .foregroundStyle(Theme.textPrimary)
                        .lineLimit(1)
                    Spacer()
                    ScoreBar(score: fw.score)
                        .frame(width: 80)
                    Text(fw.score.map { "\(Int(($0 * 100).rounded()))%" } ?? "—")
                        .font(Theme.caption.monospacedDigit())
                        .foregroundStyle(Theme.textSecondary)
                        .frame(width: 36, alignment: .trailing)
                }
            }
        }
    }

    private var eventsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            SectionLabel("Recent guardrail events")
            let hits = Array(model.guardrailHits.prefix(5))
            if hits.isEmpty {
                Text("No guardrail hits yet")
                    .font(Theme.body)
                    .foregroundStyle(Theme.textTertiary)
            } else {
                ForEach(hits) { entry in
                    HStack(alignment: .top, spacing: 8) {
                        DecisionBadge(decision: entry.decision)
                        VStack(alignment: .leading, spacing: 2) {
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
                        Spacer(minLength: 0)
                        Text(entry.time.map { RelativeTime.short($0) } ?? "")
                            .font(Theme.caption.monospacedDigit())
                            .foregroundStyle(Theme.textTertiary)
                    }
                }
            }
        }
    }

    private var actions: some View {
        VStack(spacing: 6) {
            Button {
                showMain(.overview)
            } label: {
                Label("Open Dashboard", systemImage: "rectangle.grid.2x2")
            }
            .buttonStyle(SecondaryButtonStyle(fullWidth: true))

            Button {
                showMain(.console)
            } label: {
                Label("Open Console", systemImage: "globe")
            }
            .buttonStyle(SecondaryButtonStyle(fullWidth: true))

            Button {
                model.copyMCPConfig()
            } label: {
                Label("Copy MCP config for Claude Code", systemImage: "doc.on.doc")
            }
            .buttonStyle(SecondaryButtonStyle(fullWidth: true))

            Button {
                NSApp.terminate(nil)
            } label: {
                Label("Quit Black Fortress", systemImage: "power")
            }
            .buttonStyle(SecondaryButtonStyle(fullWidth: true))
            .keyboardShortcut("q")
        }
    }

    private func showMain(_ item: SidebarItem) {
        model.section = item
        openWindow(id: WindowID.main)
        NSApp.activate(ignoringOtherApps: true)
    }
}

private struct MenuDivider: View {
    var body: some View {
        Rectangle()
            .fill(Theme.border)
            .frame(height: 1)
    }
}
