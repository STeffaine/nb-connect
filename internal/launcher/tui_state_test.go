package launcher

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLauncherStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "launcher-state.json")
	favorites := map[string]bool{"router-01::sshd::192.0.2.10:22": true}
	recents := []string{"router-01::sshd::192.0.2.10:22", "db-01::sshd::192.0.2.20:22"}
	if err := saveLauncherState(path, favorites, recents); err != nil {
		t.Fatal(err)
	}
	loadedFavorites, loadedRecents, err := loadLauncherState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loadedFavorites, favorites) || !reflect.DeepEqual(loadedRecents, recents) {
		t.Fatalf("loaded state = %#v, %#v", loadedFavorites, loadedRecents)
	}
}

func TestRecordSelectionDeduplicatesAndLimitsRecents(t *testing.T) {
	model := model{favorites: map[string]bool{}, statePath: ""}
	selections := make([]Selection, 0, 9)
	for index := range 9 {
		selection := Selection{Endpoint: "192.0.2." + string(rune('1'+index)) + ":22"}
		selection.Service.Device = "router-" + string(rune('1'+index))
		selection.Service.Name = "sshd"
		selections = append(selections, selection)
		model = model.recordSelection(selection)
	}
	model = model.recordSelection(selections[3])
	if len(model.recents) != 8 {
		t.Fatalf("recent count = %d, want 8", len(model.recents))
	}
	if got, want := model.recents[0], selectionKey(selections[3]); got != want {
		t.Fatalf("most recent key = %q, want %q", got, want)
	}
	seen := make(map[string]bool, len(model.recents))
	for _, key := range model.recents {
		if seen[key] {
			t.Fatalf("duplicate recent key %q", key)
		}
		seen[key] = true
	}
}
