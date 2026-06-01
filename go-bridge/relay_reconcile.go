package gobridge

import (
	"log/slog"
	"sync"
	"time"
)

// ─── iOS 端 Mailbox Replay 与权威 Reconcile ────────────────────────────
//
// 方案 §8.4 / §8.6 / §10.3：
//   iOS 端 durable cursor、localReconcileRequired、mailbox replay、
//   chain-head/gap 校验、lifecycle scope/prekey 补充及回源 Mac 状态收敛。
//
// 此模块在 Go 端定义数据结构和校验逻辑，iOS 端的实际持久化在 Swift 中实现。
// Go 端提供验证辅助函数，用于 handler 层和测试。

const (
	// durable apply 状态
	reconcileStateClean      = "clean"       // 无需 reconcile
	reconcileStateRequired   = "required"    // 需要从 Mac 回源
	reconcileStateInProgress = "in_progress" // 正在回源
	reconcileStateCompleted  = "completed"   // 回源完成
)

// DeliveryState 是 iOS 端的 delivery 状态快照。
// 对应 Swift 中的 UserDefaults / Keychain 持久化数据。
type DeliveryState struct {
	mu sync.Mutex

	// Durable cursor：上次成功 ack 的 cursor
	LastCommittedCursor uint64 `json:"lastCommittedCursor"`

	// Chain head 校验
	LastCommittedEpochDigest string `json:"lastCommittedEpochDigest"`
	LastCommittedCounter     uint64 `json:"lastCommittedCounter"`

	// Reconcile 状态
	ReconcileState string    `json:"reconcileState"` // clean/required/in_progress/completed
	ReconcileSince time.Time `json:"reconcileSince,omitempty"`

	// Per-session reconcile 标记
	SessionReconcileRequired map[string]bool `json:"sessionReconcileRequired,omitempty"` // sessionID -> required
}

// NewDeliveryState 创建初始 delivery 状态。
func NewDeliveryState() *DeliveryState {
	return &DeliveryState{
		ReconcileState:           reconcileStateClean,
		SessionReconcileRequired: make(map[string]bool),
	}
}

// ApplyFrame 尝试应用一个 mailbox frame 到 delivery state。
// 方案 §8.4 崩溃安全提交顺序：
//  1. 验证 counter 连续性
//  2. durable write
//  3. ack
//
// 返回 nil 表示成功，返回错误表示 frame 不应被应用。
func (ds *DeliveryState) ApplyFrame(counter uint64, epochDigest string, eventType string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// 如果正在 reconcile，不应用普通 frame
	if ds.ReconcileState == reconcileStateRequired {
		return ErrReconcileRequired
	}

	// 严格 counter 连续性检查
	expectedCounter := ds.LastCommittedCounter + 1
	if counter != expectedCounter {
		// counter 不连续 -> 触发 reconcile
		ds.ReconcileState = reconcileStateRequired
		ds.ReconcileSince = time.Now()
		slog.Warn("reconcile: counter gap detected",
			"expected", expectedCounter,
			"got", counter,
			"lastCommitted", ds.LastCommittedCounter,
		)
		return ErrCounterGap
	}

	// 更新状态
	ds.LastCommittedCounter = counter
	if epochDigest != "" {
		ds.LastCommittedEpochDigest = epochDigest
	}

	// 对于只表示"需要回源"的 milestone，标记 session reconcile
	if IsDurableMilestone(eventType) && eventType != "delivery_reconcile_required" {
		// milestone 已提交，但内容需要从 Mac 权威读取
		// 具体到哪个 session 由上层 frame 解析决定
	}

	return nil
}

// ApplyReconcileSignal 处理 delivery_reconcile_required 控制消息。
// 方案 §5.4 / §8.7：强制 iOS 设置 localReconcileRequired。
func (ds *DeliveryState) ApplyReconcileSignal(reason string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.ReconcileState = reconcileStateRequired
	ds.ReconcileSince = time.Now()
	slog.Info("reconcile: signal received", "reason", reason)
}

// MarkSessionReconcile 标记指定 session 需要 reconcile。
func (ds *DeliveryState) MarkSessionReconcile(sessionID string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.SessionReconcileRequired[sessionID] = true
}

// ClearSessionReconcile 清除指定 session 的 reconcile 标记。
func (ds *DeliveryState) ClearSessionReconcile(sessionID string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.SessionReconcileRequired, sessionID)
}

// IsSessionReconcileRequired 检查指定 session 是否需要 reconcile。
func (ds *DeliveryState) IsSessionReconcileRequired(sessionID string) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.SessionReconcileRequired[sessionID]
}

// StartReconcile 标记 reconcile 开始。
func (ds *DeliveryState) StartReconcile() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.ReconcileState = reconcileStateInProgress
}

// CompleteReconcile 标记 reconcile 完成，用 Mac 权威数据更新 chain head。
// 方案 §8.6：恢复后从 Mac history/Todo 收敛。
func (ds *DeliveryState) CompleteReconcile(macChainHead *DeliveryChainHead, macLastCounter uint64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.ReconcileState = reconcileStateCompleted
	if macChainHead != nil {
		ds.LastCommittedEpochDigest = macChainHead.EpochDigest
		ds.LastCommittedCounter = macLastCounter
	}

	slog.Info("reconcile: completed",
		"epochDigest", ds.LastCommittedEpochDigest,
		"lastCounter", ds.LastCommittedCounter,
	)
}

// IsReconcileRequired 返回是否需要 reconcile。
func (ds *DeliveryState) IsReconcileRequired() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.ReconcileState == reconcileStateRequired
}

// ValidateChainHead 验证 chain head 与本地状态一致。
// 方案 §5.5：get_delivery_chain_head 校验。
func (ds *DeliveryState) ValidateChainHead(head *DeliveryChainHead) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.LastCommittedEpochDigest == "" {
		// 首次，任何 chain head 都接受
		return nil
	}

	if head.EpochDigest != ds.LastCommittedEpochDigest {
		// Chain mismatch
		ds.ReconcileState = reconcileStateRequired
		ds.ReconcileSince = time.Now()
		slog.Warn("reconcile: chain head mismatch",
			"local", ds.LastCommittedEpochDigest,
			"remote", head.EpochDigest,
		)
		return ErrChainMismatch
	}

	return nil
}

// Snapshot 返回状态快照。
func (ds *DeliveryState) Snapshot() DeliveryState {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	sessions := make(map[string]bool)
	for k, v := range ds.SessionReconcileRequired {
		sessions[k] = v
	}

	return DeliveryState{
		LastCommittedCursor:      ds.LastCommittedCursor,
		LastCommittedEpochDigest: ds.LastCommittedEpochDigest,
		LastCommittedCounter:     ds.LastCommittedCounter,
		ReconcileState:           ds.ReconcileState,
		ReconcileSince:           ds.ReconcileSince,
		SessionReconcileRequired: sessions,
	}
}

// ── 错误定义 ─────────────────────────────────────────────────────────────

var (
	ErrReconcileRequired = &DeliveryError{Code: "relay.reconcile_required", Message: "reconcile required before applying frames"}
	ErrCounterGap        = &DeliveryError{Code: "relay.counter_invalid", Message: "counter gap detected, reconcile required"}
	ErrChainMismatch     = &DeliveryError{Code: "relay.chain_mismatch", Message: "chain head mismatch, reconcile required"}
)

// DeliveryError 是 delivery 层错误。
type DeliveryError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *DeliveryError) Error() string { return e.Message }
