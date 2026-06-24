package gobridge

import (
	"net/http/httptest"
	"testing"
	"time"
)

// newAuthMiddlewareWithStore 创建一个带内存存储的 AuthMiddleware。
func newAuthMiddlewareWithStore() (*AuthMiddleware, *MemoryDeviceStore, string) {
	store := NewMemoryDeviceStore()
	m := NewAuthMiddleware(store)
	plain, hash, _ := GenerateDeviceToken()
	rec := TrustedDeviceRecord{
		DeviceID:    "test-device",
		DisplayName: "Test Device",
		Platform:    "ios",
		TokenHash:   hash,
		CreatedAt:   time.Now(),
		LastSeenAt:  time.Now(),
	}
	_ = store.AddDevice(rec)
	return m, store, plain
}

func TestAuthMiddleware_AuthenticateRequest_Success(t *testing.T) {
	m, _, plain := newAuthMiddlewareWithStore()
	r := httptest.NewRequest("GET", "/bridge", nil)
	r.Header.Set("Authorization", "Bearer "+plain)
	r.Header.Set("X-CordCode-Device-ID", "test-device")

	rec, err := m.AuthenticateRequest(r)
	if err != nil {
		t.Fatalf("认证应成功: %v", err)
	}
	if rec.DeviceID != "test-device" {
		t.Errorf("DeviceID 不匹配: got %q, want %q", rec.DeviceID, "test-device")
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	m, _, _ := newAuthMiddlewareWithStore()
	r := httptest.NewRequest("GET", "/bridge", nil)

	_, err := m.AuthenticateRequest(r)
	authErr, ok := err.(AuthError)
	if !ok {
		t.Fatalf("应返回 AuthError, got %T: %v", err, err)
	}
	if authErr.Code != "auth.missing_token" {
		t.Errorf("错误码应为 auth.missing_token, got %q", authErr.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	m, _, _ := newAuthMiddlewareWithStore()
	r := httptest.NewRequest("GET", "/bridge", nil)
	r.Header.Set("Authorization", "Bearer bad_token")
	r.Header.Set("X-CordCode-Device-ID", "test-device")

	_, err := m.AuthenticateRequest(r)
	authErr, ok := err.(AuthError)
	if !ok {
		t.Fatalf("应返回 AuthError, got %T: %v", err, err)
	}
	if authErr.Code != "auth.invalid_token" {
		t.Errorf("错误码应为 auth.invalid_token, got %q", authErr.Code)
	}
}

func TestAuthMiddleware_RevokedToken(t *testing.T) {
	m, store, plain := newAuthMiddlewareWithStore()
	_ = store.RevokeDevice("test-device")

	r := httptest.NewRequest("GET", "/bridge", nil)
	r.Header.Set("Authorization", "Bearer "+plain)
	r.Header.Set("X-CordCode-Device-ID", "test-device")

	_, err := m.AuthenticateRequest(r)
	authErr, ok := err.(AuthError)
	if !ok {
		t.Fatalf("应返回 AuthError, got %T: %v", err, err)
	}
	if authErr.Code != "auth.revoked" {
		t.Errorf("错误码应为 auth.revoked, got %q", authErr.Code)
	}
}

func TestAuthMiddleware_UnknownToken(t *testing.T) {
	m, _, _ := newAuthMiddlewareWithStore()
	plain, _, _ := GenerateDeviceToken() // 新 token，不在 store 中

	r := httptest.NewRequest("GET", "/bridge", nil)
	r.Header.Set("Authorization", "Bearer "+plain)

	_, err := m.AuthenticateRequest(r)
	authErr, ok := err.(AuthError)
	if !ok {
		t.Fatalf("应返回 AuthError, got %T: %v", err, err)
	}
	if authErr.Code != "auth.invalid_token" {
		t.Errorf("错误码应为 auth.invalid_token, got %q", authErr.Code)
	}
}

func TestAuthMiddleware_IsPairingEndpoint_True(t *testing.T) {
	m, _, _ := newAuthMiddlewareWithStore()
	r := httptest.NewRequest("GET", "/pairing", nil)
	if !m.IsPairingEndpoint(r) {
		t.Error("/pairing 应为配对端点")
	}
}

func TestAuthMiddleware_QueryParamFallback(t *testing.T) {
	m, _, plain := newAuthMiddlewareWithStore()

	// 无 header，仅 query 参数
	r := httptest.NewRequest("GET", "/bridge?token="+plain+"&deviceId=test-device", nil)
	rec, err := m.AuthenticateRequest(r)
	if err != nil {
		t.Fatalf("query-param auth 应成功: %v", err)
	}
	if rec.DeviceID != "test-device" {
		t.Errorf("DeviceID 不匹配: got %q, want %q", rec.DeviceID, "test-device")
	}
}

func TestAuthMiddleware_HeaderPreferredOverQuery(t *testing.T) {
	m, _, plain := newAuthMiddlewareWithStore()

	// 同时有 header 和 query，header 优先
	r := httptest.NewRequest("GET", "/bridge?token=wrong_token&deviceId=wrong", nil)
	r.Header.Set("Authorization", "Bearer "+plain)
	r.Header.Set("X-CordCode-Device-ID", "test-device")

	rec, err := m.AuthenticateRequest(r)
	if err != nil {
		t.Fatalf("应使用 header auth: %v", err)
	}
	if rec.DeviceID != "test-device" {
		t.Errorf("DeviceID 不匹配: got %q, want %q", rec.DeviceID, "test-device")
	}
}

func TestAuthMiddleware_IsPairingEndpoint_False(t *testing.T) {
	m, _, _ := newAuthMiddlewareWithStore()
	r := httptest.NewRequest("GET", "/bridge", nil)
	if m.IsPairingEndpoint(r) {
		t.Error("/bridge 不应为配对端点")
	}
}

func TestExtractBearerToken_Present(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer mytoken123")
	got := extractBearerToken(r)
	if got != "mytoken123" {
		t.Errorf("应提取 Bearer token, got %q", got)
	}
}

func TestExtractBearerToken_Missing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	got := extractBearerToken(r)
	if got != "" {
		t.Errorf("无 Authorization 头应返回空, got %q", got)
	}
}

func TestExtractBearerToken_WrongScheme(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic abc123")
	got := extractBearerToken(r)
	if got != "" {
		t.Errorf("非 Bearer scheme 应返回空, got %q", got)
	}
}
