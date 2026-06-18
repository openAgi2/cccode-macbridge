package gobridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestFileDeviceStoreSaveFailureDoesNotCommitMemory 验证 P1-3：写盘失败时内存保持旧状态。
// 使目标路径的父目录不可写 → AtomicWriteFile 失败 → commit 不 swap 内存。
func TestFileDeviceStoreSaveFailureDoesNotCommitMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	store, err := NewFileDeviceStore(path)
	if err != nil {
		t.Fatalf("NewFileDeviceStore 失败: %v", err)
	}
	if err := store.AddDevice(makeTestRecord("dev1")); err != nil {
		t.Fatalf("AddDevice dev1 失败: %v", err)
	}
	before, _ := store.ListDevices()

	// 父目录设为只读：AtomicWriteFile 的 CreateTemp/Rename 将失败。
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir 失败: %v", err)
	}
	if os.Geteuid() == 0 {
		_ = os.Chmod(dir, 0o755)
		t.Skip("running as root: 目录权限不足以拒绝写，跳过")
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err = store.RevokeDevice("dev1")
	if err == nil {
		t.Fatal("RevokeDevice 在写盘失败时应返回错误")
	}

	after, _ := store.ListDevices()
	if len(after) != len(before) {
		t.Fatalf("写盘失败后内存设备数变化: before=%d after=%d", len(before), len(after))
	}
	got, _ := store.LookupByDeviceID("dev1")
	if got == nil {
		t.Fatal("dev1 应仍存在于内存")
	}
	if got.RevokedAt != nil {
		t.Fatal("写盘失败不应在内存中标记 dev1 为已吊销")
	}

	// 恢复权限后重新打开：磁盘上仍是未吊销状态。
	_ = os.Chmod(dir, 0o755)
	reopened, err := NewFileDeviceStore(path)
	if err != nil {
		t.Fatalf("重开 store 失败: %v", err)
	}
	rgot, _ := reopened.LookupByDeviceID("dev1")
	if rgot == nil || rgot.RevokedAt != nil {
		t.Fatalf("磁盘状态应保持未吊销: %#v", rgot)
	}
}

// TestFileDeviceStoreRevokedAtPointerIsDeepCopied 验证 P1-3 深拷贝陷阱：
// cloneSnapshot 必须独立复制 *time.Time，否则旧快照的 Revoke 会污染副本。
func TestFileDeviceStoreRevokedAtPointerIsDeepCopied(t *testing.T) {
	rec := makeTestRecord("dev1")
	mem := NewMemoryDeviceStore()
	_ = mem.AddDevice(rec)

	clone := mem.Clone()
	// 在 clone 上吊销
	if err := clone.RevokeDevice("dev1"); err != nil {
		t.Fatalf("clone.RevokeDevice 失败: %v", err)
	}
	cgot, _ := clone.LookupByDeviceID("dev1")
	if cgot == nil || cgot.RevokedAt == nil {
		t.Fatal("clone 上 dev1 应已吊销")
	}

	// 原 mem 不应被影响
	ogot, _ := mem.LookupByDeviceID("dev1")
	if ogot == nil {
		t.Fatal("原 mem dev1 应存在")
	}
	if ogot.RevokedAt != nil {
		t.Fatalf("深拷贝失败：原 mem 的 dev1 被 clone 的吊销污染: %v", ogot.RevokedAt)
	}
	_ = time.Now()
}

// TestFileDeviceStoreCorruptFileReopensClean 验证损坏文件重开行为：
// 损坏的 devices.json 应明确报错而非产生部分状态。
func TestFileDeviceStoreCorruptFileReopensClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileDeviceStore(path); err == nil {
		t.Fatal("损坏的 devices.json 应在 NewFileDeviceStore 时返回错误，不能静默接受")
	}
}

// TestFileDeviceStoreSuccessfulCommitPersistsAndSurvivesRestart 验证成功提交后磁盘与重开一致。
func TestFileDeviceStoreSuccessfulCommitPersistsAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	store, err := NewFileDeviceStore(path)
	if err != nil {
		t.Fatalf("NewFileDeviceStore 失败: %v", err)
	}
	rec := makeTestRecord("dev1")
	if err := store.AddDevice(rec); err != nil {
		t.Fatalf("AddDevice 失败: %v", err)
	}
	if err := store.EnableRelay("dev1", "identity-a", 2); err != nil {
		t.Fatalf("EnableRelay 失败: %v", err)
	}
	if err := store.RevokeDevice("dev1"); err != nil {
		t.Fatalf("RevokeDevice 失败: %v", err)
	}

	reopened, err := NewFileDeviceStore(path)
	if err != nil {
		t.Fatalf("重开失败: %v", err)
	}
	got, _ := reopened.LookupByDeviceID("dev1")
	if got == nil {
		t.Fatal("dev1 应在重开后存在（含已吊销）")
	}
	if got.RevokedAt == nil {
		t.Fatal("重开后 dev1 应为已吊销")
	}
	if !got.RelayEnabled || got.IdentityPublicKey != "identity-a" || got.RelayChannelGeneration != 2 {
		t.Fatalf("relay 绑定丢失: %#v", got)
	}
	// ListDevices 排除已吊销
	list, _ := reopened.ListDevices()
	if len(list) != 0 {
		t.Fatalf("已吊销设备应被 ListDevices 排除: %#v", list)
	}
}

// TestFileDeviceStoreConcurrentCommitsNoLostUpdate 验证 P1-3 并发 lost-update 修复：
// 多个并发提交（同时添加不同设备）必须全部落盘，不能因基于同一快照克隆而互相覆盖。
// 修复前：commit 无外层串行化，后 swap 者覆盖先写者，设备记录丢失。
// 修复后：commitMu 串行化整个 read-modify-write 序列。
func TestFileDeviceStoreConcurrentCommitsNoLostUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	store, err := NewFileDeviceStore(path)
	if err != nil {
		t.Fatalf("NewFileDeviceStore 失败: %v", err)
	}

	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = store.AddDevice(makeTestRecord(fmt.Sprintf("dev%d", i)))
		}()
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("并发 AddDevice dev%d 失败: %v", i, e)
		}
	}

	// 内存视图：n 个设备应全部可见。
	list, _ := store.ListDevices()
	if len(list) != n {
		t.Fatalf("内存设备数 = %d, want %d（存在并发 lost-update）", len(list), n)
	}

	// 重开后磁盘视图：n 个设备应全部持久化。
	reopened, err := NewFileDeviceStore(path)
	if err != nil {
		t.Fatalf("重开 store 失败: %v", err)
	}
	rlist := reopened.mem.allDevices()
	if len(rlist) != n {
		t.Fatalf("磁盘设备数 = %d, want %d（并发提交有记录未落盘）", len(rlist), n)
	}
}
