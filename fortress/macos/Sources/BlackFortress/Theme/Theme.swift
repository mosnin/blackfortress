import SwiftUI
import AppKit

/// Black Fortress design tokens. All-black UI with a lime accent.
enum Theme {
    // Backgrounds
    static let background = Color(hex: 0x000000)
    static let surface = Color(hex: 0x0A0A0A)
    static let surfaceRaised = Color(hex: 0x141414)
    static let border = Color(hex: 0x262626)

    // Accent
    static let accent = Color(hex: 0xA3E635)
    static let accentHover = Color(hex: 0xBEF264)
    static let accentPressed = Color(hex: 0x84CC16)

    // Text
    static let textPrimary = Color.white
    static let textSecondary = Color(hex: 0xA3A3A3)
    static let textTertiary = Color(hex: 0x6B6B6B)

    // Status
    static let warning = Color(hex: 0xF59E0B)
    static let danger = Color(hex: 0xEF4444)
    static let info = Color(hex: 0x60A5FA)

    // Shape & spacing
    static let radius: CGFloat = 12
    static let radiusSmall: CGFloat = 8
    static let gutter: CGFloat = 24
    static let spacing: CGFloat = 16

    // AppKit equivalents
    static let nsBackground = NSColor(hex: 0x000000)
    static let nsAccent = NSColor(hex: 0xA3E635)
    static let nsWarning = NSColor(hex: 0xF59E0B)
    static let nsDanger = NSColor(hex: 0xEF4444)

    // Type
    static func title(_ size: CGFloat = 28) -> Font { .system(size: size, weight: .semibold) }
    static let headline = Font.system(size: 15, weight: .semibold)
    static let body = Font.system(size: 13)
    static let caption = Font.system(size: 11, weight: .medium)
    static let mono = Font.system(size: 12, design: .monospaced)
    static func metric(_ size: CGFloat = 34) -> Font {
        .system(size: size, weight: .semibold, design: .rounded).monospacedDigit()
    }
}

extension Color {
    init(hex: UInt32, opacity: Double = 1) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255,
            opacity: opacity
        )
    }
}

extension NSColor {
    convenience init(hex: UInt32, alpha: CGFloat = 1) {
        self.init(
            srgbRed: CGFloat((hex >> 16) & 0xFF) / 255,
            green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255,
            alpha: alpha
        )
    }
}

// MARK: - Health level → color

/// Aggregate state used for the menu bar tint and status pills.
enum HealthLevel: Equatable {
    case good
    case starting
    case warning
    case error

    var color: Color {
        switch self {
        case .good: return Theme.accent
        case .starting: return Theme.textPrimary
        case .warning: return Theme.warning
        case .error: return Theme.danger
        }
    }

    var nsColor: NSColor {
        switch self {
        case .good: return Theme.nsAccent
        case .starting: return .white
        case .warning: return Theme.nsWarning
        case .error: return Theme.nsDanger
        }
    }

    var symbol: String {
        switch self {
        case .good: return "checkmark.shield.fill"
        case .starting: return "shield.lefthalf.filled"
        case .warning: return "exclamationmark.shield.fill"
        case .error: return "xmark.shield.fill"
        }
    }
}

extension Decision {
    var color: Color {
        switch self {
        case .block: return Theme.danger
        case .ask, .flag: return Theme.warning
        case .allow: return Theme.accent
        case .record: return Theme.textSecondary
        case .other: return Theme.textSecondary
        }
    }
}

// MARK: - Reusable styling

struct CardModifier: ViewModifier {
    var padding: CGFloat = 20
    var raised = false

    func body(content: Content) -> some View {
        content
            .padding(padding)
            .background(
                RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                    .fill(raised ? Theme.surfaceRaised : Theme.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                    .strokeBorder(Theme.border, lineWidth: 1)
            )
    }
}

extension View {
    func card(padding: CGFloat = 20, raised: Bool = false) -> some View {
        modifier(CardModifier(padding: padding, raised: raised))
    }
}

/// Lime filled button. Hover → #BEF264, pressed → #84CC16.
struct PrimaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        PrimaryButtonBody(configuration: configuration)
    }

    private struct PrimaryButtonBody: View {
        let configuration: ButtonStyleConfiguration
        @State private var hovering = false
        @Environment(\.isEnabled) private var isEnabled

        var body: some View {
            configuration.label
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(Color.black)
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .background(
                    RoundedRectangle(cornerRadius: Theme.radiusSmall, style: .continuous)
                        .fill(fill)
                )
                .opacity(isEnabled ? 1 : 0.4)
                .contentShape(Rectangle())
                .onHover { hovering = $0 }
        }

        private var fill: Color {
            if configuration.isPressed { return Theme.accentPressed }
            return hovering ? Theme.accentHover : Theme.accent
        }
    }
}

/// Dark outlined button for secondary actions.
struct SecondaryButtonStyle: ButtonStyle {
    var fullWidth = false

    func makeBody(configuration: Configuration) -> some View {
        SecondaryButtonBody(configuration: configuration, fullWidth: fullWidth)
    }

    private struct SecondaryButtonBody: View {
        let configuration: ButtonStyleConfiguration
        let fullWidth: Bool
        @State private var hovering = false
        @Environment(\.isEnabled) private var isEnabled

        var body: some View {
            configuration.label
                .font(.system(size: 13, weight: .medium))
                .foregroundStyle(hovering ? Theme.accentHover : Theme.textPrimary)
                .frame(maxWidth: fullWidth ? .infinity : nil, alignment: .leading)
                .padding(.horizontal, 12)
                .padding(.vertical, 7)
                .background(
                    RoundedRectangle(cornerRadius: Theme.radiusSmall, style: .continuous)
                        .fill(configuration.isPressed ? Theme.border : (hovering ? Theme.surfaceRaised : Theme.surface))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: Theme.radiusSmall, style: .continuous)
                        .strokeBorder(Theme.border, lineWidth: 1)
                )
                .opacity(isEnabled ? 1 : 0.4)
                .contentShape(Rectangle())
                .onHover { hovering = $0 }
        }
    }
}
