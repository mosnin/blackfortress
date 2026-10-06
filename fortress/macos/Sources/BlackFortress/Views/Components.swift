import SwiftUI

struct SectionLabel: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text.uppercased())
            .font(.system(size: 10, weight: .semibold))
            .tracking(0.8)
            .foregroundStyle(Theme.textTertiary)
    }
}

struct StatusDot: View {
    var color: Color

    init(color: Color) { self.color = color }

    init(service: ServiceHealth) {
        if service.isHealthy {
            color = Theme.accent
        } else if service.isStarting {
            color = Theme.textPrimary
        } else if service.isOptionalAbsent {
            color = Theme.textTertiary
        } else {
            color = Theme.danger
        }
    }

    var body: some View {
        Circle()
            .fill(color)
            .frame(width: 7, height: 7)
            .shadow(color: color.opacity(0.6), radius: 3)
    }
}

/// Thin horizontal progress bar on a dark track.
struct ScoreBar: View {
    let score: Double?

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(Theme.border)
                Capsule()
                    .fill(Theme.accent)
                    .frame(width: max(0, geo.size.width * CGFloat(score ?? 0)))
            }
        }
        .frame(height: 5)
    }
}

/// Circular score gauge.
struct ScoreRing: View {
    let score: Double?
    var lineWidth: CGFloat = 8

    var body: some View {
        ZStack {
            Circle()
                .stroke(Theme.border, lineWidth: lineWidth)
            Circle()
                .trim(from: 0, to: CGFloat(score ?? 0))
                .stroke(Theme.accent, style: StrokeStyle(lineWidth: lineWidth, lineCap: .round))
                .rotationEffect(.degrees(-90))
            Text(score.map { "\(Int(($0 * 100).rounded()))" } ?? "—")
                .font(Theme.metric(22))
                .foregroundStyle(Theme.textPrimary)
        }
    }
}

struct DecisionBadge: View {
    let decision: Decision

    var body: some View {
        Text(decision.label.uppercased())
            .font(.system(size: 9, weight: .bold))
            .tracking(0.5)
            .foregroundStyle(decision.color)
            .padding(.horizontal, 6)
            .padding(.vertical, 3)
            .background(
                RoundedRectangle(cornerRadius: 4, style: .continuous)
                    .fill(decision.color.opacity(0.12))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 4, style: .continuous)
                    .strokeBorder(decision.color.opacity(0.35), lineWidth: 1)
            )
            .fixedSize()
    }
}

struct ControlChips: View {
    let refs: [String]

    var body: some View {
        HStack(spacing: 4) {
            ForEach(refs, id: \.self) { ref in
                Text(ref)
                    .font(.system(size: 10, weight: .medium, design: .monospaced))
                    .foregroundStyle(Theme.textSecondary)
                    .padding(.horizontal, 5)
                    .padding(.vertical, 2)
                    .background(
                        RoundedRectangle(cornerRadius: 4, style: .continuous)
                            .fill(Theme.surfaceRaised)
                    )
                    .fixedSize()
            }
        }
    }
}

struct EmptyStateView: View {
    let symbol: String
    let title: String
    let message: String

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: symbol)
                .font(.system(size: 28, weight: .light))
                .foregroundStyle(Theme.textTertiary)
            Text(title)
                .font(Theme.headline)
                .foregroundStyle(Theme.textPrimary)
            Text(message)
                .font(Theme.body)
                .foregroundStyle(Theme.textSecondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 360)
        }
        .frame(maxWidth: .infinity)
        .padding(32)
    }
}

/// Page header used at the top of each detail view.
struct PageHeader<Trailing: View>: View {
    let title: String
    let subtitle: String
    @ViewBuilder var trailing: () -> Trailing

    var body: some View {
        HStack(alignment: .bottom) {
            VStack(alignment: .leading, spacing: 6) {
                Text(title)
                    .font(Theme.title())
                    .foregroundStyle(Theme.textPrimary)
                Text(subtitle)
                    .font(Theme.body)
                    .foregroundStyle(Theme.textSecondary)
            }
            Spacer()
            trailing()
        }
    }
}

extension PageHeader where Trailing == EmptyView {
    init(title: String, subtitle: String) {
        self.init(title: title, subtitle: subtitle, trailing: { EmptyView() })
    }
}

/// Small helpers for formatting times.
enum RelativeTime {
    private static let formatter: RelativeDateTimeFormatter = {
        let f = RelativeDateTimeFormatter()
        f.unitsStyle = .abbreviated
        return f
    }()

    static func short(_ date: Date) -> String {
        let seconds = Date().timeIntervalSince(date)
        if seconds < 5 { return "now" }
        return formatter.localizedString(for: date, relativeTo: Date())
    }

    static func clock(_ date: Date?) -> String {
        guard let date else { return "—" }
        return date.formatted(date: .omitted, time: .standard)
    }

    static func full(_ date: Date?) -> String {
        guard let date else { return "—" }
        return date.formatted(date: .abbreviated, time: .standard)
    }
}

/// A row of a key/value list.
struct KeyValueRow: View {
    let key: String
    let value: String
    var mono = false

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(key)
                .font(Theme.body)
                .foregroundStyle(Theme.textSecondary)
                .frame(width: 150, alignment: .leading)
            Text(value)
                .font(mono ? Theme.mono : Theme.body)
                .foregroundStyle(Theme.textPrimary)
                .textSelection(.enabled)
                .lineLimit(2)
                .truncationMode(.middle)
            Spacer(minLength: 0)
        }
    }
}
