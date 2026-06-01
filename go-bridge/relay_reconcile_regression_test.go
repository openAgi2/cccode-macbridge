package gobridge

import (
	"testing"
)

// ─── Phase 2 iOS Replay/Reconcile Regression Gate ───────────────────────
//
// 方案 §12.3 故障注入：
//   - counter 空洞
//   - chain mismatch
//   - reconcile lifecycle
//   - ack 安全性
//   - Mac authority convergence

// TestRegressionR1_CounterGapBlocksAllSubsequentFrames 验证 counter gap 后所有后续 frame 被阻塞。
func TestRegressionR1_CounterGapBlocksAllSubsequentFrames(t *testing.T) {
	ds := NewDeliveryState()

	ds.ApplyFrame(1, "d1", "text_delta")
	ds.ApplyFrame(2, "d1", "text_delta")

	// Gap: 3 → 7
	err := ds.ApplyFrame(7, "d1", "text_delta")
	if err != ErrCounterGap {
		t.Errorf("gap error = %v, want ErrCounterGap", err)
	}

	// 即使后续 frame 也不能应用（reconcile required 状态）
	err = ds.ApplyFrame(8, "d1", "turn_completed")
	if err != ErrReconcileRequired {
		t.Errorf("post-gap frame error = %v, want ErrReconcileRequired", err)
	}

	// counter 应停留在 2（gap 前的最后值）
	if ds.LastCommittedCounter != 2 {
		t.Errorf("counter = %d, want 2", ds.LastCommittedCounter)
	}
}

// TestRegressionR2_ChainMismatchTriggersReconcile 验证 chain mismatch 触发 reconcile。
func TestRegressionR2_ChainMismatchTriggersReconcile(t *testing.T) {
	ds := NewDeliveryState()

	ds.ApplyFrame(1, "digest_local", "text_delta")

	// Mac 端返回不同的 chain head
	head := &DeliveryChainHead{
		EpochIndex:  5,
		EpochDigest: "digest_remote",
	}
	err := ds.ValidateChainHead(head)
	if err != ErrChainMismatch {
		t.Errorf("error = %v, want ErrChainMismatch", err)
	}

	if !ds.IsReconcileRequired() {
		t.Error("chain mismatch should trigger reconcile")
	}
}

// TestRegressionR3_ReconcileCompletesWithMacAuthority 验证 reconcile 用 Mac 权威数据收敛。
func TestRegressionR3_ReconcileCompletesWithMacAuthority(t *testing.T) {
	ds := NewDeliveryState()

	// 本地状态
	ds.ApplyFrame(1, "d1", "text_delta")
	ds.ApplyReconcileSignal("test")

	// Reconcile lifecycle
	ds.StartReconcile()

	// Mac 权威数据
	ds.CompleteReconcile(&DeliveryChainHead{
		EpochIndex:  3,
		EpochDigest: "mac_authority_digest",
	}, 100)

	// 本地状态应以 Mac 为准
	snap := ds.Snapshot()
	if snap.LastCommittedCounter != 100 {
		t.Errorf("counter = %d, want 100 (Mac authority)", snap.LastCommittedCounter)
	}
	if snap.LastCommittedEpochDigest != "mac_authority_digest" {
		t.Errorf("digest = %q, want mac_authority_digest", snap.LastCommittedEpochDigest)
	}
}

// TestRegressionR4_AckAfterDurableApply 验证 ack 在 durable apply 后。
// 方案 §8.4：durable write 成功后才向 Relay ack。
func TestRegressionR4_AckAfterDurableApply(t *testing.T) {
	ds := NewDeliveryState()

	// 模拟：接收 frame 1-3，逐个 durable apply
	for i := 1; i <= 3; i++ {
		err := ds.ApplyFrame(uint64(i), "d1", "text_delta")
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		// 在真实 iOS 端，这里会 durable write 到 Keychain
		// 然后才 ack
	}

	// Ack 应包含 cursor 3（最后一个成功 durable apply 的）
	if ds.LastCommittedCounter != 3 {
		t.Errorf("lastCommittedCounter = %d, want 3", ds.LastCommittedCounter)
	}
}

// TestRegressionR5_CrashRecoveryFromReconcileRequired 验证崩溃恢复后从 reconcile 状态恢复。
func TestRegressionR5_CrashRecoveryFromReconcileRequired(t *testing.T) {
	ds := NewDeliveryState()

	ds.ApplyFrame(1, "d1", "text_delta")
	ds.ApplyReconcileSignal("prekey_exhausted")

	// 模拟崩溃：保存 snapshot
	snap := ds.Snapshot()

	// 模拟重启：从 snapshot 恢复
	ds2 := &DeliveryState{
		LastCommittedCursor:      snap.LastCommittedCursor,
		LastCommittedEpochDigest: snap.LastCommittedEpochDigest,
		LastCommittedCounter:     snap.LastCommittedCounter,
		ReconcileState:           snap.ReconcileState,
		SessionReconcileRequired: snap.SessionReconcileRequired,
	}

	// 重启后应仍需 reconcile
	if !ds2.IsReconcileRequired() {
		t.Error("should still require reconcile after crash recovery")
	}

	// 尝试应用 frame 应被阻塞
	err := ds2.ApplyFrame(2, "d1", "text_delta")
	if err != ErrReconcileRequired {
		t.Errorf("post-crash frame error = %v, want ErrReconcileRequired", err)
	}
}

// TestRegressionR6_PerSessionReconcileIsolation 验证 per-session reconcile 隔离。
func TestRegressionR6_PerSessionReconcileIsolation(t *testing.T) {
	ds := NewDeliveryState()

	ds.MarkSessionReconcile("sess_1")
	ds.MarkSessionReconcile("sess_3")

	if !ds.IsSessionReconcileRequired("sess_1") {
		t.Error("sess_1 should require reconcile")
	}
	if ds.IsSessionReconcileRequired("sess_2") {
		t.Error("sess_2 should NOT require reconcile")
	}
	if !ds.IsSessionReconcileRequired("sess_3") {
		t.Error("sess_3 should require reconcile")
	}

	ds.ClearSessionReconcile("sess_1")
	if ds.IsSessionReconcileRequired("sess_1") {
		t.Error("sess_1 should be cleared")
	}
	if !ds.IsSessionReconcileRequired("sess_3") {
		t.Error("sess_3 should still require reconcile")
	}
}

// TestRegressionR7_DeliveryReconcileControlMessage 验证 delivery_reconcile_required 控制消息处理。
func TestRegressionR7_DeliveryReconcileControlMessage(t *testing.T) {
	ds := NewDeliveryState()

	// 正常运行中收到 reconcile 信号
	ds.ApplyFrame(1, "d1", "turn_completed")
	ds.ApplyReconcileSignal("outbox_overflow")

	// 应阻塞后续 frame
	err := ds.ApplyFrame(2, "d1", "text_delta")
	if err != ErrReconcileRequired {
		t.Errorf("error = %v, want ErrReconcileRequired", err)
	}

	// 完成 reconcile
	ds.StartReconcile()
	ds.CompleteReconcile(&DeliveryChainHead{
		EpochDigest: "mac_new",
	}, 50)

	// 确认状态收敛
	if ds.IsReconcileRequired() {
		t.Error("should not require reconcile after completion")
	}
	if ds.LastCommittedCounter != 50 {
		t.Errorf("counter = %d, want 50", ds.LastCommittedCounter)
	}
}
