import SwiftUI

// MARK: - 远程访问页面

struct RemoteAccessView: View {
    @State private var remoteURL = ""
    @AppStorage("remoteBridgeURL") private var savedRemoteURL = ""
    @AppStorage("pairingIncludeTailscale") private var includeTailscale = true
    @AppStorage("pairingIncludeRemote") private var includeRemote = true
    @State private var relayConfigError: String?
    @State private var isProvisioningRelay = false
    @State private var showAdvancedConnections = false
    @State private var remoteStatus: RemoteStatus?
    @State private var isLoadingStatus = false
    @State private var statusError: String?

    var apiClient: ManagementAPIClient?

    private var localURL: String {
        remoteStatus?.localURL ?? remoteStatus?.listenStatus?.localURL ?? ""
    }

    private var tailscaleURL: String {
        remoteStatus?.tailscaleURL ?? ""
    }

    private var frpURL: String {
        remoteURL.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var frpValidationText: String {
        if frpURL.isEmpty { return "未配置" }
        return isValidManualRemoteURL(frpURL) ? "已配置" : "地址格式错误"
    }

    private var canIncludeRemote: Bool {
        !frpURL.isEmpty && isValidManualRemoteURL(frpURL)
    }

    private var isRelayEnabled: Bool {
        remoteStatus?.relay?.configured == true
    }

    private var isOfficialRelayAvailable: Bool {
        OfficialRelayConfiguration.isAvailable
    }

    private var relayStatusText: String {
        if isRelayEnabled { return "已启用" }
        if !isOfficialRelayAvailable { return "未配置" }
        if isProvisioningRelay { return "正在开通" }
        return "未启用"
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                headerSection
                qrContentsSection
                relaySection
                strategySection
                if let statusError {
                    Text(statusError)
                        .font(.caption)
                        .foregroundColor(.red)
                }
            }
            .padding(24)
            .frame(maxWidth: 760, alignment: .leading)
        }
        .task(id: apiClient?.baseURL.absoluteString) {
            await loadRemoteStatus()
        }
        .onChange(of: includeTailscale) { _, _ in
            notifyPairingConfigChanged()
        }
        .onChange(of: includeRemote) { _, _ in
            notifyPairingConfigChanged()
        }
    }

    private var headerSection: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 6) {
                Text("配对二维码")
                    .font(.system(size: 18, weight: .semibold))
                Text("勾选项会写入新生成的二维码；手机优先尝试局域网，远程方式作为兜底。")
                    .font(.caption)
                    .foregroundColor(.secondary)
            }

            Spacer()

            Button {
                Task { await loadRemoteStatus() }
            } label: {
                HStack(spacing: 5) {
                    if isLoadingStatus { ProgressView().controlSize(.small) }
                    Image(systemName: "arrow.clockwise")
                    Text("刷新")
                }
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
            .disabled(isLoadingStatus)
        }
    }

    private var qrContentsSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("此二维码让手机尝试以下方式连接本机")
                .font(.system(size: 15, weight: .semibold))

            connectionRow(
                isOn: .constant(!localURL.isEmpty),
                isToggleDisabled: true,
                icon: "wifi",
                iconColor: .blue,
                title: "局域网",
                status: localURL.isEmpty ? "未检测到" : "自动检测",
                detail: localURL.isEmpty ? "未检测到可写入二维码的局域网地址" : localURL,
                hint: "同一 Wi-Fi 下最快"
            )

            connectionRow(
                isOn: .constant(isRelayEnabled),
                isToggleDisabled: true,
                icon: "lock.shield",
                iconColor: .green,
                title: "官方加密 Relay",
                status: relayStatusText,
                detail: relayConnectionDetail,
                hint: isOfficialRelayAvailable
                    ? "Mac 与 iPhone 只需联网，无需 FRP 或 Tailscale"
                    : "公开构建不内置官方 Relay endpoint"
            )

            DisclosureGroup("高级连接方式（自托管 / 调试）", isExpanded: $showAdvancedConnections) {
                VStack(alignment: .leading, spacing: 12) {
                    connectionRow(
                        isOn: $includeTailscale,
                        isToggleDisabled: false,
                        icon: "network.badge.shields.half.filled",
                        iconColor: .cyan,
                        title: "Tailscale",
                        status: tailscaleURL.isEmpty ? "未检测到" : "自动检测",
                        detail: tailscaleURL.isEmpty ? "未检测到 100.x.x.x 地址，不会写入二维码" : tailscaleURL,
                        hint: "自托管网络能力，仅用于高级场景"
                    )

                    frpRow
                }
                .padding(.top, 8)
            }
            .font(.subheadline)
        }
    }

    private var frpRow: some View {
        VStack(alignment: .leading, spacing: 8) {
            connectionRow(
                isOn: $includeRemote,
                isToggleDisabled: false,
                icon: "server.rack",
                iconColor: .purple,
                title: "VPS / FRP",
                status: frpValidationText,
                detail: frpURL.isEmpty ? "填写 wss:// 地址后可写入二维码；未配置时不会写入" : frpURL,
                hint: "公网反向代理，适合 Tailscale 不可用时使用"
            )

            HStack(spacing: 8) {
                TextField("wss://bridge.example.com/bridge", text: $remoteURL)
                    .textFieldStyle(.roundedBorder)
                    .font(.system(size: 13, design: .monospaced))
                    .onSubmit { saveRemoteURL() }

                Button("保存 VPS 地址") {
                    saveRemoteURL()
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .disabled(remoteURL == savedRemoteURL)
            }

            if !frpURL.isEmpty && !isValidManualRemoteURL(frpURL) {
                Text("VPS / FRP 只接受 ws://、wss:// 或 https:// 格式；公网地址建议使用 wss://。")
                    .font(.caption)
                    .foregroundColor(.orange)
            }
        }
    }

    private var strategySection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("连接策略")
                .font(.system(size: 15, weight: .semibold))
            Text("同一网络时手机优先连接局域网；不在同一网络时自动使用官方加密 Relay。")
                .font(.caption)
                .foregroundColor(.secondary)
            Text("Tailscale 与 VPS / FRP 仅保留给自托管和排障场景，不是普通用户完成配对的前置条件。")
                .font(.caption)
                .foregroundColor(.secondary)
        }
    }

    private var relaySection: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("官方加密 Relay")
                    .font(.system(size: 15, weight: .semibold))
                Spacer()
                Text(relayStatusText)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
            Text(relayDescriptionText)
                .font(.caption)
                .foregroundColor(.secondary)

            if isProvisioningRelay {
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("正在启用官方 Relay...")
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
            } else if !isRelayEnabled && isOfficialRelayAvailable {
                Button("重试启用") {
                    Task { await enableOfficialRelay() }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)
            }

            if let relayConfigError {
                Text(relayConfigError)
                    .font(.caption)
                    .foregroundColor(.orange)
            }
        }
    }

    private var relayConnectionDetail: String {
        if isRelayEnabled { return "已准备好外网安全配对与连接" }
        if !isOfficialRelayAvailable { return "当前构建未配置官方 Relay endpoint" }
        return "开通后无需配置公网服务器地址"
    }

    private var relayDescriptionText: String {
        if isRelayEnabled {
            return "已启用端到端加密公网通道，新设备可直接扫描二维码完成安全配对。"
        }
        if !isOfficialRelayAvailable {
            return "公开构建未内置官方 Relay；局域网配对可用，公网 Relay 需由发布方配置 endpoint。"
        }
        return "MacBridge 会自动开通并管理加密公网通道，无需填写服务器地址或密钥。"
    }

    private func connectionRow(
        isOn: Binding<Bool>,
        isToggleDisabled: Bool,
        icon: String,
        iconColor: Color,
        title: String,
        status: String,
        detail: String,
        hint: String
    ) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Toggle("", isOn: isOn)
                .labelsHidden()
                .disabled(isToggleDisabled)
                .frame(width: 26)

            Image(systemName: icon)
                .foregroundColor(iconColor)
                .frame(width: 22)
                .padding(.top, 2)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    Text(title)
                        .font(.system(size: 14, weight: .medium))
                    Text(status)
                        .font(.caption)
                        .foregroundColor(status == "地址格式错误" ? .orange : .secondary)
                }
                Text(detail)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundColor(detail.hasPrefix("ws") ? .primary : .secondary)
                    .textSelection(.enabled)
                Text(hint)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
        }
        .padding(.vertical, 5)
    }

    private func saveRemoteURL() {
        let normalized = BridgeRemoteURLFormatter.normalize(frpURL)
        remoteURL = normalized
        if !normalized.isEmpty && !isValidManualRemoteURL(normalized) {
            includeRemote = false
            return
        }
        savedRemoteURL = normalized
        if normalized.isEmpty {
            includeRemote = false
        } else {
            includeRemote = true
        }
        notifyPairingConfigChanged()
    }

    private func notifyPairingConfigChanged() {
        NotificationCenter.default.post(name: .remoteURLDidChange, object: nil)
    }

    private func enableOfficialRelay() async {
        guard !isProvisioningRelay, !isRelayEnabled else { return }
        guard isOfficialRelayAvailable else {
            relayConfigError = "官方 Relay 启用失败：此构建未配置官方 Relay。"
            return
        }

        isProvisioningRelay = true
        defer { isProvisioningRelay = false }
        do {
            _ = try await OfficialRelayProvisioner.shared.ensureRoute()
            relayConfigError = nil
            notifyPairingConfigChanged()
        } catch {
            relayConfigError = "官方 Relay 启用失败：\(error.localizedDescription)"
        }
    }

    private func isValidManualRemoteURL(_ value: String) -> Bool {
        guard let url = URL(string: value),
              let scheme = url.scheme?.lowercased(),
              url.host != nil else {
            return false
        }
        return scheme == "ws" || scheme == "wss" || scheme == "https"
    }

    private func loadRemoteStatus() async {
        guard let client = apiClient else { return }
        isLoadingStatus = true
        defer { isLoadingStatus = false }
        do {
            remoteStatus = try await client.getRemoteStatus()
            statusError = nil
            if let url = remoteStatus?.remoteURL, !url.isEmpty {
                remoteURL = url
            } else {
                remoteURL = savedRemoteURL
            }
        } catch {
            statusError = error.localizedDescription
        }
    }
}

private enum BridgeRemoteURLFormatter {
    static func normalize(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.lowercased().hasPrefix("https://") {
            return "wss" + String(trimmed.dropFirst(5))
        }
        return trimmed
    }
}
