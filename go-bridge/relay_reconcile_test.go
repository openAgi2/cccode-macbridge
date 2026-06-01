package gobridge

import (
	"testing"
)

// ─── Delivery State 与 Reconcile 测试 ───────────────────────────────────

func TestDeliveryStateApplyFrame(t *testing.T) {
	ds := NewDeliveryState()

	// 连续 counter 应成功
	err := ds.ApplyFrame(1, "", "text_delta")
	if err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	if ds.LastCommittedCounter != 1 {
		t.Errorf("counter = %d, want 1", ds.LastCommittedCounter)
	}

	err = ds.ApplyFrame(2, "", "turn_completed")
	if err != nil {
		t.Fatalf("frame 2: %v", err)
	}
	if ds.LastCommittedCounter != 2 {
		t.Errorf("counter = %d, want 2", ds.LastCommittedCounter)
	}
}

func TestDeliveryStateCounterGapTriggersReconcile(t *testing.T) {
	ds := NewDeliveryState()

	// 正常 frame
	ds.ApplyFrame(1, "", "text_delta")

	// Counter 空洞
	err := ds.ApplyFrame(5, "", "text_delta")
	if err != ErrCounterGap {
		t.Errorf("error = %v, want ErrCounterGap", err)
	}

	if !ds.IsReconcileRequired() {
		t.Error("should require reconcile after counter gap")
	}
}

func TestDeliveryStateReconcileRequiredBlocksFrames(t *testing.T) {
	ds := NewDeliveryState()

	// 设置 reconcile required
	ds.ApplyReconcileSignal("prekey_exhausted")

	// 尝试应用 frame 应失败
	err := ds.ApplyFrame(1, "", "text_delta")
	if err != ErrReconcileRequired {
		t.Errorf("error = %v, want ErrReconcileRequired", err)
	}
}

func TestDeliveryStateSessionReconcile(t *testing.T) {
	ds := NewDeliveryState()

	ds.MarkSessionReconcile("sess_1")
	if !ds.IsSessionReconcileRequired("sess_1") {
		t.Error("sess_1 should require reconcile")
	}
	if ds.IsSessionReconcileRequired("sess_2") {
		t.Error("sess_2 should not require reconcile")
	}

	ds.ClearSessionReconcile("sess_1")
	if ds.IsSessionReconcileRequired("sess_1") {
		t.Error("sess_1 should be cleared")
	}
}

func TestDeliveryStateChainHeadValidation(t *testing.T) {
	ds := NewDeliveryState()

	// 设置初始 chain head
	ds.ApplyFrame(1, "digest_a", "text_delta")

	// 相同 digest 应通过
	head := &DeliveryChainHead{EpochDigest: "digest_a"}
	err := ds.ValidateChainHead(head)
	if err != nil {
		t.Errorf("valid chain head: %v", err)
	}

	// 不同 digest 应触发 reconcile
	head = &DeliveryChainHead{EpochDigest: "digest_b"}
	err = ds.ValidateChainHead(head)
	if err != ErrChainMismatch {
		t.Errorf("mismatch error = %v, want ErrChainMismatch", err)
	}
	if !ds.IsReconcileRequired() {
		t.Error("chain mismatch should trigger reconcile")
	}
}

func TestDeliveryStateChainHeadFirstTime(t *testing.T) {
	ds := NewDeliveryState()

	// 首次任何 chain head 都应接受
	head := &DeliveryChainHead{EpochDigest: "any_digest"}
	err := ds.ValidateChainHead(head)
	if err != nil {
		t.Errorf("first time chain head: %v", err)
	}
}

func TestDeliveryStateReconcileLifecycle(t *testing.T) {
	ds := NewDeliveryState()

	// 正常 frame
	ds.ApplyFrame(1, "digest_1", "text_delta")
	ds.ApplyFrame(2, "digest_1", "turn_completed")

	// 触发 reconcile
	ds.ApplyReconcileSignal("outbox_overflow")
	if !ds.IsReconcileRequired() {
		t.Error("should require reconcile")
	}

	// 开始 reconcile
	ds.StartReconcile()
	snap := ds.Snapshot()
	if snap.ReconcileState != reconcileStateInProgress {
		t.Errorf("state = %q, want %q", snap.ReconcileState, reconcileStateInProgress)
	}

	// 完成 reconcile（用 Mac 权威数据更新）
	ds.CompleteReconcile(&DeliveryChainHead{
		EpochIndex:  1,
		EpochDigest: "digest_mac_authority",
	}, 10)

	snap = ds.Snapshot()
	if snap.ReconcileState != reconcileStateCompleted {
		t.Errorf("state = %q, want %q", snap.ReconcileState, reconcileStateCompleted)
	}
	if snap.LastCommittedCounter != 10 {
		t.Errorf("counter = %d, want 10 (Mac authority)", snap.LastCommittedCounter)
	}
	if snap.LastCommittedEpochDigest != "digest_mac_authority" {
		t.Errorf("digest = %q, want digest_mac_authority", snap.LastCommittedEpochDigest)
	}
}

func TestDeliveryStateSnapshotIsolation(t *testing.T) {
	ds := NewDeliveryState()
	ds.ApplyFrame(1, "d1", "text_delta")

	snap := ds.Snapshot()
	ds.ApplyFrame(2, "d1", "text_delta")

	if snap.LastCommittedCounter != 1 {
		t.Errorf("snapshot counter = %d, want 1 (should be isolated)", snap.LastCommittedCounter)
	}
}

func TestDeliveryStateEpochDigestUpdate(t *testing.T) {
	ds := NewDeliveryState()

	ds.ApplyFrame(1, "digest_epoch_0", "text_delta")
	if ds.LastCommittedEpochDigest != "digest_epoch_0" {
		t.Errorf("digest = %q, want digest_epoch_0", ds.LastCommittedEpochDigest)
	}

	ds.ApplyFrame(2, "digest_epoch_1", "text_delta")
	if ds.LastCommittedEpochDigest != "digest_epoch_1" {
		t.Errorf("digest = %q, want digest_epoch_1", ds.LastCommittedEpochDigest)
	}
}
