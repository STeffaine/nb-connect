package launcher

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (model model) recordSelection(selection Selection) model {
	key := selectionKey(selection)
	if key == "" {
		return model
	}
	if model.favorites == nil {
		model.favorites = map[string]bool{}
	}
	model.recents = append([]string{key}, model.recents...)
	seen := make(map[string]struct{}, len(model.recents))
	filtered := make([]string, 0, len(model.recents))
	for _, recent := range model.recents {
		if recent == "" {
			continue
		}
		if _, exists := seen[recent]; exists {
			continue
		}
		seen[recent] = struct{}{}
		filtered = append(filtered, recent)
		if len(filtered) == 8 {
			break
		}
	}
	model.recents = filtered
	_ = saveLauncherState(model.statePath, model.favorites, model.recents)
	return model
}

func (model model) toggleFavorite(selection Selection) model {
	key := selectionKey(selection)
	if key == "" {
		return model
	}
	if model.favorites == nil {
		model.favorites = map[string]bool{}
	}
	if model.favorites[key] {
		delete(model.favorites, key)
	} else {
		model.favorites[key] = true
	}
	_ = saveLauncherState(model.statePath, model.favorites, model.recents)
	return model
}

func defaultLauncherStatePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cacheDir, "nb-connect", "launcher-state.json")
}

func loadLauncherState(path string) (map[string]bool, []string, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]bool{}, nil, nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bool{}, nil, nil
		}
		return nil, nil, err
	}
	var state struct {
		Favorites map[string]bool `json:"favorites"`
		Recents   []string        `json:"recents"`
	}
	if err := json.Unmarshal(contents, &state); err != nil {
		return nil, nil, err
	}
	if state.Favorites == nil {
		state.Favorites = map[string]bool{}
	}
	return state.Favorites, state.Recents, nil
}

func saveLauncherState(path string, favorites map[string]bool, recents []string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(struct {
		Favorites map[string]bool `json:"favorites"`
		Recents   []string        `json:"recents"`
	}{Favorites: favorites, Recents: recents}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o600)
}
