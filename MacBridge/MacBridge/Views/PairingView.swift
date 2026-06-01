import CoreImage.CIFilterBuiltins
import SwiftUI

struct PairingView: View {
    @ObservedObject var viewModel: PairingViewModel
    @AppStorage("bridgeDisplayName") private var bridgeDisplayName: String = ""

    private struct PairingCandidate: Identifiable, Equatable {
        let id: String
        let title: String
        let status: String
        let url: String
        let icon: String
        let color: Color
    }

    var body: some View {
        VStack(alignment: .center, spacing: 16) {
            switch viewModel.uiState {
            case .idle:
                Button(L10n.pairNewDevice) {
                    viewModel.startPairing()
                }
                .buttonStyle(.borderedProminent)

            case .creating:
                ProgressView(L10n.creatingPairingSession)

            case .waitingForClaim(_, let code, let payload):
                qrAndManualCode(code: code, payload: payload)

            case .claimed(let deviceName, let platform):
                claimedDeviceSection(deviceName: deviceName, platform: platform)

            case .approved:
                approvedSection

            case .rejected:
                rejectedSection

            case .expired:
                expiredSection

            case .error(let message):
                errorSection(message: message)
            }
        }
        .padding()
        .frame(maxWidth: 640)
    }

    // MARK: - QR + Manual Code

    @ViewBuilder
    private func qrAndManualCode(code: String, payload: String) -> some View {
        let candidates = pairingCandidates(from: payload)

        HStack(alignment: .center, spacing: 28) {
            qrImage(payload: payload)
                .interpolation(.none)
                .resizable()
                .frame(width: 180, height: 180)

            VStack(alignment: .leading, spacing: 14) {
                Text(L10n.scanWithCCCode)
                    .font(.headline)

                HStack(spacing: 4) {
                    Text(L10n.manualCode)
                        .font(.caption)
                        .foregroundColor(.secondary)
                    Text(code)
                        .font(.system(.title3, design: .monospaced))
                        .fontWeight(.bold)
                        .textSelection(.enabled)
                }

                pairingCandidatesSection(candidates)

                Text("手机扫码后先尝试局域网；如果不可用，会同时尝试二维码里的远程方式，谁先连通用谁。")
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)

                Label(L10n.securityHint2, systemImage: "hand.raised")
                    .font(.caption)
                    .foregroundColor(.secondary)

                if !bridgeDisplayName.isEmpty {
                    Text(String(format: L10n.bridgeLabel, bridgeDisplayName))
                        .font(.caption2)
                        .foregroundColor(.secondary)
                }

                ProgressView(L10n.waitingForDevice)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
            .frame(maxWidth: 300, alignment: .leading)
        }
    }

    @ViewBuilder
    private func pairingCandidatesSection(_ candidates: [PairingCandidate]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("此二维码包含")
                .font(.system(size: 13, weight: .semibold))

            ForEach(candidates) { candidate in
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: candidate.icon)
                        .foregroundColor(candidate.color)
                        .frame(width: 16)
                        .padding(.top, 1)

                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 6) {
                            Text(candidate.title)
                                .font(.caption)
                                .fontWeight(.medium)
                            Text(candidate.status)
                                .font(.caption2)
                                .foregroundColor(.secondary)
                        }
                        Text(candidate.url)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                            .textSelection(.enabled)
                    }
                }
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(NSColor.controlBackgroundColor).opacity(0.35))
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }

    // MARK: - Claimed (approve/reject)

    @ViewBuilder
    private func claimedDeviceSection(deviceName: String, platform: String) -> some View {
        Text(L10n.deviceRequest)
            .font(.headline)

        HStack(spacing: 8) {
            Image(systemName: "iphone")
                .font(.title2)
            VStack(alignment: .leading) {
                Text(deviceName)
                    .fontWeight(.medium)
                Text(platform)
                    .font(.caption)
                    .foregroundColor(.secondary)
            }
        }
        .padding()
        .glassPanel()

        HStack(spacing: 12) {
            Button(L10n.reject) {
                viewModel.reject()
            }
            .buttonStyle(.bordered)

            Button(L10n.approve) {
                viewModel.approve()
            }
            .buttonStyle(.borderedProminent)
        }
    }

    // MARK: - Result states

    @ViewBuilder
    private var approvedSection: some View {
        Image(systemName: "checkmark.circle.fill")
            .font(.largeTitle)
            .foregroundColor(.green)
        Text(L10n.devicePairedSuccessfully)
            .font(.headline)

        Button(L10n.pairAnotherDevice) {
            viewModel.reset()
        }
        .buttonStyle(.bordered)
    }

    @ViewBuilder
    private var rejectedSection: some View {
        Image(systemName: "xmark.circle.fill")
            .font(.largeTitle)
            .foregroundColor(.red)
        Text(L10n.deviceRejected)
            .font(.headline)

        Button(L10n.pairNewDevice) {
            viewModel.reset()
            viewModel.startPairing()
        }
        .buttonStyle(.bordered)
    }

    @ViewBuilder
    private var expiredSection: some View {
        Image(systemName: "clock")
            .font(.largeTitle)
            .foregroundColor(.orange)
        Text(L10n.pairingSessionExpired)
            .font(.headline)

        Button(L10n.tryAgain) {
            viewModel.reset()
            viewModel.startPairing()
        }
        .buttonStyle(.bordered)
    }

    @ViewBuilder
    private func errorSection(message: String) -> some View {
        Image(systemName: "exclamationmark.triangle")
            .font(.largeTitle)
            .foregroundColor(.red)
        Text(message)
            .font(.caption)
            .multilineTextAlignment(.center)

        Button(L10n.retry) {
            viewModel.reset()
        }
        .buttonStyle(.bordered)
    }

    // MARK: - QR Code Generation

    private func qrImage(payload: String) -> Image {
        let context = CIContext()
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(payload.utf8)
        filter.correctionLevel = "M"

        if let outputImage = filter.outputImage {
            let transform = CGAffineTransform(scaleX: 6, y: 6)
            let scaled = outputImage.transformed(by: transform)
            if let cgImage = context.createCGImage(scaled, from: scaled.extent) {
                return Image(nsImage: NSImage(cgImage: cgImage, size: NSSize(width: 180, height: 180)))
            }
        }
        return Image(systemName: "qrcode")
    }

    private func pairingCandidates(from payload: String) -> [PairingCandidate] {
        guard let components = URLComponents(string: payload) else { return [] }
        let queryItems = components.queryItems ?? []
        var result: [PairingCandidate] = []
        var seen = Set<String>()

        if let local = queryItems.first(where: { $0.name == "local" })?.value, !local.isEmpty {
            appendCandidate(
                &result,
                seen: &seen,
                title: "局域网",
                status: "优先尝试",
                url: local,
                icon: "wifi",
                color: .blue
            )
        }

        for remote in queryItems.filter({ $0.name == "remote" }).compactMap(\.value) where !remote.isEmpty {
            let type = remoteCandidateType(remote)
            appendCandidate(
                &result,
                seen: &seen,
                title: type.title,
                status: "远程兜底",
                url: remote,
                icon: type.icon,
                color: type.color
            )
        }

        if let relay = queryItems.first(where: { $0.name == "relay" })?.value, !relay.isEmpty {
            appendCandidate(
                &result,
                seen: &seen,
                title: "加密 Relay",
                status: "无需 FRP",
                url: relay,
                icon: "lock.shield",
                color: .green
            )
        }

        return result
    }

    private func appendCandidate(
        _ result: inout [PairingCandidate],
        seen: inout Set<String>,
        title: String,
        status: String,
        url: String,
        icon: String,
        color: Color
    ) {
        guard seen.insert(url).inserted else { return }
        result.append(PairingCandidate(
            id: url,
            title: title,
            status: status,
            url: url,
            icon: icon,
            color: color
        ))
    }

    private func remoteCandidateType(_ rawURL: String) -> (title: String, icon: String, color: Color) {
        guard let url = URL(string: rawURL), let host = url.host else {
            return ("VPS / FRP", "server.rack", .purple)
        }
        if isTailscaleHost(host) {
            return ("Tailscale", "network.badge.shields.half.filled", .cyan)
        }
        return ("VPS / FRP", "server.rack", .purple)
    }

    private func isTailscaleHost(_ host: String) -> Bool {
        let parts = host.split(separator: ".").compactMap { Int($0) }
        guard parts.count == 4 else { return false }
        return parts[0] == 100 && parts[1] >= 64 && parts[1] <= 127
    }
}

#Preview {
    let vm = PairingViewModel()
    vm.uiState = .waitingForClaim(
        sessionId: "pair_abc123",
        manualCode: "123456",
        qrPayload: "cccode://pair?id=pair_abc123&code=123456&local=ws://172.16.10.211:8777/pairing&remote=wss://100.79.255.127:8778/pairing&remote=wss://bridge.example.com/pairing"
    )
    return PairingView(viewModel: vm)
}
