import Foundation
import UserNotifications

/// Posts a user notification when a guardrail blocks an agent action.
///
/// UNUserNotificationCenter requires a real app bundle (it crashes when the
/// process has no bundle identifier, e.g. a bare `swift run`), so every call
/// is guarded by `isAvailable`.
final class NotificationManager: NSObject, UNUserNotificationCenterDelegate {
    static let shared = NotificationManager()

    private var authorized = false

    var isAvailable: Bool {
        Bundle.main.bundleIdentifier != nil && Bundle.main.bundleURL.pathExtension == "app"
    }

    func setUp() {
        guard isAvailable else { return }
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound]) { [weak self] granted, _ in
            DispatchQueue.main.async {
                self?.authorized = granted
            }
        }
    }

    func notifyBlocked(_ entry: LedgerEntry) {
        guard isAvailable else { return }
        let content = UNMutableNotificationContent()
        content.title = "Guardrail blocked an agent action"
        var parts: [String] = []
        if let tool = entry.tool { parts.append(tool) }
        if let target = entry.target { parts.append(target) }
        content.subtitle = parts.joined(separator: " · ")
        var body: [String] = []
        if let reason = entry.reason { body.append(reason) }
        if !entry.controls.isEmpty { body.append("Controls: " + entry.controls.joined(separator: ", ")) }
        content.body = body.joined(separator: "\n")
        content.sound = .default

        let request = UNNotificationRequest(
            identifier: "guardrail-\(entry.id)",
            content: content,
            trigger: nil
        )
        UNUserNotificationCenter.current().add(request) { _ in }
    }

    // Show banners even while the app is frontmost. (Async form: its
    // signature does not depend on SDK concurrency annotations.)
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .sound, .list]
    }
}
