import SwiftUI
import AppKit
import WebKit

/// Embedded Probo console. Loads http://localhost:7811/login, which signs the
/// local user in and redirects to the console on localhost:7810.
struct ConsoleView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        // The browser lives on AppModel so the session survives switching tabs.
        ConsoleContent(browser: model.consoleBrowser)
    }
}

private struct ConsoleContent: View {
    @EnvironmentObject private var model: AppModel
    @ObservedObject var browser: ConsoleBrowser

    var body: some View {
        VStack(spacing: 0) {
            toolbar
            Rectangle().fill(Theme.border).frame(height: 1)
            ZStack {
                Theme.background
                if model.connection == .connected {
                    ConsoleWebView(browser: browser)
                        .opacity(browser.hasLoaded ? 1 : 0)
                    if !browser.hasLoaded {
                        loading
                    }
                } else {
                    EmptyStateView(
                        symbol: "globe",
                        title: "Console unavailable",
                        message: "The console opens once the local runtime is running. \(model.supervisor.mode.label)."
                    )
                }
                if let error = browser.lastError {
                    VStack {
                        Spacer()
                        Text(error)
                            .font(Theme.caption)
                            .foregroundStyle(Theme.warning)
                            .padding(10)
                            .card(padding: 0, raised: true)
                            .padding(16)
                    }
                }
            }
        }
        .padding(.top, 28)
        .background(Theme.background)
        .onAppear {
            // Fresh one-time login link every time the tab is opened.
            if model.connection == .connected { browser.signIn() }
        }
        .onChange(of: model.connection) { oldValue, newValue in
            if newValue == .connected && oldValue != .connected { browser.signIn() }
        }
    }

    private var toolbar: some View {
        HStack(spacing: 10) {
            Button { browser.webView?.goBack() } label: { Image(systemName: "chevron.left") }
                .buttonStyle(SecondaryButtonStyle())
                .disabled(!browser.canGoBack)
            Button { browser.webView?.goForward() } label: { Image(systemName: "chevron.right") }
                .buttonStyle(SecondaryButtonStyle())
                .disabled(!browser.canGoForward)
            Button { browser.reload() } label: { Image(systemName: "arrow.clockwise") }
                .buttonStyle(SecondaryButtonStyle())

            Text(browser.currentURL?.absoluteString ?? BFDClient.browserBaseURL.appendingPathComponent("login").absoluteString)
                .font(Theme.mono)
                .foregroundStyle(Theme.textSecondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: Theme.radiusSmall).fill(Theme.surface))
                .overlay(RoundedRectangle(cornerRadius: Theme.radiusSmall).strokeBorder(Theme.border, lineWidth: 1))

            Button {
                if let url = browser.currentURL ?? model.status?.consoleURL {
                    NSWorkspace.shared.open(url)
                }
            } label: {
                Label("Open in Browser", systemImage: "arrow.up.right.square")
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }

    private var loading: some View {
        VStack(spacing: 12) {
            ProgressView()
                .controlSize(.small)
            Text("Signing in to the local console…")
                .font(Theme.body)
                .foregroundStyle(Theme.textSecondary)
        }
    }
}

/// Holds the WKWebView and its navigation state across SwiftUI updates.
@MainActor
final class ConsoleBrowser: NSObject, ObservableObject, WKNavigationDelegate, WKUIDelegate {
    @Published private(set) var canGoBack = false
    @Published private(set) var canGoForward = false
    @Published private(set) var currentURL: URL?
    @Published private(set) var hasLoaded = false
    @Published private(set) var lastError: String?

    private(set) var webView: WKWebView?
    private var pendingURL: URL?
    private var signInTask: Task<Void, Never>?
    private let client = BFDClient()

    static let allowedHosts: Set<String> = ["localhost", "127.0.0.1", "::1", "[::1]"]

    func makeWebView() -> WKWebView {
        if let existing = webView { return existing }
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .default()
        let view = WKWebView(frame: .zero, configuration: config)
        view.navigationDelegate = self
        view.uiDelegate = self
        view.allowsBackForwardNavigationGestures = true
        view.underPageBackgroundColor = Theme.nsBackground
        webView = view
        if let pending = pendingURL {
            pendingURL = nil
            view.load(URLRequest(url: pending))
        }
        return view
    }

    /// Signs in with a fresh one-time link from GET /v1/login-link and loads it.
    /// /login itself requires a single-use nonce (60 s TTL), so this runs every
    /// time the Console tab is opened or reloaded.
    func signIn() {
        hasLoaded = false
        lastError = nil
        signInTask?.cancel()
        signInTask = Task { [weak self] in
            guard let self else { return }
            do {
                let url = try await self.client.loginLink()
                if Task.isCancelled { return }
                if let view = self.webView {
                    view.load(URLRequest(url: url))
                } else {
                    // The web view is created lazily by SwiftUI; load on creation.
                    self.pendingURL = url
                }
            } catch {
                if Task.isCancelled { return }
                self.hasLoaded = true
                self.lastError = "Could not get a console sign-in link from bfd: \(error.localizedDescription)"
            }
        }
    }

    func reload() {
        signIn()
    }

    static func isLocal(_ url: URL?) -> Bool {
        guard let url else { return false }
        let scheme = url.scheme?.lowercased() ?? ""
        if scheme == "about" || scheme == "blob" || scheme == "data" { return true }
        guard scheme == "http" || scheme == "https" || scheme == "ws" || scheme == "wss" else { return false }
        return allowedHosts.contains(url.host?.lowercased() ?? "")
    }

    private func syncState(_ webView: WKWebView) {
        canGoBack = webView.canGoBack
        canGoForward = webView.canGoForward
        currentURL = webView.url
    }

    // MARK: WKNavigationDelegate

    // The async form of webView(_:decidePolicyFor:decisionHandler:) is used
    // on purpose: its signature is identical across SDKs, whereas the
    // completion-handler form gained @MainActor @Sendable annotations in newer
    // SDKs, and a mismatch on an optional ObjC requirement fails silently.
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction) async -> WKNavigationActionPolicy {
        let url = navigationAction.request.url
        if ConsoleBrowser.isLocal(url) {
            return .allow
        }
        // Anything off-box opens in the default browser, but only for
        // top-level navigations (not subframe loads such as embeds).
        if let url, navigationAction.targetFrame?.isMainFrame ?? true {
            NSWorkspace.shared.open(url)
        }
        return .cancel
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        hasLoaded = true
        lastError = nil
        syncState(webView)
    }

    func webView(_ webView: WKWebView, didCommit navigation: WKNavigation!) {
        syncState(webView)
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        handle(error, webView)
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        handle(error, webView)
    }

    private func handle(_ error: Error, _ webView: WKWebView) {
        let ns = error as NSError
        // -999 = cancelled (e.g. our own policy cancel); not a real failure.
        if ns.domain == NSURLErrorDomain && ns.code == NSURLErrorCancelled { return }
        if ns.domain == "WebKitErrorDomain" && ns.code == 102 { return } // frame load interrupted
        hasLoaded = true
        lastError = "Could not load console: \(error.localizedDescription)"
        syncState(webView)
    }

    // MARK: WKUIDelegate

    /// target="_blank" / window.open: load local URLs in place, open the rest externally.
    func webView(
        _ webView: WKWebView,
        createWebViewWith configuration: WKWebViewConfiguration,
        for navigationAction: WKNavigationAction,
        windowFeatures: WKWindowFeatures
    ) -> WKWebView? {
        if let url = navigationAction.request.url {
            if ConsoleBrowser.isLocal(url) {
                webView.load(URLRequest(url: url))
            } else {
                NSWorkspace.shared.open(url)
            }
        }
        return nil
    }
}

@MainActor
struct ConsoleWebView: NSViewRepresentable {
    let browser: ConsoleBrowser

    func makeNSView(context: Context) -> WKWebView {
        browser.makeWebView()
    }

    func updateNSView(_ nsView: WKWebView, context: Context) {}
}
