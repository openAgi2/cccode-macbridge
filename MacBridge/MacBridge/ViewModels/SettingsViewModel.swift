import Combine
import Foundation

/// 设置页面 ViewModel，管理 OpenCode 认证凭据的读写和 Bridge 重启
@MainActor
class SettingsViewModel: ObservableObject {
    @Published var opencodeUser: String = ""
    @Published var opencodePass: String = ""
    @Published var isSaving: Bool = false
    @Published var saveMessage: String? = nil
    @Published var displayName: String = ""
    @Published var displayNameMessage: String? = nil

    private var dataDir: String
    var onCredentialsChanged: (() -> Void)
    var managementAPIClient: ManagementAPIClient?

    init(dataDir: String, onCredentialsChanged: @escaping () -> Void) {
        self.dataDir = dataDir
        self.onCredentialsChanged = onCredentialsChanged
        loadCredentials()
        loadDisplayName()
    }

    // MARK: - 读取凭据

    /// 启动时从 credentials.json 加载
    func loadCredentials() {
        let path = dataDir + "/credentials.json"
        guard let data = FileManager.default.contents(atPath: path),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return
        }
        opencodeUser = json["opencode_user"] as? String ?? ""
        opencodePass = json["opencode_pass"] as? String ?? ""
    }

    // MARK: - 保存凭据并重启 Bridge

    func saveCredentials() {
        isSaving = true
        saveMessage = nil

        // 确保目录存在
        try? FileManager.default.createDirectory(atPath: dataDir, withIntermediateDirectories: true)

        let dict: [String: String] = [
            "opencode_user": opencodeUser,
            "opencode_pass": opencodePass,
        ]

        do {
            let data = try JSONSerialization.data(withJSONObject: dict, options: [.prettyPrinted, .sortedKeys])
            try data.write(to: URL(fileURLWithPath: dataDir + "/credentials.json"), options: .atomic)
            // 限制文件权限，仅 owner 可读写
            try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: dataDir + "/credentials.json")
        } catch {
            saveMessage = String(format: L10n.saveFailed, error.localizedDescription)
            isSaving = false
            return
        }

        // 通知 AppDependencies 更新并重启 Bridge
        onCredentialsChanged()

        saveMessage = L10n.savedRestarting
        isSaving = false
    }

    // MARK: - Display Name

    func loadDisplayName() {
        guard let client = managementAPIClient else { return }
        Task {
            do {
                let status = try await client.getStatus()
                if let name = status.displayName {
                    self.displayName = name
                }
            } catch {
                // 静默处理，display name 不是关键路径
            }
        }
    }

    func saveDisplayName() {
        let trimmed = displayName.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, let client = managementAPIClient else { return }
        displayNameMessage = nil
        Task {
            do {
                var req = URLRequest(url: client.baseURL.appendingPathComponent("/internal/settings/display-name"))
                req.httpMethod = "PUT"
                req.setValue("Bearer \(client.token)", forHTTPHeaderField: "Authorization")
                req.setValue("application/json", forHTTPHeaderField: "Content-Type")
                let body = ["displayName": trimmed]
                req.httpBody = try? JSONSerialization.data(withJSONObject: body)
                let (_, response) = try await URLSession.shared.data(for: req)
                let code = (response as? HTTPURLResponse)?.statusCode ?? -1
                if (200...299).contains(code) {
                    self.displayNameMessage = L10n.nameUpdated
                } else {
                    self.displayNameMessage = String(format: L10n.saveFailedHttp, code)
                }
            } catch {
                self.displayNameMessage = String(format: L10n.saveFailed, error.localizedDescription)
            }
        }
    }
}
