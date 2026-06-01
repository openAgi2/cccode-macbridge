package gobridge

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// ─── Reconcile Presentation 状态管理 ─────────────────────────────────────
//
// 方案 §8.6 / §10.3 / §13.3：
//   仅当前可见 session 展示 syncing/原子更新；
//   后台 session 仅标记待同步。
//   失败状态不展示未经 Mac 权威确认的内容。
//   Completion notification 不重复。
//
// 此模块定义 Go 端的 presentation 状态模型。
// iOS Swift 端在 ViewModel 层消费这些状态。

const (
	presentationIdle     = "idle"
	presentationSyncing  = "syncing"  // 正在从 Mac 回源同步
	presentationComplete = "complete" // 同步完成，内容已原子替换
	presentationFailed   = "failed"   // 同步失败
	presentationOffline  = "offline"  // Mac 离线
)

// SessionPresentationState 单个 session 的展示状态。
type SessionPresentationState struct {
	SessionID         string    `json:"sessionId"`
	BackendID         string    `json:"backendId"`
	State             string    `json:"state"` // idle/syncing/complete/failed/offline
	LastVerifiedAt    time.Time `json:"lastVerifiedAt,omitempty"`
	LastError         string    `json:"lastError,omitempty"`
	PendingSync       bool      `json:"pendingSync"`       // 后台待同步标记
	PendingCompletion bool      `json:"pendingCompletion"` // completion notification 待去重
}

// PresentationManager 管理 session 展示状态。
type PresentationManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionPresentationState // sessionID -> state
}

// NewPresentationManager 创建展示状态管理器。
func NewPresentationManager() *PresentationManager {
	return &PresentationManager{
		sessions: make(map[string]*SessionPresentationState),
	}
}

// GetState 返回 session 的展示状态。
func (pm *PresentationManager) GetState(sessionID string) *SessionPresentationState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return nil
	}
	// 返回副本
	copy := *s
	return &copy
}

// MarkSyncing 标记可见 session 开始同步。
// 方案 §8.6：保留最后一次经 Mac 校验的内容并标记 Relay - Syncing。
func (pm *PresentationManager) MarkSyncing(sessionID, backendID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		s = &SessionPresentationState{
			SessionID: sessionID,
			BackendID: backendID,
		}
		pm.sessions[sessionID] = s
	}

	s.State = presentationSyncing
	s.LastError = ""

	slog.Debug("presentation: syncing", "sessionID", safeID(sessionID))
}

// MarkComplete 标记可见 session 同步完成（原子替换）。
// 方案 §8.6：新结果完成后原子替换局部状态。
func (pm *PresentationManager) MarkComplete(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return
	}

	s.State = presentationComplete
	s.LastVerifiedAt = time.Now()
	s.PendingSync = false

	slog.Debug("presentation: complete", "sessionID", safeID(sessionID))
}

// MarkFailed 标记同步失败。
func (pm *PresentationManager) MarkFailed(sessionID, errorMsg string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return
	}

	s.State = presentationFailed
	s.LastError = errorMsg

	slog.Debug("presentation: failed", "sessionID", safeID(sessionID), "error", errorMsg)
}

// MarkOffline 标记 Mac 离线。
// 方案 §13.3：Relay - Mac Offline 状态。
func (pm *PresentationManager) MarkOffline(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return
	}

	s.State = presentationOffline

	slog.Debug("presentation: offline", "sessionID", safeID(sessionID))
}

// MarkPendingSync 标记后台 session 待同步。
// 方案 §8.6：后台 session 仅标记待同步，不以空白闪烁或未经校验的正文暗示已经恢复。
func (pm *PresentationManager) MarkPendingSync(sessionID, backendID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		s = &SessionPresentationState{
			SessionID: sessionID,
			BackendID: backendID,
		}
		pm.sessions[sessionID] = s
	}

	s.PendingSync = true

	slog.Debug("presentation: pending sync", "sessionID", safeID(sessionID))
}

// MarkCompletionPending 标记 completion notification 待处理（去重）。
// 方案 §8.6：pending completion 去重。
func (pm *PresentationManager) MarkCompletionPending(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return
	}
	s.PendingCompletion = true
}

// ClearCompletionPending 清除 completion pending 标记。
func (pm *PresentationManager) ClearCompletionPending(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return
	}
	s.PendingCompletion = false
}

// IsCompletionPending 检查 completion 是否 pending。
func (pm *PresentationManager) IsCompletionPending(sessionID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	s, ok := pm.sessions[sessionID]
	if !ok {
		return false
	}
	return s.PendingCompletion
}

// GetAllPendingSync 返回所有待同步的 session。
func (pm *PresentationManager) GetAllPendingSync() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []string
	for _, s := range pm.sessions {
		if s.PendingSync {
			result = append(result, s.SessionID)
		}
	}
	return result
}

// RemoveSession 移除 session 状态。
func (pm *PresentationManager) RemoveSession(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.sessions, sessionID)
}

// ── Connection Mode 展示 ────────────────────────────────────────────────
//
// 方案 §13.3：UI 中应区分连接模式状态。

// ConnectionMode 连接模式枚举。
type ConnectionMode string

const (
	ConnectionModeDirectLocal  ConnectionMode = "direct_local"  // 局域网直连
	ConnectionModeDirectRemote ConnectionMode = "direct_remote" // Tailscale/WSS
	ConnectionModeRelay        ConnectionMode = "relay"         // E2E Relay
	ConnectionModeRelaySyncing ConnectionMode = "relay_syncing" // Relay 同步中
	ConnectionModeRelayOffline ConnectionMode = "relay_offline" // Mac 离线
	ConnectionModeDisconnected ConnectionMode = "disconnected"  // 未连接
)

// ConnectionDisplayInfo 连接状态展示信息。
type ConnectionDisplayInfo struct {
	Mode          ConnectionMode `json:"mode"`
	IsEncrypted   bool           `json:"isEncrypted"`
	RelayEndpoint string         `json:"relayEndpoint,omitempty"`
	LastError     string         `json:"lastError,omitempty"`
	ConnectedAt   time.Time      `json:"connectedAt,omitempty"`
}

// ConnectionModeDisplayText 返回用户可见的连接模式文案。
// 方案 §13.3。
func ConnectionModeDisplayText(mode ConnectionMode) string {
	switch mode {
	case ConnectionModeDirectLocal:
		return "Direct - Local"
	case ConnectionModeDirectRemote:
		return "Direct - Remote"
	case ConnectionModeRelay:
		return "Relay - Encrypted"
	case ConnectionModeRelaySyncing:
		return "Relay - Syncing"
	case ConnectionModeRelayOffline:
		return "Relay - Mac Offline"
	case ConnectionModeDisconnected:
		return "Disconnected"
	default:
		return "Unknown"
	}
}

// ── Wire RPC: set_observation_scope ──────────────────────────────────────
//
// 方案 §8.3 inner RPC。

// SetObservationScopeRequest 是 set_observation_scope 请求。
type SetObservationScopeRequest struct {
	BackendID             string   `json:"backendId"`
	SessionIDs            []string `json:"sessionIds"`
	DeliveryMode          string   `json:"deliveryMode"` // "full_stream" | "milestones_only"
	IncludeRunningSignals bool     `json:"includeRunningSessionSignals"`
	LeaseSeconds          int      `json:"leaseSeconds"`
}

// ParseSetObservationScopeRequest 从 wire message params 解析请求。
func ParseSetObservationScopeRequest(params json.RawMessage) (*SetObservationScopeRequest, error) {
	var req SetObservationScopeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.BackendID == "" {
		return nil, ErrInvalidRequest
	}
	if req.DeliveryMode != scopeFullStream && req.DeliveryMode != scopeMilestonesOnly {
		return nil, ErrInvalidRequest
	}
	return &req, nil
}

var ErrInvalidRequest = &DeliveryError{Code: "invalid_request", Message: "invalid request parameters"}
