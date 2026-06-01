import Foundation

// MARK: - 语言枚举

/// 支持的语言
enum AppLanguage: String, CaseIterable, Identifiable {
    case system = ""
    case en = "en"
    case zhHans = "zh-Hans"

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .system: return L10n.tr("system_default")
        case .en: return "English"
        case .zhHans: return "简体中文"
        }
    }
}

// MARK: - 外观枚举

/// 支持的窗口外观
enum AppTheme: String, CaseIterable, Identifiable {
    case system = ""
    case light = "light"
    case dark = "dark"

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .system: return L10n.tr("theme_system")
        case .light: return L10n.tr("theme_light")
        case .dark: return L10n.tr("theme_dark")
        }
    }
}

// MARK: - 本地化字符串查找

/// 统一本地化字符串入口
///
/// 使用方式：L10n.overview / L10n.pairDevice 等
/// 语言由 UserDefaults "appLanguage" 控制，默认跟随系统
enum L10n {
    static var current: AppLanguage {
        let raw = UserDefaults.standard.string(forKey: "appLanguage") ?? ""
        if raw.isEmpty {
            let preferred = Locale.preferredLanguages.first ?? "en"
            return preferred.hasPrefix("zh") ? .zhHans : .en
        }
        return AppLanguage(rawValue: raw) ?? .en
    }

    // 查表
    static func tr(_ key: String) -> String {
        table[current]?[key] ?? table[.en]?[key] ?? key
    }

    // MARK: - Tab 标题

    static var overview: String { tr("overview") }
    static var devices: String { tr("devices") }
    static var aiTools: String { tr("ai_tools") }
    static var settings: String { tr("settings") }
    static var diagnostics: String { tr("diagnostics") }
    static var remoteAccessTab: String { tr("remote_access_tab") }
    static var devicesPairing: String { tr("devices_pairing") }
    static var logsDiagnostics: String { tr("logs_diagnostics") }

    // MARK: - Overview

    static var ccCodeBridge: String { tr("cccode_bridge") }
    static var status: String { tr("status") }
    static var bridgeRunning: String { tr("bridge_running") }
    static var aiToolsReady: String { tr("ai_tools_ready") }
    static var trustedDevices: String { tr("trusted_devices") }
    static var noTrustedDevices: String { tr("no_trusted_devices") }
    static var loadingDevices: String { tr("loading_devices") }
    static var noAiToolsDetected: String { tr("no_ai_tools_detected") }
    static var remoteAccess: String { tr("remote_access") }
    static var remoteAccessHint: String { tr("remote_access_hint") }
    static var configured: String { tr("configured") }
    static var notConfigured: String { tr("not_configured") }
    static var edit: String { tr("edit") }
    static var connectionMode: String { tr("connection_mode") }
    static var localOnly: String { tr("local_only") }
    static var remoteConfigured: String { tr("remote_configured") }
    static var securityLevel: String { tr("security_level") }
    static var secEncrypted: String { tr("sec_encrypted") }
    static var secTailscaleTunnel: String { tr("sec_tailscale_tunnel") }
    static var secLan: String { tr("sec_lan") }
    static var secInsecure: String { tr("sec_insecure") }
    static var secUnknown: String { tr("sec_unknown") }
    static var insecureWarning: String { tr("insecure_warning") }
    static var insecureWarningDetail: String { tr("insecure_warning_detail") }
    static var tailscalePath: String { tr("tailscale_path") }
    static var tailscalePathHint: String { tr("tailscale_path_hint") }
    static var frpPath: String { tr("frp_path") }
    static var frpPathHint: String { tr("frp_path_hint") }
    static var remoteURLLabel: String { tr("remote_url_label") }
    static var localURLLabel: String { tr("local_url_label") }
    static var diagnosisInfo: String { tr("diagnosis_info") }
    static var diagnosisDisclaimer: String { tr("diagnosis_disclaimer") }
    static var checkStatus: String { tr("check_status") }
    static var saveAndRestart: String { tr("save_and_restart") }
    static var lastConnected: String { tr("last_connected") }

    // MARK: - Pairing

    static var pairNewDevice: String { tr("pair_new_device") }
    static var scanWithCCCode: String { tr("scan_with_cccode") }
    static var manualCode: String { tr("manual_code") }
    static var waitingForDevice: String { tr("waiting_for_device") }
    static var creatingPairingSession: String { tr("creating_pairing_session") }
    static var securityHint1: String { tr("security_hint_1") }
    static var securityHint2: String { tr("security_hint_2") }
    static var securityHint3: String { tr("security_hint_3") }
    static var bridgeLabel: String { tr("bridge_label") }
    static var deviceRequest: String { tr("device_request") }
    static var approve: String { tr("approve") }
    static var reject: String { tr("reject") }
    static var devicePairedSuccessfully: String { tr("device_paired_successfully") }
    static var pairAnotherDevice: String { tr("pair_another_device") }
    static var deviceRejected: String { tr("device_rejected") }
    static var pairingSessionExpired: String { tr("pairing_session_expired") }
    static var tryAgain: String { tr("try_again") }
    static var retry: String { tr("retry") }

    // MARK: - Devices

    static var authorizedDevices: String { tr("authorized_devices") }
    static var noAuthorizedDevices: String { tr("no_authorized_devices") }
    static var remove: String { tr("remove") }
    static var cancel: String { tr("cancel") }
    static var removeDeviceConfirm: String { tr("remove_device_confirm") }
    static var removeDeviceMessage: String { tr("remove_device_message") }
    static var paired: String { tr("paired") }
    static var lastSeen: String { tr("last_seen") }
    static var justNow: String { tr("just_now") }
    static var minAgo: String { tr("min_ago") }
    static var hrAgo: String { tr("hr_ago") }
    static var daysAgo: String { tr("days_ago") }

    // MARK: - AI Tools

    static var refreshAll: String { tr("refresh_all") }
    static var noAiToolsConfigured: String { tr("no_ai_tools_configured") }
    static var allUnavailableGuidance: String { tr("all_unavailable_guidance") }
    static var test: String { tr("test") }
    static var externalTurnsPolling: String { tr("external_turns_polling") }
    static var notInstalled: String { tr("not_installed") }
    static var serviceNotRunning: String { tr("service_not_running") }
    static var loginRequired: String { tr("login_required") }
    static var detectionTimedOut: String { tr("detection_timed_out") }
    static var cannotReachService: String { tr("cannot_reach_service") }
    static var checkDocsGuidance: String { tr("check_docs_guidance") }

    // MARK: - AI Tools status

    static var statusReady: String { tr("status_ready") }
    static var statusNotFound: String { tr("status_not_found") }
    static var statusLoginRequired: String { tr("status_login_required") }
    static var statusNotRunning: String { tr("status_not_running") }
    static var statusPortConflict: String { tr("status_port_conflict") }
    static var statusVersionIncompatible: String { tr("status_version_incompatible") }
    static var statusPermissionDenied: String { tr("status_permission_denied") }

    // MARK: - Diagnostics

    static var rawLogs: String { tr("raw_logs") }
    static var last200Lines: String { tr("last_200_lines") }
    static var copyRawLogs: String { tr("copy_raw_logs") }
    static var noLogsAvailable: String { tr("no_logs_available") }

    // MARK: - Settings

    static var bridgeName: String { tr("bridge_name") }
    static var bridgeNameHint: String { tr("bridge_name_hint") }
    static var save: String { tr("save") }
    static var openCodeAuth: String { tr("opencode_auth") }
    static var openCodeAuthHint: String { tr("opencode_auth_hint") }
    static var openCodeAuthGuidanceAuto: String { tr("opencode_auth_guidance_auto") }
    static var openCodeAuthGuidanceManual: String { tr("opencode_auth_guidance_manual") }
    static var username: String { tr("username") }
    static var password: String { tr("password") }
    static var language: String { tr("language") }
    static var appearance: String { tr("appearance") }
    static var themeSystem: String { tr("theme_system") }
    static var themeLight: String { tr("theme_light") }
    static var themeDark: String { tr("theme_dark") }

    // MARK: - Settings messages

    static var nameUpdated: String { tr("name_updated") }
    static var saveFailed: String { tr("save_failed") }
    static var saveFailedHttp: String { tr("save_failed_http") }
    static var savedRestarting: String { tr("saved_restarting") }
    static var failedLoadAgents: String { tr("failed_load_agents") }
    static var failedRefreshAgents: String { tr("failed_refresh_agents") }
    static var failedTestAgent: String { tr("failed_test_agent") }
    static var showingLastAgentResults: String { tr("showing_last_agent_results") }
    static var errorCannotConnect: String { tr("error_cannot_connect") }
    static var errorRemoveDevice: String { tr("error_remove_device") }
    static var error: String { tr("error") }
    static var unknownError: String { tr("unknown_error") }
    static var ok: String { tr("ok") }

    // MARK: - MenuBar

    static var restartBridge: String { tr("restart_bridge") }
    static var stopBridge: String { tr("stop_bridge") }
    static var startBridge: String { tr("start_bridge") }
    static var openBridge: String { tr("open_bridge") }
    static var quit: String { tr("quit") }
    static var start: String { tr("start") }
    static var stop: String { tr("stop") }
    static var restart: String { tr("restart") }

    // MARK: - 翻译表

    private static let table: [AppLanguage: [String: String]] = [
        .en: [
            "overview": "Overview",
            "system_default": "System Default",
            "devices": "Devices",
            "ai_tools": "AI Tools",
            "settings": "Settings",
            "diagnostics": "Diagnostics",
            "cccode_bridge": "CCCode Bridge",
            "status": "Status",
            "bridge_running": "Bridge running",
            "ai_tools_ready": "%d AI tool(s) ready",
            "trusted_devices": "%d trusted device(s)",
            "no_trusted_devices": "No trusted devices",
            "loading_devices": "Loading devices...",
            "no_ai_tools_detected": "No AI tools detected",
            "remote_access": "Remote Access",
            "remote_access_hint": "Optional. Allows iPhone to connect via a public URL.",
            "remote_access_tab": "Remote Access",
            "devices_pairing": "Devices & Pairing",
            "logs_diagnostics": "Logs & Diagnostics",
            "configured": "Configured",
            "not_configured": "Not configured",
            "edit": "Edit",
            "connection_mode": "Connection Mode",
            "local_only": "Local only",
            "remote_configured": "Remote configured",
            "security_level": "Security",
            "sec_encrypted": "WSS encrypted",
            "sec_tailscale_tunnel": "Tailscale tunnel (WireGuard)",
            "sec_lan": "LAN",
            "sec_insecure": "Insecure (public ws://)",
            "sec_unknown": "Unknown",
            "insecure_warning": "⚠️ Insecure connection",
            "insecure_warning_detail": "This URL uses ws:// over a public address. Data is transmitted unencrypted. Use wss:// or Tailscale instead.",
            "tailscale_path": "Tailscale (Recommended)",
            "tailscale_path_hint": "Install Tailscale on both Mac and iPhone. Use the Tailscale IP (100.x.x.x) as the Remote URL.",
            "frp_path": "FRP / VPS / Reverse Proxy",
            "frp_path_hint": "Set up a reverse proxy with TLS termination and use the wss:// URL as the Remote URL.",
            "remote_url_label": "Remote URL",
            "local_url_label": "Local URL",
            "diagnosis_info": "Remote URL Diagnosis",
            "diagnosis_disclaimer": "Diagnosis shows local configuration state, not external reachability.",
            "check_status": "Check Status",
            "configuration_paths": "Configuration Paths",
            "save_and_restart": "Save & Restart",
            "last_connected": "— last connected %@",
            "pair_new_device": "Pair New Device",
            "scan_with_cccode": "Scan with CCCode on iPhone",
            "manual_code": "Manual code:",
            "waiting_for_device": "Waiting for device...",
            "creating_pairing_session": "Creating pairing session...",
            "security_hint_1": "Only approve devices you recognize.",
            "security_hint_2": "This Mac will ask before granting access.",
            "security_hint_3": "Pairing codes expire after a few minutes.",
            "bridge_label": "Bridge: %@",
            "device_request": "Device Request",
            "approve": "Approve",
            "reject": "Reject",
            "device_paired_successfully": "Device paired successfully",
            "pair_another_device": "Pair Another Device",
            "device_rejected": "Device rejected",
            "pairing_session_expired": "Pairing session expired",
            "try_again": "Try Again",
            "retry": "Retry",
            "authorized_devices": "Authorized Devices",
            "no_authorized_devices": "No authorized devices. Use the pairing section above to add one.",
            "remove": "Remove",
            "cancel": "Cancel",
            "remove_device_confirm": "Remove %@?",
            "remove_device_message": "This device will need to be paired again to access this Mac.",
            "paired": "paired %@",
            "last_seen": "last seen %@",
            "just_now": "just now",
            "min_ago": "%d min ago",
            "hr_ago": "%d hr ago",
            "days_ago": "%d days ago",
            "refresh_all": "Refresh All",
            "no_ai_tools_configured": "No AI tools configured",
            "all_unavailable_guidance": "CCCode Bridge is ready. Install or log in to an AI coding tool to get started.",
            "test": "Test",
            "external_turns_polling": "External turns are refreshed by polling",
            "not_installed": "Not installed. Install %@ to enable this tool.",
            "service_not_running": "Service is not running. Start it to enable detection.",
            "login_required": "Login required. Run the tool's login command first.",
            "detection_timed_out": "Detection timed out. The service may not be responding.",
            "cannot_reach_service": "Cannot reach the service. Check your connection.",
            "check_docs_guidance": "Check the tool's documentation for setup instructions.",
            "status_ready": "Ready",
            "status_not_found": "Not Found",
            "status_login_required": "Login Required",
            "status_not_running": "Not Running",
            "status_port_conflict": "Port Conflict",
            "status_version_incompatible": "Version Incompatible",
            "status_permission_denied": "Permission Denied",
            "raw_logs": "Raw Logs",
            "last_200_lines": "Last 200 lines from bridge log",
            "copy_raw_logs": "Copy Raw Logs",
            "no_logs_available": "No logs available",
            "bridge_name": "Bridge Name",
            "bridge_name_hint": "The name other devices see when connecting. Changes take effect immediately.",
            "save": "Save",
            "opencode_auth": "OpenCode Authentication",
            "opencode_auth_hint": "OpenCode HTTP service requires Basic Auth. After saving, Bridge will restart with the new credentials.",
            "opencode_auth_guidance_auto": "• Auto-Pairing: MacBridge automatically generates random credentials and writes them to OpenCode Desktop configuration folder. No manual setup is needed.",
            "opencode_auth_guidance_manual": "• Manual/CLI: If running OpenCode via command line, start it using:",
            "username": "Username",
            "password": "Password",
            "language": "Language",
            "appearance": "Appearance",
            "theme_system": "Follow System",
            "theme_light": "Light",
            "theme_dark": "Dark",
            "name_updated": "Name updated",
            "save_failed": "Save failed: %@",
            "save_failed_http": "Save failed (HTTP %d)",
            "saved_restarting": "Saved. Restarting Bridge...",
            "failed_load_agents": "Failed to load agent status: %@",
            "failed_refresh_agents": "Failed to refresh agent status: %@",
            "failed_test_agent": "Failed to test agent: %@",
            "showing_last_agent_results": "Refresh failed: %@. Showing last known results.",
            "error_cannot_connect": "Cannot connect to CCCode Bridge. Make sure Bridge is running.",
            "error_remove_device": "Failed to remove device: %@",
            "error": "Error",
            "unknown_error": "Unknown error",
            "ok": "OK",
            "restart_bridge": "Restart CCCode Bridge",
            "stop_bridge": "Stop CCCode Bridge",
            "start_bridge": "Start CCCode Bridge",
            "open_bridge": "Open CCCode Bridge",
            "quit": "Quit",
            "start": "Start",
            "stop": "Stop",
            "restart": "Restart",
        ],
        .zhHans: [
            "overview": "总览",
            "system_default": "跟随系统",
            "devices": "设备",
            "ai_tools": "AI 工具",
            "settings": "设置",
            "diagnostics": "诊断",
            "cccode_bridge": "CCCode Bridge",
            "status": "状态",
            "bridge_running": "Bridge 运行中",
            "ai_tools_ready": "%d 个 AI 工具就绪",
            "trusted_devices": "%d 个已授权设备",
            "no_trusted_devices": "暂无已授权设备",
            "loading_devices": "正在加载设备…",
            "no_ai_tools_detected": "未检测到 AI 工具",
            "remote_access": "远程访问",
            "remote_access_hint": "可选。允许 iPhone 通过公网地址连接。",
            "remote_access_tab": "远程访问",
            "devices_pairing": "设备与配对",
            "logs_diagnostics": "日志与诊断",
            "configured": "已配置",
            "not_configured": "未配置",
            "edit": "编辑",
            "connection_mode": "连接模式",
            "local_only": "仅局域网",
            "remote_configured": "远程已配置",
            "security_level": "安全性",
            "sec_encrypted": "WSS 加密",
            "sec_tailscale_tunnel": "Tailscale 隧道 (WireGuard)",
            "sec_lan": "局域网",
            "sec_insecure": "不安全（公网 ws://）",
            "sec_unknown": "未知",
            "insecure_warning": "⚠️ 不安全连接",
            "insecure_warning_detail": "此 URL 使用公网 ws://，数据未加密传输。请使用 wss:// 或 Tailscale。",
            "tailscale_path": "Tailscale（推荐）",
            "tailscale_path_hint": "在 Mac 和 iPhone 上安装 Tailscale，使用 Tailscale IP (100.x.x.x) 作为远程 URL。",
            "frp_path": "FRP / VPS / 反向代理",
            "frp_path_hint": "配置带 TLS 终结的反向代理，使用 wss:// URL 作为远程 URL。",
            "remote_url_label": "远程 URL",
            "local_url_label": "本地 URL",
            "diagnosis_info": "远程 URL 诊断",
            "diagnosis_disclaimer": "诊断仅显示本机配置状态，不代表外部可达性。",
            "check_status": "检查状态",
            "configuration_paths": "配置路径",
            "save_and_restart": "保存并重启",
            "last_connected": "— 上次连接 %@",
            "pair_new_device": "配对新设备",
            "scan_with_cccode": "使用 iPhone 上的 CCCode 扫码",
            "manual_code": "手动码：",
            "waiting_for_device": "等待设备连接…",
            "creating_pairing_session": "正在创建配对会话…",
            "security_hint_1": "仅批准你识别的设备。",
            "security_hint_2": "此 Mac 会在授权前请求确认。",
            "security_hint_3": "配对码几分钟后会过期。",
            "bridge_label": "Bridge：%@",
            "device_request": "设备请求",
            "approve": "批准",
            "reject": "拒绝",
            "device_paired_successfully": "设备配对成功",
            "pair_another_device": "配对另一个设备",
            "device_rejected": "已拒绝设备",
            "pairing_session_expired": "配对会话已过期",
            "try_again": "重试",
            "retry": "重试",
            "authorized_devices": "已授权设备",
            "no_authorized_devices": "暂无已授权设备。使用上方的配对区域添加设备。",
            "remove": "移除",
            "cancel": "取消",
            "remove_device_confirm": "移除 %@？",
            "remove_device_message": "此设备需要重新配对才能访问此 Mac。",
            "paired": "配对于 %@",
            "last_seen": "最近连接 %@",
            "just_now": "刚刚",
            "min_ago": "%d 分钟前",
            "hr_ago": "%d 小时前",
            "days_ago": "%d 天前",
            "refresh_all": "全部刷新",
            "no_ai_tools_configured": "未配置 AI 工具",
            "all_unavailable_guidance": "CCCode Bridge 已就绪。安装或登录 AI 编程工具以开始使用。",
            "test": "测试",
            "external_turns_polling": "外部会话通过轮询刷新",
            "not_installed": "未安装。请安装 %@ 以启用此工具。",
            "service_not_running": "服务未运行。请先启动服务。",
            "login_required": "需要登录。请先运行工具的登录命令。",
            "detection_timed_out": "检测超时。服务可能未响应。",
            "cannot_reach_service": "无法连接服务。请检查网络。",
            "check_docs_guidance": "请查看工具文档了解安装方法。",
            "status_ready": "就绪",
            "status_not_found": "未找到",
            "status_login_required": "需要登录",
            "status_not_running": "未运行",
            "status_port_conflict": "端口冲突",
            "status_version_incompatible": "版本不兼容",
            "status_permission_denied": "权限被拒",
            "raw_logs": "原始日志",
            "last_200_lines": "最近 200 行 Bridge 日志",
            "copy_raw_logs": "复制原始日志",
            "no_logs_available": "暂无日志",
            "bridge_name": "Bridge 名称",
            "bridge_name_hint": "其他设备连接时看到的名称。修改后立即生效。",
            "save": "保存",
            "opencode_auth": "OpenCode 认证",
            "opencode_auth_hint": "OpenCode HTTP 服务需要 Basic Auth 认证。保存后 Bridge 会自动重启并携带新凭据。",
            "opencode_auth_guidance_auto": "• 自动配对：MacBridge 会自动生成随机凭据并将其写入 OpenCode 桌面版配置，无需手动设置即可自动连接。",
            "opencode_auth_guidance_manual": "• 命令行/手动：若使用终端启动 OpenCode，请使用以下命令，并在下方配置对应账密：",
            "username": "用户名",
            "password": "密码",
            "language": "语言",
            "appearance": "外观",
            "theme_system": "跟随系统",
            "theme_light": "白天",
            "theme_dark": "黑夜",
            "name_updated": "名称已更新",
            "save_failed": "保存失败：%@",
            "save_failed_http": "保存失败 (HTTP %d)",
            "saved_restarting": "已保存，正在重启 Bridge…",
            "failed_load_agents": "加载 AI 工具状态失败：%@",
            "failed_refresh_agents": "刷新 AI 工具状态失败：%@",
            "failed_test_agent": "测试 AI 工具失败：%@",
            "showing_last_agent_results": "刷新失败：%@。正在显示上次结果。",
            "error_cannot_connect": "无法连接到 CCCode Bridge。请确认 Bridge 正在运行。",
            "error_remove_device": "移除设备失败：%@",
            "error": "错误",
            "unknown_error": "未知错误",
            "ok": "确定",
            "restart_bridge": "重启 CCCode Bridge",
            "stop_bridge": "停止 CCCode Bridge",
            "start_bridge": "启动 CCCode Bridge",
            "open_bridge": "打开 CCCode Bridge",
            "quit": "退出",
            "start": "启动",
            "stop": "停止",
            "restart": "重启",
        ],
    ]
}
