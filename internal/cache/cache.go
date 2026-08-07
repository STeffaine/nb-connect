package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steffaine/nb-connect/internal/netbox"
)

type Store struct {
	Path string
}

type Snapshot struct {
	SyncedAt time.Time          `json:"synced_at"`
	Services []netbox.Service   `json:"services"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(dir, "nb-connect", "services.json"), nil
}

func (store Store) Read() (Snapshot, error) {
	contents, err := os.ReadFile(store.Path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read cache %q: %w", store.Path, err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("parse cache %q: %w", store.Path, err)
	}
	return snapshot, nil
}

func (store Store) Write(services []netbox.Service, now time.Time) error {
	contents, err := json.MarshalIndent(Snapshot{SyncedAt: now.UTC(), Services: services}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(store.Path), ".services-*.json")
	if err != nil {
		return fmt.Errorf("create temporary cache file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary cache file: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write cache: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close cache: %w", err)
	}
	if err := os.Rename(temporaryName, store.Path); err != nil {
		return fmt.Errorf("replace cache: %w", err)
	}
	return nil
}