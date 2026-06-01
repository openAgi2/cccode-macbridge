import SwiftUI

// MARK: - 菜单栏下拉内容

/// 菜单栏图标点击后展示的下拉菜单
struct MenuBarMenu: View {
    @ObservedObject var viewModel: BridgeStatusViewModel
    let onStart: () -> Void
    let onStop: () -> Void
    let onRestart: () -> Void

    private var isRunning: Bool {
        viewModel.status == .ready || viewModel.status == .readyNoAgents
    }

    var body: some View {
        Text(viewModel.statusText)

        Divider()

        if isRunning {
            Button(L10n.restartBridge) { onRestart() }
            Button(L10n.stopBridge) { onStop() }
        } else {
            Button(L10n.startBridge) { onStart() }
        }

        Divider()

        Button(L10n.openBridge) {
            NSApp.activate(ignoringOtherApps: true)
            if let window = NSApp.windows.first(where: { $0.isVisible }) {
                window.makeKeyAndOrderFront(nil)
            }
        }

        Divider()

        Button(L10n.quit) { NSApp.terminate(nil) }
    }
}
