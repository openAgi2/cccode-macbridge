import Combine
import Foundation

/// Bridge 状态 ViewModel，绑定到 RuntimeManager
@MainActor
class BridgeStatusViewModel: ObservableObject {
    @Published var status: BridgeStatus = .starting
    @Published var statusText: String = "Starting..."
    @Published var agents: [AgentInfo] = []
    @Published var lastError: String?

    /// 绑定到 RuntimeManager 的 @Published 属性
    var runtimeManager: RuntimeManager? {
        didSet {
            cancellables.removeAll()
            bindToManager()
        }
    }

    private var cancellables = Set<AnyCancellable>()

    private func bindToManager() {
        guard let manager = runtimeManager else { return }

        // 立即同步当前值（避免 Combine publisher 只传变化不传初始值）
        status = manager.status
        statusText = manager.statusText
        agents = manager.agents
        lastError = manager.lastError

        manager.$status
            .receive(on: DispatchQueue.main)
            .assign(to: &$status)
        manager.$statusText
            .receive(on: DispatchQueue.main)
            .assign(to: &$statusText)
        manager.$agents
            .receive(on: DispatchQueue.main)
            .assign(to: &$agents)
        manager.$lastError
            .receive(on: DispatchQueue.main)
            .assign(to: &$lastError)
    }
}
