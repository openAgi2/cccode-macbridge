import Combine
import Foundation

/// 配对会话状态
enum PairingUIState: Equatable {
    case idle
    case creating
    case waitingForClaim(sessionId: String, manualCode: String, qrPayload: String)
    case claimed(deviceName: String, platform: String)
    case approved
    case rejected
    case expired
    case error(String)
}

@MainActor
class PairingViewModel: ObservableObject {
    @Published var uiState: PairingUIState = .idle
    @Published var qrPayload: String = ""
    @Published var manualCode: String = ""
    @Published var claimedDeviceName: String = ""
    @Published var claimedPlatform: String = ""

    private var apiClient: ManagementAPIClient?
    private var pollingTimer: Timer?
    private var currentSessionId: String?
    private var isStarting = false

    func configure(apiClient: ManagementAPIClient) {
        self.apiClient = apiClient
    }

    func startPairing() {
        guard !isStarting else { return }
        guard let client = apiClient else {
            uiState = .error("Management API not available")
            return
        }

        isStarting = true
        uiState = .creating

        Task {
            do {
                let session = try await client.createPairing()
                currentSessionId = session.id
                manualCode = session.manualCode
                qrPayload = session.qrPayload
                uiState = .waitingForClaim(
                    sessionId: session.id,
                    manualCode: session.manualCode,
                    qrPayload: session.qrPayload
                )
                startPolling()
            } catch {
                uiState = .error(error.localizedDescription)
            }
            isStarting = false
        }
    }

    func approve() {
        guard let client = apiClient, let sessionId = currentSessionId else { return }
        Task {
            do {
                _ = try await client.approvePairing(sessionId)
                uiState = .approved
                stopPolling()
            } catch {
                uiState = .error(error.localizedDescription)
            }
        }
    }

    func reject() {
        guard let client = apiClient, let sessionId = currentSessionId else { return }
        Task {
            do {
                try await client.rejectPairing(sessionId)
                uiState = .rejected
                stopPolling()
            } catch {
                uiState = .error(error.localizedDescription)
            }
        }
    }

    func reset() {
        stopPolling()
        currentSessionId = nil
        qrPayload = ""
        manualCode = ""
        claimedDeviceName = ""
        claimedPlatform = ""
        uiState = .idle
        isStarting = false
    }

    deinit {
        pollingTimer?.invalidate()
    }

    // MARK: - Polling

    private func startPolling() {
        stopPolling()
        // Relay pairing status 会触发公网 claim 查询，低于生产限流窗口轮询。
        pollingTimer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in
            Task { @MainActor in
                await self?.pollStatus()
            }
        }
    }

    private func stopPolling() {
        pollingTimer?.invalidate()
        pollingTimer = nil
    }

    private func pollStatus() async {
        guard let client = apiClient, let sessionId = currentSessionId else { return }
        do {
            let status = try await client.getPairingStatus(sessionId)
            switch status.state {
            case "claimed":
                claimedDeviceName = status.claimingDeviceName ?? "Unknown Device"
                claimedPlatform = status.claimingPlatform ?? ""
                uiState = .claimed(deviceName: claimedDeviceName, platform: claimedPlatform)
                stopPolling()
            case "approved":
                uiState = .approved
                stopPolling()
            case "rejected":
                uiState = .rejected
                stopPolling()
            case "expired":
                uiState = .expired
                stopPolling()
            default:
                break
            }
        } catch {
            // polling error — keep current state, will retry
        }
    }
}
