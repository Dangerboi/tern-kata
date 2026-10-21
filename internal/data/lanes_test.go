package data

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestLanesFileKeepsDoneLimitsAndDropsAnAllDefaultProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	path, err := lanesPath()
	if err != nil {
		t.Fatal(err)
	}
	if got := saveLanes(model.SaveLanes{ProjectUID: "p", ProjectName: "work", DoneDays: 30, DoneCards: 5}); got.Err != nil {
		t.Fatal(got.Err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "labels") || !strings.Contains(string(raw), "done_days = 30") || !strings.Contains(string(raw), "done_cards = 5") {
		t.Errorf("file:\n%s", raw)
	}
	want := model.LanesLoaded{ProjectUID: "p", DoneDays: 30, DoneCards: 5}
	if got := loadLanes(model.LoadLanes{ProjectUID: "p"}); !reflect.DeepEqual(got, want) {
		t.Errorf("loaded %#v, want %#v", got, want)
	}
	if got := saveLanes(model.SaveLanes{ProjectUID: "p", ProjectName: "work"}); got.Err != nil {
		t.Fatal(got.Err)
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), `"p"`) {
		t.Errorf("an all-default Project kept its table:\n%s", raw)
	}
}
