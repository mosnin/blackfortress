import SwiftUI
import AppKit
import ServiceManagement

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var launchAtLogin = SMAppService.mainApp.status == .enabled
    @State private var loginItemError: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                PageHeader(title: "Settings", subtitle: "Local runtime, startup and agent integration.")

                dataSection
                startupSection
                agentSection
                aboutSection
            }
            .padding(.horizontal, 32)
            .padding(.top, 40)
            .padding(.bottom, 32)
            .frame(maxWidth: 860, alignment: .leading)
        }
        .background(Theme.background)
    }

    // MARK: Data directory

    private var dataSection: some View {
        SettingsGroup(title: "Data", symbol: "externaldrive") {
            KeyValueRow(key: "Data directory", value: model.status?.dataDir ?? model.paths.dataDir.path, mono: true)
            KeyValueRow(key: "Binaries", value: model.paths.binDir?.path ?? "Not found (attach-only)", mono: true)
            KeyValueRow(key: "PostgreSQL", value: model.paths.postgresDir?.path ?? "Not bundled — bfd will look on PATH", mono: true)
            KeyValueRow(key: "Runtime", value: model.supervisor.mode.label)
            if let exit = model.supervisor.lastExitDescription {
                KeyValueRow(key: "Last exit", value: exit)
            }
            HStack(spacing: 10) {
                Button {
                    model.revealDataDirectory()
                } label: {
                    Label("Reveal in Finder", systemImage: "folder")
                }
                .buttonStyle(SecondaryButtonStyle())

                Button {
                    model.revealLogs()
                } label: {
                    Label("Show Logs", systemImage: "doc.text.magnifyingglass")
                }
                .buttonStyle(SecondaryButtonStyle())

                if model.supervisor.canRestart {
                    Button {
                        model.restartRuntime()
                    } label: {
                        Label("Restart Runtime", systemImage: "arrow.triangle.2.circlepath")
                    }
                    .buttonStyle(SecondaryButtonStyle())
                }
            }
            .padding(.top, 4)
        }
    }

    // MARK: Startup

    private var startupSection: some View {
        SettingsGroup(title: "Startup", symbol: "power") {
            Toggle(isOn: Binding(
                get: { launchAtLogin },
                set: { setLaunchAtLogin($0) }
            )) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Launch Black Fortress at login")
                        .font(Theme.body)
                        .foregroundStyle(Theme.textPrimary)
                    Text("Keeps guardrails and evidence collection running whenever you code.")
                        .font(Theme.caption)
                        .foregroundStyle(Theme.textSecondary)
                }
            }
            .toggleStyle(.switch)
            .tint(Theme.accent)

            if SMAppService.mainApp.status == .requiresApproval {
                Text("Approval required in System Settings → General → Login Items.")
                    .font(Theme.caption)
                    .foregroundStyle(Theme.warning)
            }
            if let loginItemError {
                Text(loginItemError)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.danger)
            }
        }
    }

    private func setLaunchAtLogin(_ enabled: Bool) {
        do {
            if enabled {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
            loginItemError = nil
        } catch {
            loginItemError = "Could not update login item: \(error.localizedDescription)"
        }
        launchAtLogin = SMAppService.mainApp.status == .enabled
    }

    // MARK: Agent setup

    private var installCommand: String {
        "\(shellQuote(model.paths.bfCommandPath)) install-claude"
    }

    private var projectInstallCommand: String {
        "\(shellQuote(model.paths.bfCommandPath)) install-claude --project ."
    }

    private var agentSection: some View {
        SettingsGroup(title: "Agent setup", symbol: "terminal") {
            Text("Connect Claude Code to Black Fortress: this registers the local MCP server and the guardrail hooks (`bf hook`) in your Claude Code settings.")
                .font(Theme.body)
                .foregroundStyle(Theme.textSecondary)
                .fixedSize(horizontal: false, vertical: true)

            HStack(spacing: 10) {
                Button {
                    model.installIntoClaudeCode()
                } label: {
                    Label(model.installingClaude ? "Installing…" : "Install into Claude Code", systemImage: "square.and.arrow.down")
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(model.installingClaude || model.paths.bfExecutable == nil)

                Text("Runs `bf install-claude` for your user settings.")
                    .font(Theme.caption)
                    .foregroundStyle(Theme.textTertiary)
            }

            if let result = model.installOutput {
                VStack(alignment: .leading, spacing: 6) {
                    HStack(spacing: 6) {
                        Image(systemName: result.exitCode == 0 ? "checkmark.circle.fill" : "xmark.octagon.fill")
                            .foregroundStyle(result.exitCode == 0 ? Theme.accent : Theme.danger)
                        Text(result.exitCode == 0 ? "Installed" : "Failed (exit \(result.exitCode))")
                            .font(Theme.caption)
                            .foregroundStyle(Theme.textSecondary)
                    }
                    ScrollView {
                        Text(result.output.isEmpty ? "(no output)" : result.output)
                            .font(Theme.mono)
                            .foregroundStyle(Theme.textPrimary)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .frame(maxHeight: 180)
                    .padding(10)
                    .background(RoundedRectangle(cornerRadius: Theme.radiusSmall).fill(Theme.background))
                    .overlay(RoundedRectangle(cornerRadius: Theme.radiusSmall).strokeBorder(Theme.border, lineWidth: 1))
                }
            }

            CommandBox(label: "Or run it yourself — all projects (user settings)", command: installCommand) {
                model.copyToPasteboard(installCommand, message: "Command copied")
            }
            CommandBox(label: "Current project only", command: projectInstallCommand) {
                model.copyToPasteboard(projectInstallCommand, message: "Command copied")
            }

            HStack(spacing: 10) {
                Button {
                    model.copyMCPConfig()
                } label: {
                    Label("Copy MCP config JSON", systemImage: "doc.on.doc")
                }
                .buttonStyle(SecondaryButtonStyle())

                if let mcp = model.status?.mcpURL {
                    Text(mcp.absoluteString)
                        .font(Theme.mono)
                        .foregroundStyle(Theme.textTertiary)
                        .textSelection(.enabled)
                }
            }
            Text("Other agents: run `bf agent-config cursor` or `bf agent-config codex` and paste the output into the agent's MCP settings.")
                .font(Theme.caption)
                .foregroundStyle(Theme.textTertiary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    // MARK: About

    private var aboutSection: some View {
        SettingsGroup(title: "About", symbol: "info.circle") {
            KeyValueRow(key: "App version", value: appVersion)
            KeyValueRow(key: "Runtime version", value: model.status?.version ?? "—")
            KeyValueRow(key: "Organization", value: model.status?.organizationID ?? "—", mono: true)
            KeyValueRow(key: "Console", value: model.status?.consoleURL?.absoluteString ?? "—", mono: true)
            KeyValueRow(key: "Control API", value: model.status?.controlURL?.absoluteString ?? model.client.baseURL.absoluteString, mono: true)
        }
    }

    private var appVersion: String {
        let info = Bundle.main.infoDictionary
        let short = info?["CFBundleShortVersionString"] as? String ?? "dev"
        let build = info?["CFBundleVersion"] as? String
        return build.map { "\(short) (\($0))" } ?? short
    }

    private func shellQuote(_ s: String) -> String {
        if s.rangeOfCharacter(from: CharacterSet(charactersIn: " '\"$`\\")) == nil { return s }
        return "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }
}

private struct SettingsGroup<Content: View>: View {
    let title: String
    let symbol: String
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 8) {
                Image(systemName: symbol)
                    .foregroundStyle(Theme.accent)
                Text(title)
                    .font(Theme.headline)
                    .foregroundStyle(Theme.textPrimary)
            }
            content()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .card(padding: 22)
    }
}

private struct CommandBox: View {
    let label: String
    let command: String
    let onCopy: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(Theme.caption)
                .foregroundStyle(Theme.textSecondary)
            HStack(spacing: 10) {
                Text("$")
                    .font(Theme.mono)
                    .foregroundStyle(Theme.accent)
                Text(command)
                    .font(Theme.mono)
                    .foregroundStyle(Theme.textPrimary)
                    .textSelection(.enabled)
                    .lineLimit(1)
                    .truncationMode(.head)
                Spacer(minLength: 8)
                Button(action: onCopy) {
                    Label("Copy", systemImage: "doc.on.doc")
                }
                .buttonStyle(SecondaryButtonStyle())
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(RoundedRectangle(cornerRadius: Theme.radiusSmall).fill(Theme.background))
            .overlay(RoundedRectangle(cornerRadius: Theme.radiusSmall).strokeBorder(Theme.border, lineWidth: 1))
        }
    }
}
