import SwiftUI

/// Main window: a fixed black sidebar plus the selected detail page.
///
/// A hand-rolled sidebar (rather than NavigationSplitView) keeps the whole
/// window pure black without fighting the system sidebar material.
struct MainWindow: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        HStack(spacing: 0) {
            Sidebar()
                .frame(width: 228)
            Rectangle()
                .fill(Theme.border)
                .frame(width: 1)
            detail
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(Theme.background)
        }
        .background(Theme.background)
        .ignoresSafeArea()
        .overlay(alignment: .bottom) {
            if let message = model.transientMessage {
                Text(message)
                    .font(Theme.body.weight(.medium))
                    .foregroundStyle(Color.black)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(Capsule().fill(Theme.accent))
                    .padding(.bottom, 24)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeOut(duration: 0.2), value: model.transientMessage)
    }

    @ViewBuilder
    private var detail: some View {
        switch model.section {
        case .overview: OverviewView()
        case .activity: AgentActivityView()
        case .frameworks: FrameworksView()
        case .console: ConsoleView()
        case .settings: SettingsView()
        }
    }
}

private struct Sidebar: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Room for the traffic-light buttons (hidden title bar).
            Spacer().frame(height: 44)

            HStack(spacing: 10) {
                Image(systemName: "shield.lefthalf.filled")
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundStyle(Theme.accent)
                Text("Black Fortress")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(Theme.textPrimary)
            }
            .padding(.horizontal, 18)
            .padding(.bottom, 24)

            VStack(spacing: 2) {
                ForEach(SidebarItem.allCases) { item in
                    SidebarRow(item: item, selected: model.section == item) {
                        model.section = item
                    }
                }
            }
            .padding(.horizontal, 10)

            Spacer()

            RuntimeFooter()
                .padding(14)
        }
        .frame(maxHeight: .infinity)
        .background(Theme.background)
    }
}

private struct SidebarRow: View {
    let item: SidebarItem
    let selected: Bool
    let action: () -> Void
    @State private var hovering = false

    var body: some View {
        Button(action: action) {
            HStack(spacing: 10) {
                Image(systemName: item.symbol)
                    .font(.system(size: 13, weight: .medium))
                    .frame(width: 18)
                    .foregroundStyle(selected ? Theme.accent : Theme.textSecondary)
                Text(item.title)
                    .font(.system(size: 13, weight: selected ? .semibold : .regular))
                    .foregroundStyle(selected ? Theme.textPrimary : Theme.textSecondary)
                Spacer()
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(
                RoundedRectangle(cornerRadius: Theme.radiusSmall, style: .continuous)
                    .fill(selected ? Theme.surfaceRaised : (hovering ? Theme.surface : Color.clear))
            )
            .overlay(
                RoundedRectangle(cornerRadius: Theme.radiusSmall, style: .continuous)
                    .strokeBorder(selected ? Theme.border : Color.clear, lineWidth: 1)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hovering = $0 }
    }
}

private struct RuntimeFooter: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        HStack(spacing: 10) {
            StatusDot(color: model.health.color)
            VStack(alignment: .leading, spacing: 2) {
                Text(model.healthTitle)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.textPrimary)
                    .lineLimit(1)
                Text(model.streamOpen ? "Live" : model.supervisor.mode.label)
                    .font(.system(size: 10))
                    .foregroundStyle(Theme.textTertiary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .card(padding: 12)
    }
}
