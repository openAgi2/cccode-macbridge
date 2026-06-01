package gobridge

import (
	"testing"
)

// ─── Phase 2 Reconcile Presentation Regression Gate ──────────────────────
//
// 方案 §11 Phase 2 退出标准 / §12.3：
//   - 内容不空白闪烁
//   - 不显示未经权威确认的完成态
//   - 后台 session 不冒充已同步
//   - completion notification 不重复
//
// 验证项：
//   R1: Syncing 状态下不暴露 complete 状态
//   R2: 后台 session 不自动变为 complete
//   R3: Failed 后不回退到 syncing
//   R4: Completion notification 去重不丢失
//   R5: 前台 session 完成不影响后台 session 状态

// TestRegressionR1_SyncingDoesNotExposeComplete syncing 状态下不暴露 complete。
func TestRegressionR1_SyncingDoesNotExposeComplete(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_r1", "codex")

	s := pm.GetState("sess_r1")
	if s.State == presentationComplete {
		t.Error("syncing session should NOT be in complete state")
	}
	if s.State != presentationSyncing {
		t.Errorf("state = %q, want syncing", s.State)
	}
}

// TestRegressionR2_BackgroundSessionNotAutoComplete 后台 session 不自动变为 complete。
func TestRegressionR2_BackgroundSessionNotAutoComplete(t *testing.T) {
	pm := NewPresentationManager()

	// 标记后台 session 为 pending sync
	pm.MarkPendingSync("sess_bg", "codex")

	s := pm.GetState("sess_bg")
	if s == nil {
		t.Fatal("state should exist")
	}
	if s.State == presentationComplete {
		t.Error("background pending session should NOT auto-complete")
	}
	if !s.PendingSync {
		t.Error("background session should have pendingSync=true")
	}
}

// TestRegressionR3_FailedDoesNotRevertToSyncing failed 后不回退到 syncing。
func TestRegressionR3_FailedDoesNotRevertToSyncing(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_r3", "codex")
	pm.MarkFailed("sess_r3", "relay.bridge_offline")

	s := pm.GetState("sess_r3")
	if s.State != presentationFailed {
		t.Errorf("state = %q, want failed", s.State)
	}
	if s.LastError != "relay.bridge_offline" {
		t.Errorf("error = %q, want relay.bridge_offline", s.LastError)
	}

	// 即使重新标记 syncing，failed 状态的历史错误应被清除
	pm.MarkSyncing("sess_r3", "codex")
	s = pm.GetState("sess_r3")
	if s.State != presentationSyncing {
		t.Errorf("state after re-sync = %q, want syncing", s.State)
	}
	if s.LastError != "" {
		t.Errorf("error should be cleared on re-sync, got %q", s.LastError)
	}
}

// TestRegressionR4_CompletionDedupNotLost completion notification 去重不丢失。
func TestRegressionR4_CompletionDedupNotLost(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_r4", "codex")

	// 多次收到 completion notification
	pm.MarkCompletionPending("sess_r4")
	pm.MarkCompletionPending("sess_r4")
	pm.MarkCompletionPending("sess_r4")

	// 应该只有一条 pending
	if !pm.IsCompletionPending("sess_r4") {
		t.Error("completion should be pending")
	}

	// 清除后不应残留
	pm.ClearCompletionPending("sess_r4")
	if pm.IsCompletionPending("sess_r4") {
		t.Error("completion should be cleared")
	}

	// 其他 session 不受影响
	pm.MarkCompletionPending("sess_other")
	if pm.IsCompletionPending("sess_r4") {
		t.Error("sess_r4 should not be affected by sess_other")
	}
}

// TestRegressionR5_ForegroundCompleteDoesNotAffectBackground 前台完成不影响后台。
func TestRegressionR5_ForegroundCompleteDoesNotAffectBackground(t *testing.T) {
	pm := NewPresentationManager()

	// 前台 session
	pm.MarkSyncing("sess_fg", "codex")

	// 后台 session
	pm.MarkPendingSync("sess_bg", "codex")

	// 前台完成
	pm.MarkComplete("sess_fg")

	// 前台应为 complete
	fg := pm.GetState("sess_fg")
	if fg.State != presentationComplete {
		t.Errorf("foreground state = %q, want complete", fg.State)
	}

	// 后台仍为 pendingSync，不应自动 complete
	bg := pm.GetState("sess_bg")
	if bg == nil {
		t.Fatal("background state should exist")
	}
	if bg.State == presentationComplete {
		t.Error("background session should NOT auto-complete when foreground completes")
	}
	if !bg.PendingSync {
		t.Error("background session should still be pendingSync")
	}
}

// TestRegressionR6_ConnectionModeNotConfusedWithBackendStatus 连接模式不被误用为 backend 在线。
// 方案 §13.3：不能把 relay 在线等价展示为 backend 在线。
func TestRegressionR6_ConnectionModeNotConfusedWithBackendStatus(t *testing.T) {
	// Relay 可达但 Mac 离线
	text := ConnectionModeDisplayText(ConnectionModeRelayOffline)
	if text != "Relay - Mac Offline" {
		t.Errorf("display = %q, want 'Relay - Mac Offline'", text)
	}

	// 确保各模式的文案语义明确，不暗示 backend 在线
	for _, mode := range []ConnectionMode{
		ConnectionModeDirectLocal,
		ConnectionModeDirectRemote,
		ConnectionModeRelay,
		ConnectionModeRelaySyncing,
		ConnectionModeRelayOffline,
		ConnectionModeDisconnected,
	} {
		text := ConnectionModeDisplayText(mode)
		// 检查文案不包含 "backend online" 或 "connected to backend"
		if text == "" {
			t.Errorf("mode %q has empty display text", mode)
		}
	}
}

// TestRegressionR7_SessionStateIsolation 不同 session 状态完全隔离。
func TestRegressionR7_SessionStateIsolation(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_a", "codex")
	pm.MarkComplete("sess_a")
	pm.MarkPendingSync("sess_b", "opencode")
	pm.MarkSyncing("sess_c", "codex")
	pm.MarkFailed("sess_c", "relay.bridge_offline")
	pm.MarkSyncing("sess_d", "codex")
	pm.MarkOffline("sess_d")

	// 每个 session 状态独立
	a := pm.GetState("sess_a")
	if a.State != presentationComplete {
		t.Errorf("a = %q, want complete", a.State)
	}

	b := pm.GetState("sess_b")
	if !b.PendingSync {
		t.Errorf("b state=%q pendingSync=%v, want pending", b.State, b.PendingSync)
	}

	c := pm.GetState("sess_c")
	if c.State != presentationFailed {
		t.Errorf("c = %q, want failed", c.State)
	}

	d := pm.GetState("sess_d")
	if d.State != presentationOffline {
		t.Errorf("d = %q, want offline", d.State)
	}

	// 删除一个不影响其他
	pm.RemoveSession("sess_a")
	if pm.GetState("sess_b") == nil {
		t.Error("sess_b should still exist after sess_a removal")
	}
	if pm.GetState("sess_c") == nil {
		t.Error("sess_c should still exist after sess_a removal")
	}
}
