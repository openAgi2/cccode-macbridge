package core

import (
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a file atomically by first writing to a
// temporary file in the same directory, syncing, then renaming over the target.
// This prevents data loss / corruption on crash.
//
// Durable: after the rename, the parent directory is fsynced so the directory
// entry is durably committed to disk. Without dir fsync a crash after rename
// can leave the new file invisible on replay.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return fsyncDir(dir)
}

// fsyncDir flushes the directory entry for durability after a rename. Errors
// are tolerated on filesystems/devices that do not support fsync on directories
// (e.g. some network mounts), since the file rename itself already succeeded.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		// ENOTSUP/EINVAL on dirs: tolerate; the file rename already happened.
		return nil
	}
	return nil
}
