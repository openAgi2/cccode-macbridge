package gobridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerServeHTTP_RootPathRequiresAuthWhenEnabled(t *testing.T) {
	auth, _, _ := newAuthMiddlewareWithStore()
	server := NewServer(NewHandlers())
	server.SetAuthMiddleware(auth)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://bridge.local/", nil)
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestServerServeHTTP_BridgePathRequiresAuthWhenEnabled(t *testing.T) {
	auth, _, _ := newAuthMiddlewareWithStore()
	server := NewServer(NewHandlers())
	server.SetAuthMiddleware(auth)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://bridge.local/bridge", nil)
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
