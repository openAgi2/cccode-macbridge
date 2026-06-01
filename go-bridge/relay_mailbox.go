package gobridge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ─── Mailbox 服务 ───────────────────────────────────────────────────────
//
// 方案 §7.3-7.4 / §15.4：
//   Relay service 实现 ciphertext mailbox、deliveryCursor、crash-safe ack 支撑、
//   TTL/capacity、revocation 清理与 opaque epoch 元数据保存。
//
// Mailbox 只存储密文信封，不解析内层 payload。
// TTL 到期后 frame 可被淘汰，iOS 回源 Mac reconcile。
// ack 必须在 iOS 端 durable apply 之后才发送。

const (
	defaultMailboxTTL     = 24 * time.Hour   // 默认 TTL 24 小时
	maxMailboxBytesPerDev = 50 * 1024 * 1024 // 单设备最大 mailbox 50MB
)

// MailboxFrameMetadata 是 mailbox frame 的可检索元数据。
// 不包含 ciphertext 解码内容。
type MailboxFrameMetadata struct {
	Cursor     uint64    `json:"cursor"`
	DeviceID   string    `json:"deviceId"`
	RouteID    string    `json:"routeId"`
	KeyEpochID string    `json:"keyEpochId"`
	Counter    uint64    `json:"counter"`
	PrekeyID   string    `json:"prekeyId,omitempty"`
	EpochIndex *uint64   `json:"epochIndex,omitempty"`
	Size       int       `json:"size"`
	InsertedAt time.Time `json:"insertedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Acked      bool      `json:"acked"`
}

// MailboxFetchResponse mailbox 补取响应。
type MailboxFetchResponse struct {
	Frames     []MailboxFrameEntry `json:"frames"`
	HasMore    bool                `json:"hasMore"`
	NextCursor uint64              `json:"nextCursor,omitempty"`
}

// MailboxFrameEntry 单个 mailbox frame 返回项。
type MailboxFrameEntry struct {
	Cursor   uint64          `json:"cursor"`
	Envelope json.RawMessage `json:"envelope"`
}

// MailboxAckRequest ack 请求。
type MailboxAckRequest struct {
	RouteID  string `json:"routeId"`
	DeviceID string `json:"deviceId"`
	Cursor   uint64 `json:"cursor"` // ack 此 cursor 及之前的所有 frame
}

// MailboxService 管理 relay 的密文 mailbox 存储。
// 部署在 relay service 端，与 RelayHub 协作。
type MailboxService struct {
	mu sync.Mutex

	hub *RelayHub

	// per-device mailbox
	mailboxes  map[string]*deviceMailbox // key: routeID + ":" + deviceID
	mailboxSeq uint64                    // 全局单调 cursor
}

type deviceMailbox struct {
	routeID    string
	deviceID   string
	frames     []mailboxEntry
	totalBytes int64
}

type mailboxEntry struct {
	cursor     uint64
	envelope   json.RawMessage
	insertedAt time.Time
	expiresAt  time.Time
	acked      bool
	size       int

	// opaque epoch 元数据（用于 epoch chain 验证）
	epochIndex *uint64
	prekeyID   string
	keyEpochID string
	counter    uint64
}

// NewMailboxService 创建 mailbox service。
func NewMailboxService(hub *RelayHub) *MailboxService {
	return &MailboxService{
		hub:       hub,
		mailboxes: make(map[string]*deviceMailbox),
	}
}

// mailboxKey 返回 mailbox 的 map key。
func mailboxKey(routeID, deviceID string) string {
	return routeID + ":" + deviceID
}

// Enqueue 将密文信封入队到指定设备的 mailbox。
// 方案 §7.3：离线 device 的 frame 存入 mailbox。
// 方案 §15.4：opaque epoch 元数据保存。
func (ms *MailboxService) Enqueue(routeID, deviceID string, envelope json.RawMessage, epochMeta *EpochMetadata) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	key := mailboxKey(routeID, deviceID)
	mb, ok := ms.mailboxes[key]
	if !ok {
		mb = &deviceMailbox{
			routeID:  routeID,
			deviceID: deviceID,
		}
		ms.mailboxes[key] = mb
	}

	// 容量检查
	frameSize := len(envelope)
	if mb.totalBytes+int64(frameSize) > maxMailboxBytesPerDev {
		// 淘汰最早未 ack 的 frame
		ms.evictOldestUnacked(mb, int64(frameSize))
	}

	// 检查是否仍超容量
	if mb.totalBytes+int64(frameSize) > maxMailboxBytesPerDev {
		return fmt.Errorf("mailbox capacity exceeded for device %s", safeID(deviceID))
	}

	ms.mailboxSeq++
	entry := mailboxEntry{
		cursor:     ms.mailboxSeq,
		envelope:   envelope,
		insertedAt: time.Now(),
		expiresAt:  time.Now().Add(defaultMailboxTTL),
		size:       frameSize,
	}

	// 保存 opaque epoch 元数据
	if epochMeta != nil {
		entry.epochIndex = epochMeta.EpochIndex
		entry.prekeyID = epochMeta.PrekeyID
		entry.keyEpochID = epochMeta.KeyEpochID
		entry.counter = epochMeta.Counter
	}

	mb.frames = append(mb.frames, entry)
	mb.totalBytes += int64(frameSize)

	slog.Debug("mailbox-service: enqueue",
		"routeID", safeID(routeID),
		"deviceID", safeID(deviceID),
		"cursor", entry.cursor,
		"size", frameSize,
		"totalBytes", mb.totalBytes,
	)

	return nil
}

// Fetch 补取设备 mailbox 中 afterCursor 之后的未确认 frame。
// 方案 §7.3：cursor 补取，支持分页。
func (ms *MailboxService) Fetch(routeID, deviceID string, afterCursor uint64, limit int) MailboxFetchResponse {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	key := mailboxKey(routeID, deviceID)
	mb, ok := ms.mailboxes[key]
	if !ok {
		return MailboxFetchResponse{}
	}

	if limit <= 0 {
		limit = 100 // 默认分页大小
	}

	var frames []MailboxFrameEntry
	var nextCursor uint64
	hasMore := false

	for _, f := range mb.frames {
		if f.acked || f.cursor <= afterCursor {
			continue
		}
		if len(frames) >= limit {
			hasMore = true
			nextCursor = f.cursor
			break
		}
		frames = append(frames, MailboxFrameEntry{
			Cursor:   f.cursor,
			Envelope: f.envelope,
		})
	}

	resp := MailboxFetchResponse{
		Frames:  frames,
		HasMore: hasMore,
	}
	if hasMore {
		resp.NextCursor = nextCursor
	}

	return resp
}

// Ack 确认 cursor 及之前的 frame 已被 durable apply。
// 方案 §15.4：crash-safe ack——iOS 在 durable apply 或 durable
// localReconcileRequired 标记完成后才 ack。
func (ms *MailboxService) Ack(routeID, deviceID string, cursor uint64) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	key := mailboxKey(routeID, deviceID)
	mb, ok := ms.mailboxes[key]
	if !ok {
		return fmt.Errorf("mailbox not found for device %s", safeID(deviceID))
	}

	ackedBytes := int64(0)
	ackedCount := 0
	for i := range mb.frames {
		if mb.frames[i].cursor <= cursor && !mb.frames[i].acked {
			mb.frames[i].acked = true
			ackedBytes += int64(mb.frames[i].size)
			ackedCount++
		}
	}
	mb.totalBytes -= ackedBytes

	slog.Debug("mailbox-service: ack",
		"routeID", safeID(routeID),
		"deviceID", safeID(deviceID),
		"cursor", cursor,
		"ackedCount", ackedCount,
		"freedBytes", ackedBytes,
	)

	return nil
}

// Expire 清理已过期的 mailbox frame。
// 方案 §15.4：TTL 到期后 frame 可被淘汰。
func (ms *MailboxService) Expire(routeID, deviceID string) int {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	key := mailboxKey(routeID, deviceID)
	mb, ok := ms.mailboxes[key]
	if !ok {
		return 0
	}

	now := time.Now()
	expiredBytes := int64(0)
	expiredCount := 0
	remaining := mb.frames[:0]

	for _, f := range mb.frames {
		if now.After(f.expiresAt) && !f.acked {
			expiredBytes += int64(f.size)
			expiredCount++
		} else {
			remaining = append(remaining, f)
		}
	}

	mb.frames = remaining
	mb.totalBytes -= expiredBytes

	if expiredCount > 0 {
		slog.Info("mailbox-service: expired frames",
			"routeID", safeID(routeID),
			"deviceID", safeID(deviceID),
			"count", expiredCount,
			"freedBytes", expiredBytes,
		)
	}

	return expiredCount
}

// ClearForDevice 清除设备的所有 mailbox 数据。
// 用于设备撤销时调用。
// 方案 §7.3 POST /v1/devices/revoke。
func (ms *MailboxService) ClearForDevice(routeID, deviceID string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	key := mailboxKey(routeID, deviceID)
	delete(ms.mailboxes, key)

	slog.Info("mailbox-service: cleared for device",
		"routeID", safeID(routeID),
		"deviceID", safeID(deviceID),
	)
}

// Stats 返回设备的 mailbox 统计信息。
// 仅暴露聚合统计，不暴露 payload。
func (ms *MailboxService) Stats(routeID, deviceID string) (totalFrames, pendingFrames int, totalBytes int64, ok bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	key := mailboxKey(routeID, deviceID)
	mb, ok := ms.mailboxes[key]
	if !ok {
		return 0, 0, 0, false
	}

	pending := 0
	for _, f := range mb.frames {
		if !f.acked {
			pending++
		}
	}

	return len(mb.frames), pending, mb.totalBytes, true
}

// ── 内部方法 ─────────────────────────────────────────────────────────────

func (ms *MailboxService) evictOldestUnacked(mb *deviceMailbox, neededBytes int64) {
	evictedBytes := int64(0)
	for i := range mb.frames {
		if mb.frames[i].acked {
			continue
		}
		evictedBytes += int64(mb.frames[i].size)
		mb.frames[i].acked = true // 标记为已处理（实际丢弃）
		if evictedBytes >= neededBytes {
			break
		}
	}
	mb.totalBytes -= evictedBytes
}

// ── Epoch Metadata ───────────────────────────────────────────────────────

// EpochMetadata 是 mailbox frame 关联的 opaque epoch 元数据。
// 用于 delivery epoch chain 验证，不包含业务内容。
type EpochMetadata struct {
	EpochIndex *uint64 `json:"epochIndex,omitempty"`
	PrekeyID   string  `json:"prekeyId,omitempty"`
	KeyEpochID string  `json:"keyEpochId,omitempty"`
	Counter    uint64  `json:"counter"`
}

// ── Milestone 白名单 ─────────────────────────────────────────────────────

// durableMilestoneWhitelist 是 mailbox 持久投递的事件类型白名单。
// 方案 §8.3：白名单外事件不持久投递。
var durableMilestoneWhitelist = map[string]bool{
	"turn_completed":              true,
	"turn_error":                  true,
	"todos_updated":               true,
	"session_running_signal":      true,
	"delivery_reconcile_required": true,
}

// IsDurableMilestone 检查事件类型是否在 mailbox 持久投递白名单中。
func IsDurableMilestone(eventType string) bool {
	return durableMilestoneWhitelist[eventType]
}

// ── Reconcile 控制消息 ───────────────────────────────────────────────────

// DeliveryReconcileRequired 是 Mac→iOS 的控制消息。
// 方案 §5.4 / §8.7：当 prekey 耗尽或 delivery chain 出现空洞时发送。
type DeliveryReconcileRequired struct {
	Type     string `json:"type"` // "delivery_reconcile_required"
	DeviceID string `json:"deviceId"`
	Reason   string `json:"reason"` // "prekey_exhausted" | "chain_mismatch" | "epoch_gap"
}

// NewDeliveryReconcileRequired 创建 reconcile 控制消息。
func NewDeliveryReconcileRequired(deviceID, reason string) *DeliveryReconcileRequired {
	return &DeliveryReconcileRequired{
		Type:     "delivery_reconcile_required",
		DeviceID: deviceID,
		Reason:   reason,
	}
}
