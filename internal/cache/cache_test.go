package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steffaine/nb-connect/internal/netbox"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "services.json")
	store := Store{Path: path}
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if err := store.Write([]netbox.Service{{Device: "router", Name: "sshd"}}, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.SyncedAt.Equal(now) || len(snapshot.Services) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %o", info.Mode().Perm())
	}
}

func TestStoreReadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Path: path}).Read(); err == nil {
		t.Fatal("Read() error = nil")
	}
}

func TestDefaultPath(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cacheRoot, "nb-connect", "services.json")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}
