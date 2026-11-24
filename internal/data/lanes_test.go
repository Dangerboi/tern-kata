package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (f configFixture) loadLanes(t *testing.T, gateway *Gateway, uid string) model.LanesLoaded {
	t.Helper()
	return as[model.LanesLoaded](t, run(t, gateway, model.LoadLanes{ProjectUID: uid}))
}

func TestSavedLanesAndDoneLimitsComeBackOnAFreshGateway(t *testing.T) {
	f := newConfigFixture(t)
	saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"needs-triage", "in-review"}, DoneDays: 14, DoneCards: 20}))
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	got := f.loadLanes(t, f.again(), "p")
	if got.Err != nil || !slices.Equal(got.Lanes, []string{"needs-triage", "in-review"}) || got.DoneDays != 14 || got.DoneCards != 20 {
		t.Errorf("loaded %+v", got)
	}
	if none := f.loadLanes(t, f.again(), "unsaved"); none.Err != nil || len(none.Lanes) != 0 {
		t.Errorf("a Project with no list loaded %+v", none)
	}
}

func TestLoadedLaneLabelsAreNormalisedAndRepeatedOrEmptyOnesDropped(t *testing.T) {
	f := newConfigFixture(t)
	f.write(t, "lanes.toml", "[\"p\"]\nname = \"work\"\nlabels = [\"  Needs-Triage \", \"needs-triage\", \"\", \"in-review\"]\n")
	got := f.loadLanes(t, f.gateway, "p")
	if got.Err != nil || !slices.Equal(got.Lanes, []string{"needs-triage", "in-review"}) {
		t.Errorf("loaded %+v", got)
	}
}

func TestSavingLanesKeepsOtherTablesAndUnknownKeys(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "[\"other\"]\nname = \"elsewhere\"\nlabels = [\"a\"]\n\n[\"p\"]\nname = \"old\"\nnote = \"mine\"\n")
	if saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"b"}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"[other]", `name = "elsewhere"`, `note = "mine"`, `name = "work"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file lacks %q:\n%s", want, raw)
		}
	}
	if other := f.loadLanes(t, f.again(), "other"); !slices.Equal(other.Lanes, []string{"a"}) {
		t.Errorf("the other Project's list changed: %+v", other)
	}
}

func TestAnAllDefaultLanesSaveRemovesAnOtherwiseEmptyTable(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "[\"p\"]\nname = \"work\"\nlabels = [\"a\"]\ndone_days = 3\n\n[\"q\"]\nname = \"w\"\nlabels = [\"a\"]\nnote = \"keep\"\n")
	for _, uid := range []string{"p", "q"} {
		if saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: uid, ProjectName: "w"})); saved.Err != nil {
			t.Fatal(saved.Err)
		}
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "[p]") || strings.Contains(string(raw), "labels") || !strings.Contains(string(raw), "[q]") || !strings.Contains(string(raw), `note = "keep"`) {
		t.Errorf("file:\n%s", raw)
	}
}

func TestAnUnreadableLanesFileIsRefusedAndNotOverwritten(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "this is [not toml")
	saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"a"}}))
	if saved.Err == nil || !strings.Contains(saved.Err.Error(), "lanes.toml") {
		t.Fatalf("error %v, want one naming the file", saved.Err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "this is [not toml" {
		t.Errorf("file overwritten: %q", raw)
	}
	if got := f.loadLanes(t, f.again(), "p"); got.Err == nil {
		t.Error("loading an unreadable file gave no error")
	}
}

func TestLanesSaveAndLoadWhenOnlyTheStateFolderCannotBeFound(t *testing.T) {
	config := t.TempDir()
	loc := newLocator(startKata(t, "0.21.0").target())
	newOne := func() *Gateway {
		g := NewGateway(loc.locate)
		g.workerDirs = func() (worker.Dirs, error) {
			return worker.Dirs{Config: config}, errors.New("no state folder")
		}
		t.Cleanup(g.Close)
		return g
	}
	if saved := as[model.LanesSaved](t, run(t, newOne(), model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"a"}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if got := as[model.LanesLoaded](t, run(t, newOne(), model.LoadLanes{ProjectUID: "p"})); got.Err != nil || !slices.Equal(got.Lanes, []string{"a"}) {
		t.Errorf("loaded %+v", got)
	}
}

func TestASaveThatCannotCompleteLeavesTheLaneListAndDefaultFilterFilesAsTheyWere(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a folder this user cannot write to")
	}
	f := newConfigFixture(t)
	lanes := f.write(t, "lanes.toml", "[\"p\"]\nname = \"old\"\nlabels = [\"a\"]\n")
	filters := f.write(t, "filters.toml", "[\"p\"]\nname = \"old\"\nstatus = \"open\"\n")
	if err := os.Chmod(f.folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.folder, 0o755) })
	savedLanes := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: "p", ProjectName: "new", Lanes: []string{"b"}}))
	savedFilters := f.saveFilters(t, "p", "new", model.Filters{Status: "closed"})
	if savedLanes.Err == nil || savedFilters.Err == nil {
		t.Fatalf("errors: lanes %v, filters %v", savedLanes.Err, savedFilters.Err)
	}
	for path, want := range map[string]string{lanes: "[\"p\"]\nname = \"old\"\nlabels = [\"a\"]\n", filters: "[\"p\"]\nname = \"old\"\nstatus = \"open\"\n"} {
		if raw, _ := os.ReadFile(path); string(raw) != want {
			t.Errorf("%s holds %q", path, raw)
		}
	}
	if left, _ := os.ReadDir(f.folder); len(left) != 2 {
		t.Errorf("stray files %v", left)
	}
}

func (f configFixture) lanes(t *testing.T, gateway *Gateway) model.LanesLoaded {
	t.Helper()
	return as[model.LanesLoaded](t, run(t, gateway, model.LoadLanes{Workspace: f.workspace, ProjectUID: f.project.UID}))
}

// share is the Lanes card's shared write through gateway, guarded on revision; stop clears the shared list.
func (f configFixture) share(t *testing.T, gateway *Gateway, revision int64, stop bool, lanes ...string) model.LanesShared {
	t.Helper()
	return as[model.LanesShared](t, run(t, gateway, model.ShareLanes{Workspace: f.workspace, Actor: "tester", ProjectUID: f.project.UID,
		ProjectID: f.project.ID, Revision: revision, Lanes: lanes, Stop: stop}))
}

// setProjectMeta is another client patching the Project's metadata, unguarded.
func (f configFixture) setProjectMeta(t *testing.T, patch map[string]any) {
	t.Helper()
	_, err := f.kata.target().Client.PatchProjectMetadataWithResponse(context.Background(), &g.PatchProjectMetadataRequestOptions{
		PathParams: &g.PatchProjectMetadataPath{ProjectID: f.project.ID},
		Body:       &g.PatchProjectMetadataBody{Actor: "rival", Patch: patch},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f configFixture) writeOwnList(t *testing.T) {
	t.Helper()
	f.write(t, "lanes.toml", fmt.Sprintf("[%q]\nname = \"work\"\nlabels = [\"mine\"]\ndone_days = 3\n", f.project.UID))
}

func TestTheSharedListComesBeforeTheFileAndTheFileBeforeTheDefault(t *testing.T) {
	f := newConfigFixture(t)
	none := f.lanes(t, f.gateway)
	if none.Err != nil || none.SharedErr != nil || none.Shared || len(none.Lanes) != 0 || none.ProjectID != f.project.ID {
		t.Fatalf("with neither %+v, want the default and the Project read", none)
	}
	f.writeOwnList(t)
	local := f.lanes(t, f.gateway)
	if local.Shared || !slices.Equal(local.Lanes, []string{"mine"}) || local.DoneDays != 3 {
		t.Fatalf("with the file only %+v", local)
	}
	shared := f.share(t, f.gateway, local.Revision, false, "triage", "review")
	if shared.Err != nil || shared.Conflict || shared.Revision <= local.Revision {
		t.Fatalf("share %+v", shared)
	}
	got := f.lanes(t, f.gateway)
	if !got.Shared || !slices.Equal(got.Lanes, []string{"triage", "review"}) || got.DoneDays != 0 || got.Revision != shared.Revision {
		t.Fatalf("with both %+v, want the shared list and its default Done limits", got)
	}
	// Another machine on the same daemon, with no lanes.toml of its own.
	elsewhere, folder := f.again(), t.TempDir()
	elsewhere.workerDirs = func() (worker.Dirs, error) { return worker.Dirs{Config: folder, State: folder}, nil }
	if other := f.lanes(t, elsewhere); !other.Shared || !slices.Equal(other.Lanes, []string{"triage", "review"}) {
		t.Errorf("another machine read %+v", other)
	}
}

func TestASharedValueThatIsNotALaneListIsReportedAndTheFileUsed(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  string
	}{
		{"not an object", "triage", "not an object"},
		{"a label that is no label", map[string]any{"labels": []any{"ok", "!!!"}}, `"!!!"`},
		{"a repeated label", map[string]any{"labels": []any{"ok", "OK"}}, "twice"},
		{"a zero limit", map[string]any{"labels": []any{"ok"}, "done_days": 0}, "done_days"},
		{"a fractional limit", map[string]any{"done_cards": 2.5}, "done_cards"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newConfigFixture(t)
			f.writeOwnList(t)
			f.setProjectMeta(t, map[string]any{model.SharedLanesKey: tt.value})
			got := f.lanes(t, f.gateway)
			if got.Shared || got.SharedErr == nil || !strings.Contains(got.SharedErr.Error(), tt.want) || !slices.Equal(got.Lanes, []string{"mine"}) {
				t.Fatalf("loaded %+v, want the file's list and an error naming %s", got, tt.want)
			}
		})
	}
}

func TestAnArchivedProjectsSharedListIsRead(t *testing.T) {
	f := newConfigFixture(t)
	opened := f.lanes(t, f.gateway)
	if shared := f.share(t, f.gateway, opened.Revision, false, "triage", "review"); shared.Err != nil || shared.Conflict {
		t.Fatalf("share %+v", shared)
	}
	if _, err := f.kata.target().Client.RemoveProjectWithResponse(context.Background(), &g.RemoveProjectRequestOptions{
		PathParams: &g.RemoveProjectPath{ProjectID: f.project.ID}, Query: &g.RemoveProjectQuery{Actor: "tester"},
	}); err != nil {
		t.Fatal(err)
	}
	got := f.lanes(t, f.gateway)
	if got.SharedErr != nil || !got.Shared || !slices.Equal(got.Lanes, []string{"triage", "review"}) || got.ProjectID != f.project.ID || !got.Archived {
		t.Fatalf("archived Project read %+v, want its shared list", got)
	}
	// kata refuses a write to an archived Project; the card shows its reason, not that the Project could not be read.
	if again := f.share(t, f.gateway, got.Revision, false, "mine"); again.Err == nil || again.Conflict || again.Current.ProjectUID != "" {
		t.Fatalf("share on the archived Project %+v, want kata's refusal", again)
	}
}

func TestASharedWriteOnAStaleRevisionAnswersWithTheListAsItStands(t *testing.T) {
	f := newConfigFixture(t)
	opened := f.lanes(t, f.gateway)
	rival := f.share(t, f.again(), opened.Revision, false, "theirs")
	if rival.Err != nil || rival.Conflict {
		t.Fatalf("rival %+v", rival)
	}
	mine := f.share(t, f.gateway, opened.Revision, false, "mine")
	if !mine.Conflict || mine.Err != nil || !mine.Current.Shared || !slices.Equal(mine.Current.Lanes, []string{"theirs"}) || mine.Current.Revision != rival.Revision {
		t.Fatalf("stale write %+v, want a conflict naming the rival's list at its revision %d", mine, rival.Revision)
	}
	if got := f.lanes(t, f.gateway); !slices.Equal(got.Lanes, []string{"theirs"}) {
		t.Fatalf("the stale write changed the list: %+v", got)
	}
	if again := f.share(t, f.gateway, mine.Current.Revision, false, "mine"); again.Err != nil || again.Conflict {
		t.Fatalf("retry on the current revision %+v", again)
	}
	if got := f.lanes(t, f.gateway); !slices.Equal(got.Lanes, []string{"mine"}) {
		t.Errorf("after the retry %+v", got)
	}
}

func TestStopSharingClearsTheKeyAndTheFileListReturns(t *testing.T) {
	f := newConfigFixture(t)
	f.writeOwnList(t)
	shared := f.share(t, f.gateway, f.lanes(t, f.gateway).Revision, false, "ours")
	if shared.Err != nil {
		t.Fatal(shared.Err)
	}
	if stopped := f.share(t, f.gateway, shared.Revision, true); stopped.Err != nil || stopped.Conflict {
		t.Fatalf("stop %+v", stopped)
	}
	got := f.lanes(t, f.gateway)
	if got.Shared || got.SharedErr != nil || !slices.Equal(got.Lanes, []string{"mine"}) {
		t.Fatalf("after stopping %+v, want the file's list", got)
	}
	shown, err := f.kata.target().Client.ShowProjectWithResponse(context.Background(), &g.ShowProjectRequestOptions{PathParams: &g.ShowProjectPath{ProjectID: f.project.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, held := shown.JSON200.Project.Metadata[model.SharedLanesKey]; held {
		t.Errorf("the Project still holds %s: %v", model.SharedLanesKey, shown.JSON200.Project.Metadata)
	}
}

func TestARefusedSharedWriteShowsKatasReasonAndTheFileStillSaves(t *testing.T) {
	f := newConfigFixture(t)
	metadata := "/api/v1/projects/" + strconv.FormatInt(f.project.ID, 10) + "/metadata"
	refuse := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || r.URL.Path != metadata {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"error":{"code":"forbidden","message":"this token is read-only"}}`))
		return true
	}
	f.kata.intercept.Store(&refuse)
	refused := f.share(t, f.gateway, f.lanes(t, f.gateway).Revision, false, "ours")
	if refused.Err == nil || !strings.Contains(refused.Err.Error(), "this token is read-only") || refused.Conflict {
		t.Fatalf("refused write %+v, want kata's reason", refused)
	}
	if saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: f.project.UID, ProjectName: "work", Lanes: []string{"mine"}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if got := f.lanes(t, f.gateway); got.Shared || !slices.Equal(got.Lanes, []string{"mine"}) {
		t.Errorf("after the local save %+v", got)
	}
}

func TestConcurrentLaneSavesOfDifferentProjectsKeepEach(t *testing.T) {
	f := newConfigFixture(t)
	const panes = 24
	var wg sync.WaitGroup
	for i := range panes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: fmt.Sprintf("p%02d", i), ProjectName: "work", Lanes: []string{fmt.Sprintf("l%02d", i)}}))
			if saved.Err != nil {
				t.Error(saved.Err)
			}
		}()
	}
	wg.Wait()
	for i := range panes {
		uid := fmt.Sprintf("p%02d", i)
		if got := f.loadLanes(t, f.again(), uid); got.Err != nil || !slices.Equal(got.Lanes, []string{fmt.Sprintf("l%02d", i)}) {
			t.Errorf("%s: loaded %+v", uid, got)
		}
	}
}

// saveOrder is the Board's drag write through gateway for the fixture's Project, setting the Lane's order to order on a Lane that
// has none; no order drops it.
func (f configFixture) saveOrder(t *testing.T, gateway *Gateway, shared bool, lane string, order ...string) model.LaneOrderSaved {
	t.Helper()
	if len(order) == 0 {
		return f.orderChange(t, gateway, shared, lane, model.OrderChange{Reset: true})
	}
	return f.orderChange(t, gateway, shared, lane, model.OrderChange{Shown: order})
}

func (f configFixture) orderChange(t *testing.T, gateway *Gateway, shared bool, lane string, change model.OrderChange) model.LaneOrderSaved {
	t.Helper()
	return as[model.LaneOrderSaved](t, run(t, gateway, model.SaveLaneOrder{Workspace: f.workspace, Actor: "tester", ProjectUID: f.project.UID,
		ProjectName: "work", ProjectID: f.project.ID, Shared: shared, Lane: lane, Change: change}))
}

func (f configFixture) projectMeta(t *testing.T) map[string]any {
	t.Helper()
	shown, err := f.kata.target().Client.ShowProjectWithResponse(context.Background(), &g.ShowProjectRequestOptions{PathParams: &g.ShowProjectPath{ProjectID: f.project.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return shown.JSON200.Project.Metadata
}

func TestAFileOrderRoundTripsAndKeepsOtherTablesAndUnknownKeys(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", fmt.Sprintf("[\"other\"]\nname = \"elsewhere\"\nlabels = [\"a\"]\n\n[%q]\nname = \"old\"\nnote = \"mine\"\n", f.project.UID))
	saved := f.saveOrder(t, f.gateway, false, "triage", "b", "a")
	if saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"triage": {"b", "a"}}) {
		t.Fatalf("saved %+v", saved)
	}
	if second := f.saveOrder(t, f.gateway, false, "review", "z"); second.Err != nil || len(second.Current) != 2 {
		t.Fatalf("second %+v", second)
	}
	got := f.lanes(t, f.again())
	if got.Err != nil || !reflect.DeepEqual(got.Order, model.LaneOrders{"triage": {"b", "a"}, "review": {"z"}}) {
		t.Errorf("loaded %+v", got)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"[other]", `name = "elsewhere"`, `note = "mine"`, `name = "old"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file lacks %q:\n%s", want, raw)
		}
	}
	if other := f.loadLanes(t, f.again(), "other"); !slices.Equal(other.Lanes, []string{"a"}) || other.Order != nil {
		t.Errorf("the other Project changed: %+v", other)
	}
}

func TestAFileOrderForANewProjectIsNamedAndEmptyingItRemovesTheTable(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "[\"other\"]\nname = \"elsewhere\"\n")
	if saved := f.saveOrder(t, f.gateway, false, "triage", "a"); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), `name = "work"`) || !strings.Contains(string(raw), "order") {
		t.Fatalf("file:\n%s", raw)
	}
	emptied := f.saveOrder(t, f.gateway, false, "triage")
	if emptied.Err != nil || len(emptied.Current) != 0 {
		t.Fatalf("emptied %+v", emptied)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), f.project.UID) || strings.Contains(string(raw), "order") || !strings.Contains(string(raw), "[other]") {
		t.Errorf("file:\n%s", raw)
	}
	// With a Lane list in the table, only the order goes.
	f.writeOwnList(t)
	f.saveOrder(t, f.gateway, false, "mine", "a")
	f.saveOrder(t, f.gateway, false, "mine")
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "order") || !strings.Contains(string(raw), `labels = ["mine"]`) {
		t.Errorf("file:\n%s", raw)
	}
}

func TestTheLanesCardSaveKeepsAnOrder(t *testing.T) {
	f := newConfigFixture(t)
	f.saveOrder(t, f.gateway, false, "triage", "b", "a")
	for _, lanes := range [][]string{{"triage", "review"}, nil} {
		saved := as[model.LanesSaved](t, run(t, f.gateway, model.SaveLanes{ProjectUID: f.project.UID, ProjectName: "work", Lanes: lanes}))
		if saved.Err != nil {
			t.Fatal(saved.Err)
		}
		got := f.lanes(t, f.again())
		if !reflect.DeepEqual(got.Order, model.LaneOrders{"triage": {"b", "a"}}) || !slices.Equal(got.Lanes, lanes) {
			t.Errorf("after saving lanes %v: %+v", lanes, got)
		}
	}
}

func TestABadFileOrderLosesOnlyWhatDoesNotParse(t *testing.T) {
	f := newConfigFixture(t)
	f.write(t, "lanes.toml", fmt.Sprintf("[%q.order]\ntriage = [\"a\", \"b\", \"a\"]\n\"Bad Label\" = [\"x\"]\nreview = \"x\"\nempty = []\n", f.project.UID))
	if got := f.lanes(t, f.gateway); got.Err != nil || !reflect.DeepEqual(got.Order, model.LaneOrders{"triage": {"a", "b"}}) {
		t.Errorf("loaded %+v", got)
	}
}

func TestNoLanesOrderRoundTripsUnderTheEmptyKey(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "lanes.toml", "")
	saved := f.saveOrder(t, f.gateway, false, "", "b", "a")
	if saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"": {"b", "a"}}) {
		t.Fatalf("saved %+v", saved)
	}
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), `"" = ["b", "a"]`) {
		t.Errorf("file lacks the empty key:\n%s", raw)
	}
	if got := f.lanes(t, f.again()); got.Err != nil || !reflect.DeepEqual(got.Order, model.LaneOrders{"": {"b", "a"}}) {
		t.Errorf("loaded %+v", got)
	}
	f.writeOwnList(t)
	f.share(t, f.gateway, f.lanes(t, f.gateway).Revision, false, "triage")
	if shared := f.saveOrder(t, f.gateway, true, "", "z", "y"); shared.Err != nil || !reflect.DeepEqual(shared.Current, model.LaneOrders{"": {"z", "y"}}) {
		t.Fatalf("shared save %+v", shared)
	}
	if got := f.lanes(t, f.again()); !got.Shared || !reflect.DeepEqual(got.Order, model.LaneOrders{"": {"z", "y"}}) {
		t.Errorf("shared load %+v", got)
	}
	if _, held := f.projectMeta(t)[model.LaneOrderKey]; !held {
		t.Errorf("metadata lacks the order: %v", f.projectMeta(t))
	}
	f.saveOrder(t, f.gateway, true, "")
	if _, held := f.projectMeta(t)[model.LaneOrderKey]; held {
		t.Errorf("an emptied order stays in the metadata: %v", f.projectMeta(t))
	}
}

func TestASharedOrderRoundTripsApartFromTheSharedList(t *testing.T) {
	f := newConfigFixture(t)
	f.writeOwnList(t)
	f.saveOrder(t, f.gateway, false, "mine", "file")
	opened := f.lanes(t, f.gateway)
	if shared := f.share(t, f.gateway, opened.Revision, false, "triage", "review"); shared.Err != nil || shared.Conflict {
		t.Fatalf("share %+v", shared)
	}
	if got := f.lanes(t, f.gateway); !got.Shared || got.Order != nil {
		t.Fatalf("shared list without a shared order read %+v, want no order from the file", got)
	}
	saved := f.saveOrder(t, f.gateway, true, "triage", "b", "a")
	if saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"triage": {"b", "a"}}) {
		t.Fatalf("saved %+v", saved)
	}
	if second := f.saveOrder(t, f.gateway, true, "review", "z"); second.Err != nil || len(second.Current) != 2 {
		t.Fatalf("second %+v", second)
	}
	got := f.lanes(t, f.again())
	if !got.Shared || !slices.Equal(got.Lanes, []string{"triage", "review"}) || !reflect.DeepEqual(got.Order, model.LaneOrders{"triage": {"b", "a"}, "review": {"z"}}) {
		t.Errorf("loaded %+v", got)
	}
	meta := f.projectMeta(t)
	if _, held := meta[model.SharedLanesKey]; !held || meta[model.LaneOrderKey] == nil {
		t.Errorf("metadata %v", meta)
	}
	if cleared := f.saveOrder(t, f.gateway, true, "triage"); cleared.Err != nil || !reflect.DeepEqual(cleared.Current, model.LaneOrders{"review": {"z"}}) {
		t.Errorf("cleared %+v", cleared)
	}
	f.saveOrder(t, f.gateway, true, "review")
	if _, held := f.projectMeta(t)[model.LaneOrderKey]; held {
		t.Errorf("an emptied order stays in the metadata: %v", f.projectMeta(t))
	}
	// Stopping the sharing returns the file's own order.
	if stopped := f.share(t, f.gateway, f.lanes(t, f.gateway).Revision, true); stopped.Err != nil {
		t.Fatal(stopped.Err)
	}
	if got := f.lanes(t, f.gateway); got.Shared || !reflect.DeepEqual(got.Order, model.LaneOrders{"mine": {"file"}}) {
		t.Errorf("after stopping %+v", got)
	}
}

func TestASharedOrderSaveAfterAnotherWriteBumpedTheRevisionStillLands(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage"}}})
	metadata := "/api/v1/projects/" + strconv.FormatInt(f.project.ID, 10) + "/metadata"
	var rivalled atomic.Bool
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == metadata && rivalled.CompareAndSwap(false, true) {
			f.setProjectMeta(t, map[string]any{model.LaneOrderKey: map[string]any{"other": []any{"x"}}})
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	saved := f.saveOrder(t, f.gateway, true, "triage", "a")
	f.kata.intercept.Store(nil)
	want := model.LaneOrders{"other": {"x"}, "triage": {"a"}}
	if saved.Err != nil || !reflect.DeepEqual(saved.Current, want) {
		t.Fatalf("saved %+v, want %v", saved, want)
	}
	if got := f.lanes(t, f.gateway); got.SharedErr != nil || !reflect.DeepEqual(got.Order, want) {
		t.Errorf("loaded %+v", got)
	}
}

func TestASharedOrderSaveThatKeepsConflictingOrIsRefusedSaysWhy(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage"}}, model.LaneOrderKey: map[string]any{"other": []any{"x"}}})
	metadata := "/api/v1/projects/" + strconv.FormatInt(f.project.ID, 10) + "/metadata"
	var bumps int
	var inside atomic.Bool
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		// Every guarded write meets a rival's write first; the rival's own request passes.
		if r.Method == http.MethodPost && r.URL.Path == metadata && !inside.Swap(true) {
			bumps++
			f.setProjectMeta(t, map[string]any{"rival": bumps})
			inside.Store(false)
		}
		return false
	}
	f.kata.intercept.Store(&hook)
	conflicted := f.saveOrder(t, f.gateway, true, "triage", "a")
	f.kata.intercept.Store(nil)
	if conflicted.Err == nil || !strings.Contains(conflicted.Err.Error(), "kept changing") || !reflect.DeepEqual(conflicted.Current, model.LaneOrders{"other": {"x"}}) {
		t.Fatalf("conflicted %+v", conflicted)
	}
	refuse := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || r.URL.Path != metadata {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"error":{"code":"forbidden","message":"this token is read-only"}}`))
		return true
	}
	f.kata.intercept.Store(&refuse)
	refused := f.saveOrder(t, f.gateway, true, "triage", "a")
	if refused.Err == nil || !strings.Contains(refused.Err.Error(), "this token is read-only") || !reflect.DeepEqual(refused.Current, model.LaneOrders{"other": {"x"}}) {
		t.Fatalf("refused %+v", refused)
	}
}

func TestASharedOrderThatIsNotAnObjectIsNoOrderAndNotASharedListProblem(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage"}}, model.LaneOrderKey: "triage"})
	got := f.lanes(t, f.gateway)
	if got.Err != nil || got.SharedErr != nil || !got.Shared || got.Order != nil || got.ProjectID != f.project.ID {
		t.Errorf("loaded %+v", got)
	}
	if saved := f.saveOrder(t, f.gateway, true, "triage", "a"); saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"triage": {"a"}}) {
		t.Errorf("a save over it %+v", saved)
	}
}

func TestASharedOrderWithOneBadLaneLoadsTheGoodLanesAndASaveKeepsThem(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{
		model.SharedLanesKey: map[string]any{"labels": []any{"triage", "needs-info"}},
		model.LaneOrderKey:   map[string]any{"needs-info": []any{"n1", "n2"}, "In Review": []any{"x"}, "triage": "x"},
	})
	want := model.LaneOrders{"needs-info": {"n1", "n2"}}
	if got := f.lanes(t, f.gateway); got.Err != nil || got.SharedErr != nil || !got.Shared || !reflect.DeepEqual(got.Order, want) {
		t.Fatalf("loaded %+v, want the good Lane and no SharedErr", got)
	}
	saved := f.saveOrder(t, f.gateway, true, "triage", "a")
	if saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"needs-info": {"n1", "n2"}, "triage": {"a"}}) {
		t.Fatalf("saved %+v", saved)
	}
	if got := f.lanes(t, f.again()); !reflect.DeepEqual(got.Order, saved.Current) {
		t.Errorf("stored %+v", got.Order)
	}
}

func TestASharedOrderSaveAfterSharingStoppedWritesNothingAndSaysSo(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{model.LaneOrderKey: map[string]any{"other": []any{"x"}}})
	before := f.projectMeta(t)
	saved := f.saveOrder(t, f.gateway, true, "triage", "a")
	if saved.Err == nil || !strings.Contains(saved.Err.Error(), "no longer shares a readable Lane list") || saved.Current != nil {
		t.Fatalf("saved %+v", saved)
	}
	if after := f.projectMeta(t); !reflect.DeepEqual(after, before) {
		t.Errorf("metadata changed from %v to %v", before, after)
	}
}

func TestAFailedFileOrderSaveAnswersTheStoredOrder(t *testing.T) {
	f := newConfigFixture(t)
	if first := f.saveOrder(t, f.gateway, false, "triage", "a", "b"); first.Err != nil {
		t.Fatal(first.Err)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if err := os.Chmod(f.folder, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(f.folder, 0o755) })
		failed := f.orderChange(t, f.gateway, false, "triage", model.OrderChange{Move: "b", Anchor: "a"})
		if failed.Err == nil || !reflect.DeepEqual(failed.Current, model.LaneOrders{"triage": {"a", "b"}}) {
			t.Errorf("failed write %+v, want the stored order", failed)
		}
		if err := os.Chmod(f.folder, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A file that cannot be parsed has no readable orders.
	f.write(t, "lanes.toml", "this is [not toml")
	if bad := f.saveOrder(t, f.gateway, false, "triage", "b"); bad.Err == nil || bad.Current != nil {
		t.Errorf("unreadable file %+v, want Err and no Current", bad)
	}
}

func TestAFailedSharedOrderSaveWhoseFirstReadFailsAnswersNoOrders(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage"}}})
	down := func(w http.ResponseWriter, r *http.Request) bool {
		w.WriteHeader(http.StatusBadGateway)
		return true
	}
	f.kata.intercept.Store(&down)
	saved := f.saveOrder(t, f.gateway, true, "triage", "a")
	f.kata.intercept.Store(nil)
	if saved.Err == nil || saved.Current != nil {
		t.Errorf("saved %+v, want Err and no Current", saved)
	}
}

func TestAFileOrderChangeThatLeavesTheLaneAsItIsWritesNothing(t *testing.T) {
	f := newConfigFixture(t)
	noops := []model.OrderChange{
		{Reset: true},
		{Remove: []string{"gone"}},
		{Shown: []string{"a", "b"}, Move: "a", Anchor: "b"},
	}
	// A file that holds nothing stays missing.
	path := f.folder + "/lanes.toml"
	if saved := f.orderChange(t, f.gateway, false, "triage", noops[0]); saved.Err != nil || len(saved.Current) != 0 {
		t.Fatalf("reset of nothing %+v", saved)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a reset of a Lane with no order created the file")
	}
	f.saveOrder(t, f.gateway, false, "triage", "a", "b")
	before, _ := os.ReadFile(path)
	for _, change := range noops[1:] {
		saved := f.orderChange(t, f.gateway, false, "triage", change)
		if saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"triage": {"a", "b"}}) {
			t.Errorf("%+v: saved %+v, want the stored order", change, saved)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("the file was rewritten:\n%s\nwas\n%s", after, before)
	}
}

func TestASharedNoOpChangeKeepsTheRevision(t *testing.T) {
	f := newConfigFixture(t)
	f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage"}}})
	f.saveOrder(t, f.gateway, true, "triage", "a", "b")
	revision := f.lanes(t, f.gateway).Revision
	for _, change := range []model.OrderChange{
		{Remove: []string{"gone"}},
		{Shown: []string{"a", "b"}, Move: "a", Anchor: "b"},
		{Shown: []string{"a"}},
	} {
		saved := f.orderChange(t, f.gateway, true, "triage", change)
		if saved.Err != nil || !reflect.DeepEqual(saved.Current, model.LaneOrders{"triage": {"a", "b"}}) {
			t.Errorf("%+v: saved %+v, want the stored order", change, saved)
		}
	}
	if got := f.lanes(t, f.gateway).Revision; got != revision {
		t.Errorf("revision %d after no-op changes, want %d", got, revision)
	}
	f.orderChange(t, f.gateway, true, "triage", model.OrderChange{Remove: []string{"a"}})
	if got := f.lanes(t, f.gateway).Revision; got == revision {
		t.Errorf("revision stayed %d after a real change", got)
	}
}

func TestAMoveLandsOnAnOrderAnotherPaneChangedAndKeepsItsWork(t *testing.T) {
	for name, shared := range map[string]bool{"file": false, "shared": true} {
		t.Run(name, func(t *testing.T) {
			f := newConfigFixture(t)
			if shared {
				f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage", "review"}}})
			}
			f.saveOrder(t, f.gateway, shared, "triage", "a", "b", "c")
			f.saveOrder(t, f.gateway, shared, "review", "r1", "r2")
			// Another Pane puts b first and drops r2 from review; this Pane still holds the first order.
			f.orderChange(t, f.again(), shared, "triage", model.OrderChange{Move: "b", Anchor: "a"})
			f.orderChange(t, f.again(), shared, "review", model.OrderChange{Remove: []string{"r2"}})
			saved := f.orderChange(t, f.gateway, shared, "triage", model.OrderChange{Shown: []string{"a", "b", "c"}, Move: "c", Anchor: "a"})
			want := model.LaneOrders{"triage": {"b", "c", "a"}, "review": {"r1"}}
			if saved.Err != nil || !reflect.DeepEqual(saved.Current, want) {
				t.Fatalf("saved %+v, want %v", saved, want)
			}
			if got := f.lanes(t, f.again()); !reflect.DeepEqual(got.Order, want) {
				t.Errorf("stored %v, want %v", got.Order, want)
			}
		})
	}
}

func TestADropChangeWritesNothingWhereAnotherPaneResetTheLane(t *testing.T) {
	for name, shared := range map[string]bool{"file": false, "shared": true} {
		t.Run(name, func(t *testing.T) {
			f := newConfigFixture(t)
			if shared {
				f.setProjectMeta(t, map[string]any{model.SharedLanesKey: map[string]any{"labels": []any{"triage", "review"}}})
			}
			f.saveOrder(t, f.gateway, shared, "triage", "a", "b")
			f.saveOrder(t, f.gateway, shared, "review", "r1")
			// Another Pane resets triage; this Pane still sees it as custom and drops a card of review into it.
			f.saveOrder(t, f.again(), shared, "triage")
			revision := f.lanes(t, f.gateway).Revision
			drop := model.OrderChange{Shown: []string{"a", "b", "r1"}, Move: "r1", Anchor: "a", OnlyOrdered: true}
			saved := f.orderChange(t, f.gateway, shared, "triage", drop)
			if want := (model.LaneOrders{"review": {"r1"}}); saved.Err != nil || !reflect.DeepEqual(saved.Current, want) {
				t.Fatalf("saved %+v, want only %v stored", saved, want)
			}
			if got := f.lanes(t, f.again()); !reflect.DeepEqual(got.Order, model.LaneOrders{"review": {"r1"}}) {
				t.Errorf("stored %v, the reset order came back", got.Order)
			}
			if shared {
				if got := f.lanes(t, f.gateway).Revision; got != revision {
					t.Errorf("revision %d after the refused drop, want %d", got, revision)
				}
			}
		})
	}
}
