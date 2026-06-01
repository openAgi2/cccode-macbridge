import SwiftUI

// MARK: - 导航枚举

/// 主窗口 tab 定义，为后续 Sidebar 化准备
enum NavigationTab: String, CaseIterable, Identifiable {
    case overview
    case devices
    case remoteAccess
    case settings
    case diagnostics

    var id: String { rawValue }

    var title: String {
        switch self {
        case .overview: return L10n.overview
        case .devices: return L10n.devicesPairing
        case .remoteAccess: return L10n.remoteAccessTab
        case .settings: return L10n.settings
        case .diagnostics: return L10n.logsDiagnostics
        }
    }

    var systemImage: String {
        switch self {
        case .overview: return "circle.hexagonpath"
        case .devices: return "lock.shield"
        case .remoteAccess: return "antenna.radiowaves.left.and.right"
        case .settings: return "gearshape"
        case .diagnostics: return "doc.text"
        }
    }
}

/// 主窗口内容，承载各功能标签页
struct ContentView: View {
    @ObservedObject var viewModel: BridgeStatusViewModel
    @ObservedObject var pairingViewModel: PairingViewModel
    @ObservedObject var settingsViewModel: SettingsViewModel
    @StateObject private var backendVM = BackendStatusViewModel()
    @State private var devices: [TrustedDevice] = []
    @State private var logs: [String] = []
    @State private var isLoadingLogs = false
    @State private var hasLoadedDevices = false

    @State private var errorMessage: String?
    @State private var showErrorAlert = false
    @State private var selectedTab: NavigationTab = .overview
    @EnvironmentObject private var dependencies: AppDependencies

    var body: some View {
        HStack(spacing: 0) {
            sidebar

            Divider()

            currentTabContent
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .frame(minWidth: 560, minHeight: 480)
        .background(Color(NSColor.windowBackgroundColor))
        .alert(isPresented: $showErrorAlert) {
            Alert(
                title: Text(L10n.error),
                message: Text(errorMessage ?? L10n.unknownError),
                dismissButton: .default(Text(L10n.ok))
            )
        }
        .onChange(of: selectedTab) { _, tab in
            if tab == .overview { Task { await loadDevices() } }
            if tab == .devices { Task { await loadDevices() } }
        }
        .onAppear {
            configureBackendClientIfAvailable()
        }
        .task {
            await loadDevices()
            configureBackendClientIfAvailable()
            await backendVM.loadAgents()
        }
    }

    // MARK: - Navigation

    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(NavigationTab.allCases) { tab in
                Button {
                    selectedTab = tab
                } label: {
                    Label(tab.title, systemImage: tab.systemImage)
                        .labelStyle(.titleAndIcon)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .font(.system(size: 14, weight: selectedTab == tab ? .semibold : .medium))
                        .padding(.horizontal, 12)
                        .padding(.vertical, 9)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .foregroundColor(selectedTab == tab ? .primary : .secondary)
                .background {
                    if selectedTab == tab {
                        RoundedRectangle(cornerRadius: 8)
                            .fill(Color.accentColor.opacity(0.18))
                    }
                }
            }

            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.top, 16)
        .padding(.bottom, 12)
        .frame(width: 188)
        .background(Color(NSColor.controlBackgroundColor).opacity(0.35))
    }

    @ViewBuilder
    private var currentTabContent: some View {
        switch selectedTab {
        case .overview:
            overviewTab
        case .devices:
            devicesTab
        case .remoteAccess:
            remoteAccessTab
        case .settings:
            SettingsView(viewModel: settingsViewModel)
        case .diagnostics:
            logsTab
        }
    }

    // MARK: - Overview Tab

    private var overviewTab: some View {
        BridgeStatusView(
            viewModel: viewModel,
            backendViewModel: backendVM,
            devices: devices,
            hasLoadedDevices: hasLoadedDevices,
            onStartBridge: {
                dependencies.runtimeManager.start()
            },
            onStopBridge: {
                dependencies.runtimeManager.stop()
            },
            onRestartBridge: {
                dependencies.runtimeManager.restart()
            },
            onNavigateToDevices: {
                selectedTab = .devices
            },
            onPairDevice: {
                selectedTab = .devices
                pairingViewModel.startPairing()
            }
        )
    }

    // MARK: - Devices Tab

    @State private var deviceToRemove: TrustedDevice?
    @State private var showRemoveConfirmation = false

    private var remoteAccessTab: some View {
        RemoteAccessView(apiClient: apiClient)
    }

    private var devicesTab: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // 配对区域
                PairingView(viewModel: pairingViewModel)

                Divider()

                // 设备列表
                Text(L10n.authorizedDevices)
                    .font(.headline)

                if devices.isEmpty {
                    HStack(spacing: 6) {
                        Image(systemName: "info.circle")
                            .foregroundColor(.secondary)
                        Text(L10n.noAuthorizedDevices)
                            .foregroundColor(.secondary)
                            .font(.subheadline)
                    }
                } else {
                    ForEach(devices) { device in
                        deviceRow(device)
                    }
                }
            }
            .padding()
            .frame(maxWidth: 680)
        }
        .confirmationDialog(
            String(format: L10n.tr("remove_device_confirm"), deviceToRemove?.displayName ?? "Device"),
            isPresented: $showRemoveConfirmation,
            titleVisibility: .visible
        ) {
            Button(L10n.remove, role: .destructive) {
                if let device = deviceToRemove {
                    Task { await revokeDevice(device) }
                }
            }
            Button(L10n.cancel, role: .cancel) {}
        } message: {
            Text(L10n.removeDeviceMessage)
        }
    }

    private func deviceRow(_ device: TrustedDevice) -> some View {
        HStack(spacing: 10) {
            Image(systemName: device.platform == "ios" ? "iphone" : "desktopcomputer")
                .foregroundColor(.secondary)
                .frame(width: 20)

            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Text(device.displayName ?? device.deviceId)
                        .fontWeight(.medium)
                    // 同名设备显示 ID 后 6 位
                    if devices.filter({ $0.displayName == device.displayName }).count > 1 {
                        Text("(\(String(device.deviceId.suffix(6))))")
                            .font(.caption2)
                            .foregroundColor(.secondary)
                    }
                }
                HStack(spacing: 8) {
                    if let platform = device.platform {
                        Text(platform)
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                    if let created = device.createdAt {
                        Text(String(format: L10n.paired, Self.relativeTimeString(created)))
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                    if let lastSeen = device.lastSeenAt {
                        Text(String(format: L10n.lastSeen, Self.relativeTimeString(lastSeen)))
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                }
            }

            Spacer()

            Button(L10n.remove, role: .destructive) {
                deviceToRemove = device
                showRemoveConfirmation = true
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
        }
        .padding(.vertical, 6)
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

    // MARK: - Diagnostics Tab

    private var logsTab: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(L10n.rawLogs)
                    .font(.title2)
                    .fontWeight(.semibold)
                Spacer()
                Button {
                    Task { await loadLogs() }
                } label: {
                    HStack(spacing: 4) {
                        if isLoadingLogs {
                            ProgressView()
                                .controlSize(.small)
                        }
                        Image(systemName: "arrow.clockwise")
                        Text(L10n.refreshAll)
                    }
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .disabled(isLoadingLogs)

                Button(L10n.copyRawLogs) {
                    let text = logs.joined(separator: "\n")
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(text, forType: .string)
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .disabled(logs.isEmpty)
            }

            Text(L10n.last200Lines)
                .font(.caption)
                .foregroundColor(.secondary)

            if logs.isEmpty {
                Text(L10n.noLogsAvailable)
                    .foregroundColor(.secondary)
            } else {
                ScrollView(.vertical) {
                    LazyVStack(alignment: .leading, spacing: 2) {
                        ForEach(Array(logs.enumerated()), id: \.offset) { _, line in
                            Text(Self.displayLogLine(line))
                                .font(.system(size: 11, design: .monospaced))
                                .lineLimit(1)
                                .truncationMode(.middle)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(10)
                }
                .frame(maxHeight: .infinity)
                .glassPanel()
            }
        }
        .padding()
    }

    // MARK: - Data Loading

    private var apiClient: ManagementAPIClient? {
        guard let url = dependencies.runtimeManager.managementURL,
              let token = dependencies.runtimeManager.managementToken,
              !url.isEmpty, !token.isEmpty else {
            return nil
        }
        return try? ManagementAPIClient(baseURL: url, token: token)
    }

    private func configureBackendClientIfAvailable() {
        if let client = apiClient {
            backendVM.configure(apiClient: client)
        }
    }

    private func loadDevices() async {
        guard let client = apiClient else {
            hasLoadedDevices = false
            return
        }
        do {
            devices = try await client.listDevices()
            hasLoadedDevices = true
        } catch {
            hasLoadedDevices = false
        }
    }

    private func loadLogs() async {
        guard !isLoadingLogs else { return }
        isLoadingLogs = true
        defer { isLoadingLogs = false }

        let logPath = dependencies.runtimeManager.config.logFilePath
        logs = await Task.detached(priority: .utility) {
            Self.readTailLines(at: logPath, maxLines: 200, maxBytes: 1_048_576)
        }.value
    }

    private nonisolated static func readTailLines(at path: String, maxLines: Int, maxBytes: UInt64) -> [String] {
        guard let handle = try? FileHandle(forReadingFrom: URL(fileURLWithPath: path)) else {
            return []
        }
        defer { try? handle.close() }

        guard let fileSize = try? handle.seekToEnd() else {
            return []
        }
        let bytesToRead = min(fileSize, maxBytes)
        guard bytesToRead > 0 else {
            return []
        }

        do {
            try handle.seek(toOffset: fileSize - bytesToRead)
            guard let data = try handle.readToEnd(),
                  let text = String(data: data, encoding: .utf8) else {
                return []
            }
            let lines = text.split(separator: "\n", omittingEmptySubsequences: true).map(String.init)
            return Array(lines.suffix(maxLines))
        } catch {
            return []
        }
    }

    private static func displayLogLine(_ line: String) -> String {
        let maxCharacters = 500
        guard line.count > maxCharacters else { return line }
        return String(line.prefix(maxCharacters)) + " …"
    }

    private func revokeDevice(_ device: TrustedDevice) async {
        guard let client = apiClient else {
            errorMessage = L10n.errorCannotConnect
            showErrorAlert = true
            return
        }
        do {
            try await client.revokeDevice(device.deviceId)
            await loadDevices()
        } catch {
            errorMessage = String(format: L10n.errorRemoveDevice, error.localizedDescription)
            showErrorAlert = true
        }
    }
}
