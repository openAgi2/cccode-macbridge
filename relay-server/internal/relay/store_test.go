package relay

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsMailboxAndAcknowledgement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.db")
	now := time.Unix(1_700_000_000, 0)
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	routeID, bridgeAuth, err := store.CreateRoute(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	deviceAuth, err := store.RegisterDevice(context.Background(), routeID, "phone-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendFrame(context.Background(), routeID, "phone-1", []byte(`{"ciphertext":"opaque"}`), now, time.Hour, 1024); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if !store.AuthenticateBridge(context.Background(), routeID, bridgeAuth, now) ||
		!store.AuthenticateDevice(context.Background(), routeID, "phone-1", deviceAuth, now) {
		t.Fatal("credentials did not survive database reopen")
	}
	frames, err := store.FetchFrames(context.Background(), routeID, "phone-1", 0, 10, now)
	if err != nil || len(frames) != 1 || frames[0].Cursor != 1 {
		t.Fatalf("persisted frames = %+v, err = %v", frames, err)
	}
	if err := store.AckFrames(context.Background(), routeID, "phone-1", 1, now); err != nil {
		t.Fatal(err)
	}
	frames, err = store.FetchFrames(context.Background(), routeID, "phone-1", 0, 10, now)
	if err != nil || len(frames) != 0 {
		t.Fatalf("frames after ack = %+v, err = %v", frames, err)
	}
}

func TestStoreMailboxTTLCapacityAndRevocation(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	routeID, _, _ := store.CreateRoute(ctx, now)
	_, _ = store.RegisterDevice(ctx, routeID, "phone-1", now)
	if _, _, err := store.AppendFrame(ctx, routeID, "phone-1", []byte("1234"), now, time.Second, 7); err != nil {
		t.Fatal(err)
	}
	cursor, evicted, err := store.AppendFrame(ctx, routeID, "phone-1", []byte("5678"), now, time.Hour, 7)
	if err != nil || cursor != 2 || evicted != 1 {
		t.Fatalf("cursor=%d evicted=%d err=%v", cursor, evicted, err)
	}
	frames, err := store.FetchFrames(ctx, routeID, "phone-1", 0, 10, now)
	if err != nil || len(frames) != 1 || string(frames[0].Envelope) != "5678" {
		t.Fatalf("capacity frames=%+v err=%v", frames, err)
	}
	if _, _, err := store.AppendFrame(ctx, routeID, "phone-1", []byte("x"), now, time.Second, 7); err != nil {
		t.Fatal(err)
	}
	frames, err = store.FetchFrames(ctx, routeID, "phone-1", 0, 10, now.Add(2*time.Second))
	if err != nil || len(frames) != 1 {
		t.Fatalf("expired short frame should be removed while long frame remains: %+v err=%v", frames, err)
	}
	if err := store.RevokeDevice(ctx, routeID, "phone-1", now); err != nil {
		t.Fatal(err)
	}
	if store.DeviceActive(ctx, routeID, "phone-1") {
		t.Fatal("revoked device remains active")
	}
	frames, err = store.FetchFrames(ctx, routeID, "phone-1", 0, 10, now)
	if err != nil || len(frames) != 0 {
		t.Fatalf("revoked mailbox was not cleared: %+v err=%v", frames, err)
	}
}

func TestStoreActivationSurvivesReopenAndRotatesCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.db")
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	publicKey := []byte("activation-public-key-material")

	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	routeID, err := store.ActivateRoute(ctx, "install_fresh", publicKey, "credential_first", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	recoveredRouteID, err := store.ActivateRoute(ctx, "install_fresh", publicKey, "credential_recovered", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if recoveredRouteID != routeID {
		t.Fatalf("recovered activation route=%s, want %s", recoveredRouteID, routeID)
	}
	if store.AuthenticateBridge(ctx, routeID, "credential_first", now) {
		t.Fatal("old credential still authenticates after signed recovery")
	}
	if !store.AuthenticateBridge(ctx, routeID, "credential_recovered", now) {
		t.Fatal("recovered credential does not authenticate after reopen")
	}
}
