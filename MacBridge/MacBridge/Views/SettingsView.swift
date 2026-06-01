import SwiftUI

/// 设置页面：编辑后端认证信息
struct SettingsView: View {
    @ObservedObject var viewModel: SettingsViewModel
    @AppStorage("appLanguage") private var appLanguage: String = ""
    @AppStorage("appTheme") private var appTheme: String = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Text(L10n.settings)
                    .font(.title2)
                    .fontWeight(.semibold)

                Divider()

                // 语言设置
                VStack(alignment: .leading, spacing: 12) {
                    HStack {
                        Image(systemName: "globe")
                            .foregroundColor(.secondary)
                        Text(L10n.language)
                            .font(.headline)
                    }

                    Picker("", selection: $appLanguage) {
                        ForEach(AppLanguage.allCases) { lang in
                            Text(lang.displayName).tag(lang.rawValue)
                        }
                    }
                    .pickerStyle(.menu)
                }
                .padding(12)
                .glassPanel()

                // 外观设置
                VStack(alignment: .leading, spacing: 12) {
                    HStack {
                        Image(systemName: "circle.lefthalf.filled")
                            .foregroundColor(.secondary)
                        Text(L10n.appearance)
                            .font(.headline)
                    }

                    Picker("", selection: $appTheme) {
                        ForEach(AppTheme.allCases) { theme in
                            Text(theme.displayName).tag(theme.rawValue)
                        }
                    }
                    .pickerStyle(.segmented)
                    .frame(maxWidth: 320)
                }
                .padding(12)
                .glassPanel()

                Divider()

                // Bridge 显示名称
                VStack(alignment: .leading, spacing: 12) {
                    HStack {
                        Image(systemName: "desktopcomputer")
                            .foregroundColor(.secondary)
                        Text(L10n.bridgeName)
                            .font(.headline)
                    }

                    Text(L10n.bridgeNameHint)
                        .font(.caption)
                        .foregroundColor(.secondary)

                    HStack(spacing: 8) {
                        TextField("Mac", text: $viewModel.displayName)
                            .textFieldStyle(.roundedBorder)

                        Button {
                            viewModel.saveDisplayName()
                        } label: {
                            Text(L10n.save)
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(viewModel.displayName.trimmingCharacters(in: .whitespaces).isEmpty)

                        if let msg = viewModel.displayNameMessage {
                            Text(msg)
                                .font(.caption)
                                .foregroundColor(msg.contains("failed") ? .red : .green)
                        }
                    }
                }
                .padding(12)
                .glassPanel()

                Divider()

                // OpenCode 认证
                VStack(alignment: .leading, spacing: 12) {
                    HStack {
                        Image(systemName: "server.rack")
                            .foregroundColor(.secondary)
                        Text(L10n.openCodeAuth)
                            .font(.headline)
                    }

                    Text(L10n.openCodeAuthHint)
                        .font(.caption)
                        .foregroundColor(.secondary)
                        .fixedSize(horizontal: false, vertical: true)

                    VStack(alignment: .leading, spacing: 8) {
                        Text(L10n.openCodeAuthGuidanceAuto)
                            .font(.caption)
                            .foregroundColor(.secondary)
                            .fixedSize(horizontal: false, vertical: true)

                        Divider()
                            .padding(.vertical, 2)

                        Text(L10n.openCodeAuthGuidanceManual)
                            .font(.caption)
                            .foregroundColor(.secondary)
                            .fixedSize(horizontal: false, vertical: true)

                        Text("opencode serve --port 64667 --hostname 127.0.0.1")
                            .font(.system(.caption, design: .monospaced))
                            .padding(6)
                            .background(Color(NSColor.textBackgroundColor))
                            .cornerRadius(4)
                            .textSelection(.enabled)
                    }
                    .padding(10)
                    .background(Color.secondary.opacity(0.06))
                    .cornerRadius(8)

                    VStack(alignment: .leading, spacing: 6) {
                        Text(L10n.username)
                            .font(.subheadline)
                            .fontWeight(.medium)
                        TextField("opencode", text: $viewModel.opencodeUser)
                            .textFieldStyle(.roundedBorder)
                    }

                    VStack(alignment: .leading, spacing: 6) {
                        Text(L10n.password)
                            .font(.subheadline)
                            .fontWeight(.medium)
                        SecureField("password", text: $viewModel.opencodePass)
                            .textFieldStyle(.roundedBorder)
                    }

                    HStack {
                        Button {
                            viewModel.saveCredentials()
                        } label: {
                            HStack(spacing: 4) {
                                if viewModel.isSaving {
                                    ProgressView()
                                        .controlSize(.small)
                                }
                                Text(L10n.saveAndRestart)
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(viewModel.isSaving)

                        if let msg = viewModel.saveMessage {
                            Text(msg)
                                .font(.caption)
                                .foregroundColor(msg.contains("failed") ? .red : .green)
                        }
                    }
                }
                .padding(12)
                .glassPanel()

                Spacer()
            }
            .padding()
            .frame(maxWidth: 680)
            .frame(maxWidth: .infinity, alignment: .center)
        }
    }
}

#Preview {
    let vm = SettingsViewModel(dataDir: "/tmp/CCCode Bridge", onCredentialsChanged: {})
    vm.opencodeUser = "opencode"
    return SettingsView(viewModel: vm)
}
