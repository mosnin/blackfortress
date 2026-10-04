import SwiftUI
import AppKit

@main
@MainActor
struct BlackFortressApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel.shared

    var body: some Scene {
        Window("Black Fortress", id: WindowID.main) {
            MainWindow()
                .environmentObject(model)
                .preferredColorScheme(.dark)
                .frame(minWidth: 960, minHeight: 620)
        }
        .windowStyle(.hiddenTitleBar)
        .defaultSize(width: 1200, height: 780)
        .commands {
            CommandGroup(replacing: .newItem) {}
        }

        MenuBarExtra {
            MenuBarContent()
                .environmentObject(model)
                .preferredColorScheme(.dark)
        } label: {
            MenuBarIcon(level: model.health)
        }
        .menuBarExtraStyle(.window)
    }
}

enum WindowID {
    static let main = "main"
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var terminating = false
    private var signalSources: [DispatchSourceSignal] = []

    func applicationDidFinishLaunching(_ notification: Notification) {
        installSignalHandlers()
        NSApp.appearance = NSAppearance(named: .darkAqua)
        NotificationManager.shared.setUp()
        AppModel.shared.start()
    }

    /// `kill`/`pkill` send SIGTERM, which would end the app without running
    /// applicationShouldTerminate and leave the managed bfd running. Route
    /// SIGTERM and SIGINT through the normal Quit path instead.
    private func installSignalHandlers() {
        for sig in [SIGTERM, SIGINT] {
            signal(sig, SIG_IGN)
            let source = DispatchSource.makeSignalSource(signal: sig, queue: .main)
            source.setEventHandler {
                NSApp.terminate(nil)
            }
            source.resume()
            signalSources.append(source)
        }
    }

    // Keep running in the menu bar when the main window is closed.
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    // Reopen the main window when the Dock icon is clicked with no windows open.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag {
            for window in sender.windows where window.identifier?.rawValue.hasPrefix(WindowID.main) == true {
                window.makeKeyAndOrderFront(nil)
                return false
            }
        }
        return true
    }

    /// Stop the managed bfd (SIGTERM + wait) before the app exits.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if terminating { return .terminateLater }
        let model = AppModel.shared
        guard model.supervisor.ownsProcess else { return .terminateNow }
        terminating = true
        Task { @MainActor in
            await model.shutdown()
            NSApp.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}
