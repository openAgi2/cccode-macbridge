package gobridge

import (
	"encoding/json"
	"testing"
)

// ─── Reconcile Presentation 测试 ────────────────────────────────────────

func TestPresentationSyncingState(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_1", "codex")
	s := pm.GetState("sess_1")
	if s == nil {
		t.Fatal("state should exist")
	}
	if s.State != presentationSyncing {
		t.Errorf("state = %q, want %q", s.State, presentationSyncing)
	}
}

func TestPresentationCompleteState(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_1", "codex")
	pm.MarkComplete("sess_1")

	s := pm.GetState("sess_1")
	if s.State != presentationComplete {
		t.Errorf("state = %q, want %q", s.State, presentationComplete)
	}
	if s.LastVerifiedAt.IsZero() {
		t.Error("lastVerifiedAt should be set")
	}
	if s.PendingSync {
		t.Error("pendingSync should be cleared on complete")
	}
}

func TestPresentationFailedState(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_1", "codex")
	pm.MarkFailed("sess_1", "relay.bridge_offline")

	s := pm.GetState("sess_1")
	if s.State != presentationFailed {
		t.Errorf("state = %q, want %q", s.State, presentationFailed)
	}
	if s.LastError != "relay.bridge_offline" {
		t.Errorf("error = %q, want relay.bridge_offline", s.LastError)
	}
}

func TestPresentationOfflineState(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_1", "codex")
	pm.MarkOffline("sess_1")

	s := pm.GetState("sess_1")
	if s.State != presentationOffline {
		t.Errorf("state = %q, want %q", s.State, presentationOffline)
	}
}

func TestPresentationBackgroundPendingSync(t *testing.T) {
	pm := NewPresentationManager()

	// 后台 session 标记待同步
	pm.MarkPendingSync("sess_bg_1", "codex")
	pm.MarkPendingSync("sess_bg_2", "opencode")

	// 前台 session 开始同步
	pm.MarkSyncing("sess_fg", "codex")

	// 获取所有待同步
	pending := pm.GetAllPendingSync()
	if len(pending) != 2 {
		t.Errorf("pending = %d, want 2", len(pending))
	}

	// 前台 session 不在 pending 列表中
	for _, sid := range pending {
		if sid == "sess_fg" {
			t.Error("foreground syncing session should not be in pending list")
		}
	}
}

func TestPresentationCompletionDedup(t *testing.T) {
	pm := NewPresentationManager()

	pm.MarkSyncing("sess_1", "codex")

	// 收到 completion notification
	pm.MarkCompletionPending("sess_1")
	if !pm.IsCompletionPending("sess_1") {
		t.Error("should be pending")
	}

	// 再次标记应仍为 pending（不重复）
	pm.MarkCompletionPending("sess_1")
	if !pm.IsCompletionPending("sess_1") {
		t.Error("should still be pending")
	}

	// 处理完毕后清除
	pm.ClearCompletionPending("sess_1")
	if pm.IsCompletionPending("sess_1") {
		t.Error("should be cleared")
	}
}

func TestPresentationAtomicUpdate(t *testing.T) {
	pm := NewPresentationManager()

	// 模拟完整 lifecycle
	pm.MarkSyncing("sess_1", "codex") // 开始同步
	pm.MarkComplete("sess_1")         // 原子替换完成

	s := pm.GetState("sess_1")
	if s.State != presentationComplete {
		t.Errorf("final state = %q, want %q", s.State, presentationComplete)
	}
}

func TestPresentationConnectionModeDisplay(t *testing.T) {
	tests := []struct {
		mode    ConnectionMode
		display string
	}{
		{ConnectionModeDirectLocal, "Direct - Local"},
		{ConnectionModeDirectRemote, "Direct - Remote"},
		{ConnectionModeRelay, "Relay - Encrypted"},
		{ConnectionModeRelaySyncing, "Relay - Syncing"},
		{ConnectionModeRelayOffline, "Relay - Mac Offline"},
		{ConnectionModeDisconnected, "Disconnected"},
	}
	for _, tt := range tests {
		got := ConnectionModeDisplayText(tt.mode)
		if got != tt.display {
			t.Errorf("display(%q) = %q, want %q", tt.mode, got, tt.display)
		}
	}
}

func TestPresentationParseSetObservationScopeRequest(t *testing.T) {
	params := json.RawMessage(`{
		"backendId": "codex",
		"sessionIds": ["sess_1", "sess_2"],
		"deliveryMode": "full_stream",
		"includeRunningSessionSignals": true,
		"leaseSeconds": 45
	}`)

	req, err := ParseSetObservationScopeRequest(params)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.BackendID != "codex" {
		t.Errorf("backendID = %q, want codex", req.BackendID)
	}
	if req.DeliveryMode != "full_stream" {
		t.Errorf("mode = %q, want full_stream", req.DeliveryMode)
	}
	if len(req.SessionIDs) != 2 {
		t.Errorf("sessions = %d, want 2", len(req.SessionIDs))
	}
	if req.LeaseSeconds != 45 {
		t.Errorf("lease = %d, want 45", req.LeaseSeconds)
	}

	// 无效请求
	_, err = ParseSetObservationScopeRequest(json.RawMessage(`{"backendId":""}`))
	if err == nil {
		t.Error("empty backendID should be invalid")
	}

	_, err = ParseSetObservationScopeRequest(json.RawMessage(`{"backendId":"codex","deliveryMode":"invalid"}`))
	if err == nil {
		t.Error("invalid deliveryMode should be rejected")
	}
}
