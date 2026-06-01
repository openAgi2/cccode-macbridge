import Combine
import Foundation

// MARK: - DI 容器

/// 全局依赖注入容器，创建并绑定 RuntimeManager ↔ ViewModels
@MainActor
class AppDependencies: ObservableObject {
    let runtimeManager: RuntimeManager
    let statusViewModel: BridgeStatusViewModel
    let pairingViewModel: PairingViewModel
    let settingsViewModel: SettingsViewModel

    private let dataDir: String

    init() {
        // 从 Bundle 获取 runtime binary 路径，回退到 /usr/local/bin
        let executablePath = Bundle.main.url(forResource: "cccode-bridge-runtime", withExtension: nil)?.path
            ?? "/usr/local/bin/cccode-bridge-runtime"

        let dir = NSSearchPathForDirectoriesInDomains(.applicationSupportDirectory, .userDomainMask, true).first!
            + "/CCCode Bridge"
        self.dataDir = dir
        let logDir = dir + "/logs"

        // OpenCode 凭据：环境变量 → credentials.json 降级
        var opencodeUser = ""
        var opencodePass = ""
        if let envUser = ProcessInfo.processInfo.environment["OPENCODE_SERVER_USERNAME"],
           !envUser.isEmpty {
            opencodeUser = envUser
        } else {
            opencodeUser = Self.readCredential("opencode_user", from: dir) ?? ""
        }
        if let envPass = ProcessInfo.processInfo.environment["OPENCODE_SERVER_PASSWORD"],
           !envPass.isEmpty {
            opencodePass = envPass
        } else {
            opencodePass = Self.readCredential("opencode_pass", from: dir) ?? ""
        }

        // 首次运行或凭据为空时，自动生成随机凭据并保存
        if opencodeUser.isEmpty || opencodePass.isEmpty {
            opencodeUser = "opencode"
            opencodePass = UUID().uuidString.lowercased()
            Self.writeCredentials(user: opencodeUser, pass: opencodePass, to: dir)
            NSLog("[AppDependencies] Automatically generated OpenCode credentials for first-time launch.")
        }

        let relayEndpoint = OfficialRelayConfiguration.isAvailable
            ? UserDefaults.standard.string(forKey: "relayEndpoint") ?? ""
            : ""
        let relayRouteID = OfficialRelayConfiguration.isAvailable
            ? UserDefaults.standard.string(forKey: "relayRouteID") ?? ""
            : ""
        let relayCredential = OfficialRelayConfiguration.isAvailable
            ? RelayRouteCredentialStore.load()
            : ""

        let config = RuntimeConfig(
            executablePath: executablePath,
            dataDir: dir,
            logDir: logDir,
            workDir: FileManager.default.homeDirectoryForCurrentUser.path,
            codexBackend: "app_server",
            codexAppServerURL: "ws://127.0.0.1:4141",
            opencodeUser: opencodeUser,
            opencodePass: opencodePass,
            remoteURL: UserDefaults.standard.string(forKey: "remoteBridgeURL") ?? "",
            includeTailscaleInPairing: UserDefaults.standard.object(forKey: "pairingIncludeTailscale") as? Bool ?? true,
            includeRemoteInPairing: UserDefaults.standard.object(forKey: "pairingIncludeRemote") as? Bool ?? true,
            relayEndpoint: relayEndpoint,
            relayRouteID: relayRouteID,
            relayCredential: relayCredential,
            relayServiceAddress: UserDefaults.standard.string(forKey: "relayServiceAddress") ?? ""
        )

        self.runtimeManager = RuntimeManager(config: config)
        self.statusViewModel = BridgeStatusViewModel()
        self.statusViewModel.runtimeManager = runtimeManager
        self.pairingViewModel = PairingViewModel()
        // SettingsViewModel 的 onCredentialsChanged 在 didLoad 中绑定，避免 init 阶段捕获 self
        self.settingsViewModel = SettingsViewModel(dataDir: dir, onCredentialsChanged: {})

        // 延迟绑定凭据变更回调（self 已完成初始化）
        self.settingsViewModel.onCredentialsChanged = { [weak self] in
            self?.handleCredentialsChanged()
        }

        // management 端点变更后自动刷新 Pairing API client，支持 launchctl restart 后重新附着
        Publishers.CombineLatest(runtimeManager.$managementURL, runtimeManager.$managementToken)
            .receive(on: DispatchQueue.main)
            .compactMap { url, token -> ManagementAPIClient? in
                guard let url, let token else { return nil }
                return try? ManagementAPIClient(baseURL: url, token: token)
            }
            .sink { [weak self] client in
                self?.pairingViewModel.configure(apiClient: client)
            }
            .store(in: &cancellables)
    }

    private var cancellables = Set<AnyCancellable>()

    /// 延迟启动 bridge，给 SwiftUI 足够的时间完成 UI 初始化
    func startBridge() {
        // 监听远程 URL 变更，更新 RuntimeConfig 并重启
        NotificationCenter.default.publisher(for: .remoteURLDidChange)
            .receive(on: DispatchQueue.main)
            .sink { [weak self] _ in
                self?.handleRemoteURLChange()
            }
            .store(in: &cancellables)

        runtimeManager.start()
        if OfficialRelayConfiguration.isAvailable {
            Task { [weak self] in
                do {
                    let relay = try await OfficialRelayProvisioner.shared.ensureRoute()
                    guard let self else { return }
                    guard self.runtimeManager.config.relayEndpoint != OfficialRelayConfiguration.endpoint ||
                            self.runtimeManager.config.relayRouteID != relay.routeID ||
                            self.runtimeManager.config.relayCredential != relay.credential else {
                        return
                    }
                    self.runtimeManager.config.relayEndpoint = OfficialRelayConfiguration.endpoint
                    self.runtimeManager.config.relayRouteID = relay.routeID
                    self.runtimeManager.config.relayCredential = relay.credential
                    self.runtimeManager.restart()
                } catch {
                    NSLog("[AppDependencies] 官方 Relay 自动启用失败: \(error.localizedDescription)")
                }
            }
        }
    }

    /// 远程 URL 变更回调：从 UserDefaults 读取最新 remoteURL，更新配置并重启
    private func handleRemoteURLChange() {
        let remoteURL = UserDefaults.standard.string(forKey: "remoteBridgeURL") ?? ""
        runtimeManager.config.remoteURL = remoteURL
        runtimeManager.config.includeTailscaleInPairing = UserDefaults.standard.object(forKey: "pairingIncludeTailscale") as? Bool ?? true
        runtimeManager.config.includeRemoteInPairing = UserDefaults.standard.object(forKey: "pairingIncludeRemote") as? Bool ?? true
        runtimeManager.config.relayEndpoint = OfficialRelayConfiguration.isAvailable
            ? UserDefaults.standard.string(forKey: "relayEndpoint") ?? ""
            : ""
        runtimeManager.config.relayRouteID = OfficialRelayConfiguration.isAvailable
            ? UserDefaults.standard.string(forKey: "relayRouteID") ?? ""
            : ""
        runtimeManager.config.relayCredential = OfficialRelayConfiguration.isAvailable
            ? RelayRouteCredentialStore.load()
            : ""
        runtimeManager.config.relayServiceAddress = UserDefaults.standard.string(forKey: "relayServiceAddress") ?? ""
        runtimeManager.restart()
    }

    /// 凭据变更回调：重新读取 credentials.json，构造新 RuntimeConfig，重启 Bridge
    private func handleCredentialsChanged() {
        let opencodeUser = Self.readCredential("opencode_user", from: dataDir)
            ?? ProcessInfo.processInfo.environment["OPENCODE_SERVER_USERNAME"]
            ?? ""
        let opencodePass = Self.readCredential("opencode_pass", from: dataDir)
            ?? ProcessInfo.processInfo.environment["OPENCODE_SERVER_PASSWORD"]
            ?? ""

        runtimeManager.updateOpenCodeCredentials(user: opencodeUser, pass: opencodePass)
        runtimeManager.restart()
    }

    /// 从 dataDir/credentials.json 读取持久化凭据。
    /// credentials.json 格式：{ "opencode_user": "...", "opencode_pass": "..." }
    static func readCredential(_ key: String, from dataDir: String) -> String? {
        let path = dataDir + "/credentials.json"
        guard let data = FileManager.default.contents(atPath: path),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return nil
        }
        return json[key] as? String
    }

    /// 将 OpenCode 凭据持久化写入 dataDir/credentials.json
    static func writeCredentials(user: String, pass: String, to dataDir: String) {
        let path = dataDir + "/credentials.json"
        let dict = [
            "opencode_user": user,
            "opencode_pass": pass
        ]
        do {
            try FileManager.default.createDirectory(atPath: dataDir, withIntermediateDirectories: true)
            let data = try JSONSerialization.data(withJSONObject: dict, options: [.prettyPrinted, .sortedKeys])
            try data.write(to: URL(fileURLWithPath: path), options: .atomic)
            try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: path)
        } catch {
            NSLog("[AppDependencies] Failed to write credentials: \(error.localizedDescription)")
        }
    }
}
