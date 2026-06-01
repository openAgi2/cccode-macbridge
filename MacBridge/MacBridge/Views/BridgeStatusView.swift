import SwiftUI

// MARK: - Overview 页面

/// Bridge 运行状态总览：状态摘要 + AI Tools 摘要 + 设备摘要 + 远程访问
struct BridgeStatusView: View {
    @ObservedObject var viewModel: BridgeStatusViewModel
    @ObservedObject var backendViewModel: BackendStatusViewModel
    let devices: [TrustedDevice]
    let hasLoadedDevices: Bool
    let onStartBridge: () -> Void
    let onStopBridge: () -> Void
    let onRestartBridge: () -> Void

    /// 外部注入：切换到 Devices tab 的回调
    var onNavigateToDevices: (() -> Void)?
    /// 外部注入：触发配对的回调
    var onPairDevice: (() -> Void)?

    private var readyAgentCount: Int {
        overviewAgents.filter { $0.status == "available" }.count
    }

    private var overviewAgents: [BackendAgentStatus] {
        if !backendViewModel.agents.isEmpty {
            return backendViewModel.agents
        }
        return viewModel.agents.map {
            BackendAgentStatus(
                id: $0.id,
                displayName: $0.displayName,
                kind: $0.kind,
                status: $0.status,
                reason: $0.reason,
                isRefreshing: false,
                requiresPollingForExternalTurns: $0.requiresPollingForExternalTurns
            )
        }
    }

    private var lastSeenText: String? {
        guard let lastSeen = devices.compactMap(\.lastSeenAt).sorted().last else { return nil }
        return Self.relativeTimeString(lastSeen)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 26) {
                headerSection

                sectionBlock {
                    statusSummarySection
                }

                sectionBlock {
                    aiToolsSummarySection
                }

                sectionBlock {
                    devicesSummarySection
                }
            }
            .padding(.horizontal, 38)
            .padding(.top, 34)
            .padding(.bottom, 48)
            .frame(maxWidth: 820, alignment: .leading)
            .frame(maxWidth: .infinity)
        }
    }

    // MARK: - Header

    private var headerSection: some View {
        HStack(alignment: .top, spacing: 14) {
            statusIconBadge

            VStack(alignment: .leading, spacing: 12) {
                Text(L10n.ccCodeBridge)
                    .font(.system(size: 26, weight: .semibold))
                Text(viewModel.statusText)
                    .font(.subheadline)
                    .foregroundColor(.secondary)

                actionsSection
            }

            Spacer()
        }
    }

    // MARK: - 主操作按钮

    private var actionsSection: some View {
        HStack(spacing: 12) {
            if let onPair = onPairDevice {
                Button(action: onPair) {
                    Label(L10n.pairNewDevice, systemImage: "qrcode")
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
            }
            if let onDevices = onNavigateToDevices, !devices.isEmpty {
                Button(action: onDevices) {
                    Label(L10n.devices, systemImage: "iphone")
                }
                .buttonStyle(.bordered)
                .controlSize(.large)
            }
        }
    }

    // MARK: - 状态摘要

    private var statusSummarySection: some View {
        VStack(alignment: .leading, spacing: 14) {
            sectionHeader(L10n.status)

            HStack(spacing: 16) {
                statusRow(
                    icon: "checkmark.circle.fill",
                    color: viewModel.status == .ready ? .green : .orange,
                    title: viewModel.status == .ready ? L10n.bridgeRunning : viewModel.statusText,
                    detail: viewModel.lastError
                )
                Spacer()
                runtimeControls
            }
        }
    }

    private var runtimeControls: some View {
        HStack(spacing: 8) {
            switch viewModel.status {
            case .ready, .readyNoAgents:
                Button(L10n.stop) {
                    onStopBridge()
                }
                .buttonStyle(.bordered)
                .controlSize(.small)

                Button(L10n.restart) {
                    onRestartBridge()
                }
                .buttonStyle(.bordered)
                .controlSize(.small)

            case .starting:
                Button(L10n.stop) {
                    onStopBridge()
                }
                .buttonStyle(.bordered)
                .controlSize(.small)

            case .idle, .stopped, .crashed, .sleeping:
                Button(L10n.start) {
                    onStartBridge()
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)
            }
        }
    }

    private func statusRow(icon: String, color: Color, title: String, detail: String?) -> some View {
        HStack(spacing: 9) {
            Image(systemName: icon)
                .foregroundColor(color)
                .font(.system(size: 15, weight: .semibold))
                .frame(width: 18)
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.system(size: 15, weight: .medium))
                if let detail {
                    Text(detail)
                        .font(.caption)
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                }
            }
        }
    }

    // MARK: - AI Tools 摘要

    private var aiToolsSummarySection: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                sectionHeader(L10n.aiTools)
                Spacer()
                Button {
                    Task { await backendViewModel.refreshAgents() }
                } label: {
                    HStack(spacing: 4) {
                        if backendViewModel.isLoading {
                            ProgressView()
                                .controlSize(.small)
                        }
                        Image(systemName: "arrow.clockwise")
                        Text(L10n.refreshAll)
                    }
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .disabled(backendViewModel.isLoading)
            }

            if let error = backendViewModel.errorMessage {
                Label(error, systemImage: backendViewModel.isShowingStaleResults ? "exclamationmark.triangle" : "xmark.circle")
                    .font(.caption)
                    .foregroundColor(backendViewModel.isShowingStaleResults ? .orange : .red)
            }

            if overviewAgents.isEmpty {
                Text(L10n.noAiToolsDetected)
                    .foregroundColor(.secondary)
                    .font(.subheadline)
            } else {
                VStack(spacing: 0) {
                    ForEach(overviewAgents) { agent in
                        aiToolRow(agent)
                        if agent.id != overviewAgents.last?.id {
                            Hairline()
                                .padding(.leading, 22)
                        }
                    }
                }
            }
        }
    }

    private func aiToolRow(_ agent: BackendAgentStatus) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Circle()
                    .fill(agent.isAvailable ? Color.green : Color.orange)
                    .frame(width: 8, height: 8)

                Text(agent.displayName)
                    .font(.system(size: 15, weight: .medium))

                Spacer()

                statusBadge(agent.displayStatus, color: agent.isAvailable ? .green : .orange)

                Button(L10n.test) {
                    Task { await backendViewModel.testAgent(id: agent.id) }
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .disabled(agent.isRefreshing)

                if agent.isRefreshing {
                    ProgressView()
                        .controlSize(.small)
                }
            }

            if !agent.isAvailable {
                Text(Self.nextStepGuidance(agent.reason, kind: agent.kind))
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .padding(.leading, 16)
            }

            if agent.isAvailable && agent.requiresPollingForExternalTurns {
                Text(L10n.externalTurnsPolling)
                    .font(.caption2)
                    .foregroundColor(.secondary)
                    .padding(.leading, 16)
            }
        }
        .padding(.vertical, 11)
    }

    // MARK: - 设备摘要

    private var devicesSummarySection: some View {
        VStack(alignment: .leading, spacing: 14) {
            sectionHeader(L10n.devices)

            if !hasLoadedDevices {
                HStack(spacing: 6) {
                    Image(systemName: "clock")
                        .foregroundColor(.secondary)
                        .frame(width: 16)
                    Text(L10n.loadingDevices)
                        .font(.body)
                        .foregroundColor(.secondary)
                }
            } else if !devices.isEmpty {
                HStack(spacing: 6) {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundColor(.green)
                        .frame(width: 16)
                    Text(String(format: L10n.tr("trusted_devices"), devices.count))
                        .font(.body)
                    if let last = lastSeenText {
                        Text(String(format: L10n.lastConnected, last))
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                }
            } else {
                HStack(spacing: 6) {
                    Image(systemName: "questionmark.circle")
                        .foregroundColor(.secondary)
                        .frame(width: 16)
                    Text(L10n.noTrustedDevices)
                        .font(.body)
                        .foregroundColor(.secondary)
                }
            }
        }
    }

    // MARK: - 状态图标

    @ViewBuilder
    private func sectionBlock<Content: View>(@ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Hairline()
                .padding(.bottom, 24)
            content()
        }
    }

    private func sectionHeader(_ title: String) -> some View {
        Text(title)
            .font(.system(size: 15, weight: .semibold))
            .foregroundColor(.primary)
    }

    private func statusBadge(_ text: String, color: Color) -> some View {
        Text(text)
            .font(.system(size: 12, weight: .semibold))
            .foregroundColor(color)
            .padding(.horizontal, 7)
            .padding(.vertical, 3)
            .background(
                Capsule()
                    .fill(color.opacity(0.12))
            )
    }

    @ViewBuilder
    private var statusIconBadge: some View {
        statusIcon
            .font(.system(size: 15, weight: .semibold))
            .frame(width: 28, height: 28)
            .background(
                Circle()
                    .fill(Color.green.opacity(viewModel.status == .ready ? 0.14 : 0))
            )
    }

    @ViewBuilder
    private var statusIcon: some View {
        switch viewModel.status {
        case .ready:
            Image(systemName: "checkmark.circle.fill")
                .foregroundColor(.green)
        case .readyNoAgents:
            Image(systemName: "exclamationmark.circle.fill")
                .foregroundColor(.orange)
        case .crashed:
            Image(systemName: "xmark.circle.fill")
                .foregroundColor(.red)
        case .starting:
            ProgressView()
                .controlSize(.small)
        case .stopped:
            Image(systemName: "stop.circle")
                .foregroundColor(.gray)
        case .sleeping:
            Image(systemName: "moon.fill")
                .foregroundColor(.blue)
        case .idle:
            Image(systemName: "pc")
                .foregroundColor(.orange)
        }
    }

    static func displayStatus(_ status: String) -> String {
        switch status {
        case "available": return L10n.statusReady
        case "not_detected": return L10n.statusNotFound
        case "not_logged_in": return L10n.statusLoginRequired
        case "service_not_running": return L10n.statusNotRunning
        case "port_conflict": return L10n.statusPortConflict
        case "version_unsupported": return L10n.statusVersionIncompatible
        case "permission_denied": return L10n.statusPermissionDenied
        default: return status
        }
    }

    private static func nextStepGuidance(_ reason: String?, kind: String) -> String {
        guard let reason, !reason.isEmpty else {
            return L10n.checkDocsGuidance
        }
        if reason.contains("not found in PATH") {
            return String(format: L10n.notInstalled, kind)
        }
        if reason.contains("not running") {
            return L10n.serviceNotRunning
        }
        if reason.contains("not logged in") {
            return L10n.loginRequired
        }
        if reason.contains("timed out") {
            return L10n.detectionTimedOut
        }
        if reason.contains("unreachable") {
            return L10n.cannotReachService
        }
        return reason
    }

    private static func relativeTimeString(_ isoString: String) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        guard let date = formatter.date(from: isoString) else { return isoString }
        let interval = Date().timeIntervalSince(date)
        if interval < 60 { return L10n.justNow }
        if interval < 3600 { return String(format: L10n.tr("min_ago"), Int(interval / 60)) }
        if interval < 86400 { return String(format: L10n.tr("hr_ago"), Int(interval / 3600)) }
        return String(format: L10n.tr("days_ago"), Int(interval / 86400))
    }
}

private struct Hairline: View {
    var body: some View {
        Rectangle()
            .fill(Color(NSColor.separatorColor).opacity(0.55))
            .frame(height: 0.5)
    }
}

#Preview {
    let vm = BridgeStatusViewModel()
    vm.status = .ready
    vm.statusText = "Bridge is ready"
    vm.agents = [
        AgentInfo(id: "1", kind: "claude", displayName: "Claude Code", status: "available", reason: nil, liveEvents: "stream", requiresPollingForExternalTurns: true),
        AgentInfo(id: "2", kind: "codex", displayName: "Codex", status: "available", reason: nil, liveEvents: "stream", requiresPollingForExternalTurns: false),
    ]
    return BridgeStatusView(
        viewModel: vm,
        backendViewModel: BackendStatusViewModel(),
        devices: [],
        hasLoadedDevices: true,
        onStartBridge: {},
        onStopBridge: {},
        onRestartBridge: {}
    )
}
