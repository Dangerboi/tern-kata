package driver_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// asked is the first command of type C among cmds.
func asked[C model.Cmd](cmds []model.Cmd) (C, bool) {
	for _, cmd := range cmds {
		if c, ok := cmd.(C); ok {
			return c, true
		}
	}
	var none C
	return none, false
}

// shownText is the text of every node the view draws, one per line and with no JSON escaping, so a quoted name reads as typed.
func shownText(t *testing.T, s driver.State) string {
	t.Helper()
	var lines []string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if text, ok := n.P["text"].(string); ok {
			lines = append(lines, text)
		}
		if head, ok := n.P["head"].(string); ok {
			lines = append(lines, head)
		}
		if raw, err := json.Marshal(n.P["spans"]); err == nil {
			var spans []struct {
				T string `json:"t"`
			}
			if json.Unmarshal(raw, &spans) == nil {
				for _, span := range spans {
					lines = append(lines, span.T)
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func wantShown(t *testing.T, what string, s driver.State, want ...string) {
	t.Helper()
	text := shownText(t, s)
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, text)
		}
	}
}

func wantNotShown(t *testing.T, what string, s driver.State, unwanted ...string) {
	t.Helper()
	text := shownText(t, s)
	for _, u := range unwanted {
		if strings.Contains(text, u) {
			t.Errorf("%s shows %q:\n%s", what, u, text)
		}
	}
}

// enrollPane is the Federation Page of a Pane on work (id 7), acting as dana, beside acme (3), Zeta (9, a spoke), hubby (11, a hub
// Project) and an archived old; the status read bound Zeta as a spoke and hubby as a hub.
func enrollPane(t *testing.T) driver.State {
	t.Helper()
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	catalog := []model.CatalogProject{
		{Project: model.Project{ID: 7, UID: "P-WORK", Name: "work"}},
		{Project: model.Project{ID: 3, UID: "P-ACME", Name: "acme"}},
		{Project: model.Project{ID: 9, UID: "P-ZETA", Name: "Zeta"}},
		{Project: model.Project{ID: 11, UID: "P-HUBBY", Name: "hubby"}},
		{Project: model.Project{ID: 5, UID: "P-OLD", Name: "old", Archived: true}},
	}
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "P-WORK", Name: "work"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{boardCard("a")}, Catalog: catalog}, LocalDate: boardToday, TimeZone: "UTC"})
	s = feed(t, s).State
	out := federationOpened(t, s)
	load, _ := askedFederation(out.Cmds)
	zeta := healthySpoke("Zeta")
	zeta.ProjectID = 9
	answer := federationAnswer(load, zeta)
	answer.Bindings = []model.FederationBinding{{ProjectID: 9, ProjectName: "Zeta", Role: "spoke"}, {ProjectID: 11, ProjectName: "hubby", Role: "hub"}}
	s = step(t, out.State, answer).State
	s.Model.Actor = "dana"
	return s
}

// The local Project step lists, after the new replica, the Projects by name without the archived one: acme, hubby, work, Zeta.
// work is where the cursor starts.
const (
	rowReplica = 0
	rowAcme    = 1
	rowHubby   = 2
	rowWork    = 3
	rowZeta    = 4
)

// cursorOn presses g and then j until the cursor of the choice screen is on row.
func cursorOn(t *testing.T, s driver.State, row int) driver.State {
	t.Helper()
	s = step(t, s, key("g")).State
	for range row {
		s = step(t, s, key("j")).State
	}
	if got := s.Model.FederationChoiceCursor(); got != row {
		t.Fatalf("cursor on %d, want %d", got, row)
	}
	return s
}

func hubsAnswer(load model.LoadFederationHubs) model.FederationHubsLoaded {
	return model.FederationHubsLoaded{Request: load.Request, Hubs: []model.FederationHub{
		{Name: "work", Endpoint: "https://spoke.example:7474", Auth: "token", Active: true},
		{Name: "laptop", Endpoint: "local", Auth: "no token", Local: true},
		{Name: "hub", Endpoint: "https://hub.example:7474", Auth: "token", AllowInsecure: true},
	}}
}

func hubProjectsAnswer(load model.LoadFederationHubProjects, projects ...model.Project) model.FederationHubProjectsLoaded {
	return model.FederationHubProjectsLoaded{Request: load.Request, Auth: "token actor erin", Actor: "erin", Projects: projects,
		Hub: model.FederationHub{Name: load.Hub, Endpoint: "https://hub.example:7474", Auth: "token", AllowInsecure: true}}
}

// atHubStep is the enrollment after n, the local Project at row and the hub step answered with the catalog.
func atHubStep(t *testing.T, s driver.State, row int) driver.State {
	t.Helper()
	s = step(t, s, key("n")).State
	out := step(t, cursorOn(t, s, row), key("enter"))
	load, ok := asked[model.LoadFederationHubs](out.Cmds)
	if !ok || load.Workspace != "/repo" || out.State.Model.Federation.Screen != model.FederationPickHub {
		t.Fatalf("Enter on a local Project gave screen %v and %#v, want the hub step and a read of the catalog", out.State.Model.Federation.Screen, out.Cmds)
	}
	return step(t, out.State, hubsAnswer(load)).State
}

// atHubProjectStep is atHubStep with the hub chosen (the third row) and its Projects answered.
func atHubProjectStep(t *testing.T, s driver.State, row int, projects ...model.Project) driver.State {
	t.Helper()
	s = cursorOn(t, atHubStep(t, s, row), 2)
	out := step(t, s, key("enter"))
	load, ok := asked[model.LoadFederationHubProjects](out.Cmds)
	if !ok || load.Hub != "hub" || load.Workspace != "/repo" || out.State.Model.Federation.Screen != model.FederationPickHubProject {
		t.Fatalf("Enter on the hub gave screen %v and %#v, want the hub Project step and a read of the hub", out.State.Model.Federation.Screen, out.Cmds)
	}
	return step(t, out.State, hubProjectsAnswer(load, projects...)).State
}

var (
	hubBeta  = model.Project{ID: 2, UID: "H-BETA", Name: "beta"}
	hubAcme  = model.Project{ID: 1, UID: "H-ACME", Name: "acme"}
	hubHubby = model.Project{ID: 4, UID: "P-HUBBY", Name: "hubby"}
)

func TestNOpensTheLocalProjectStepWithTheNewReplicaFirstAndThePanesProjectSelected(t *testing.T) {
	s := enrollPane(t)
	out := step(t, s, key("n"))
	m := out.State.Model
	if m.Federation.Screen != model.FederationPickLocal || len(out.Cmds) != 0 || m.CurrentMode() != model.ModeFederationPick {
		t.Fatalf("n gave screen %v, mode %q and %#v, want the local Project step and no daemon call", m.Federation.Screen, m.CurrentMode(), out.Cmds)
	}
	text := shownText(t, out.State)
	wantShown(t, "the local Project step", out.State, "Select local spoke project", "selected project: work", "create new local replica from hub project",
		"Federation for active daemon: work https://spoke.example:7474")
	order := []string{"create new local replica from hub project", "acme", "hubby", "work", "Zeta"}
	pos := 0
	for _, name := range order {
		at := strings.Index(text[pos:], "\n"+name)
		if at < 0 {
			t.Fatalf("the step lacks %q after the rows before it:\n%s", name, text)
		}
		pos += at + 1 + len(name)
	}
	if strings.Contains(text, "\nold") {
		t.Error("the step lists an archived Project")
	}
	if sel, total := m.Position(); sel != rowWork+1 || total != 5 {
		t.Errorf("the cursor is on row %d of %d, want the Pane's own Project, work, of the 5 rows", sel, total)
	}
	wantText(t, "the footer", footerText(t, out.State), "↵", "select", "back")

	if moved := step(t, out.State, key("j")).State.Model.FederationChoiceCursor(); moved != rowZeta {
		t.Errorf("j left the cursor on %d", moved)
	}
	if moved := step(t, out.State, key("k")).State.Model.FederationChoiceCursor(); moved != rowHubby {
		t.Errorf("k left the cursor on %d", moved)
	}
	if first := step(t, out.State, key("g")).State.Model.FederationChoiceCursor(); first != rowReplica {
		t.Errorf("g left the cursor on %d", first)
	}
	if last := step(t, out.State, key("shift+g")).State.Model.FederationChoiceCursor(); last != rowZeta {
		t.Errorf("G left the cursor on %d", last)
	}
}

func TestEscAndBackspaceLeaveEachEnrollmentStepForTheOneBefore(t *testing.T) {
	for _, back := range []string{"escape", "backspace"} {
		s := atHubProjectStep(t, enrollPane(t), rowAcme, hubBeta)
		want := []model.FederationScreen{model.FederationPickHub, model.FederationPickLocal, model.FederationList}
		for _, screen := range want {
			s = step(t, s, key(back)).State
			if s.Model.Federation.Screen != screen {
				t.Fatalf("%s gave screen %v, want %v", back, s.Model.Federation.Screen, screen)
			}
		}
		if s.Model.CurrentMode() != model.ModeFederation || s.Model.Page != model.PageFederation {
			t.Errorf("%s back at the list: mode %q on page %v", back, s.Model.CurrentMode(), s.Model.Page)
		}
	}
}

func TestEnterOnALocalProjectReadsTheCatalogAndTheHubStepRefusesTheActiveAndLocalDaemons(t *testing.T) {
	s := step(t, enrollPane(t), key("n")).State
	out := step(t, cursorOn(t, s, rowAcme), key("enter"))
	if _, ok := asked[model.LoadFederationHubs](out.Cmds); !ok {
		t.Fatalf("Enter asked %#v, want a read of the catalog", out.Cmds)
	}
	wantShown(t, "the hub step while the catalog is read", out.State, "Select hub daemon", "local spoke project: acme", "loading hub daemons...")
	load, _ := asked[model.LoadFederationHubs](out.Cmds)
	hubs := step(t, out.State, hubsAnswer(load)).State
	wantShown(t, "the hub step", hubs, "work https://spoke.example:7474 auth token (active spoke)", "laptop local auth no token",
		"hub https://hub.example:7474 auth token allow_insecure")

	refused := step(t, hubs, key("enter"))
	if len(refused.Cmds) != 0 || refused.State.Model.Federation.Screen != model.FederationPickHub {
		t.Fatalf("Enter on the active daemon gave screen %v and %#v", refused.State.Model.Federation.Screen, refused.Cmds)
	}
	wantShown(t, "the active daemon's refusal", refused.State, "active daemon cannot be selected as hub")

	local := step(t, step(t, hubs, key("j")).State, key("enter"))
	if len(local.Cmds) != 0 || local.State.Model.Federation.Screen != model.FederationPickHub {
		t.Fatalf("Enter on a local entry gave screen %v and %#v", local.State.Model.Federation.Screen, local.Cmds)
	}
	wantShown(t, "the local entry's refusal", local.State, "local hub targets cannot be used for federation enrollment; select a hub daemon with a spoke-reachable URL")

	chosen := step(t, cursorOn(t, hubs, 2), key("enter"))
	read, ok := asked[model.LoadFederationHubProjects](chosen.Cmds)
	if !ok || read.Hub != "hub" || chosen.State.Model.Federation.Screen != model.FederationPickHubProject {
		t.Fatalf("Enter on the hub gave screen %v and %#v, want a read of its Projects", chosen.State.Model.Federation.Screen, chosen.Cmds)
	}
	wantShown(t, "the hub Project step while the hub is read", chosen.State, "Select hub project", "hub daemon: hub https://hub.example:7474", "allow_insecure: true", "loading hub projects...")
}

func TestAnAnswerToAStepYouLeftIsDropped(t *testing.T) {
	s := step(t, enrollPane(t), key("n")).State
	out := step(t, cursorOn(t, s, rowAcme), key("enter"))
	load, _ := asked[model.LoadFederationHubs](out.Cmds)
	back := step(t, out.State, key("escape")).State
	late := step(t, back, hubsAnswer(load)).State
	if late.Model.Federation.Screen != model.FederationPickLocal || len(late.Model.Federation.Hubs) != 0 {
		t.Errorf("a late catalog answer gave screen %v with %d hubs", late.Model.Federation.Screen, len(late.Model.Federation.Hubs))
	}

	hubs := atHubStep(t, enrollPane(t), rowAcme)
	chosen := step(t, cursorOn(t, hubs, 2), key("enter"))
	read, _ := asked[model.LoadFederationHubProjects](chosen.Cmds)
	left := step(t, chosen.State, key("escape")).State
	if got := step(t, left, hubProjectsAnswer(read, hubBeta)).State; got.Model.Federation.Screen != model.FederationPickHub || len(got.Model.Federation.HubProjects) != 0 {
		t.Errorf("a late hub answer gave screen %v with %d Projects", got.Model.Federation.Screen, len(got.Model.Federation.HubProjects))
	}
}

func TestACatalogThatCannotBeReadIsTheHubSteps(t *testing.T) {
	s := step(t, enrollPane(t), key("n")).State
	out := step(t, cursorOn(t, s, rowAcme), key("enter"))
	load, _ := asked[model.LoadFederationHubs](out.Cmds)
	failed := step(t, out.State, model.FederationHubsLoaded{Request: load.Request, Err: errors.New("parse /kata/config.toml: bad")}).State
	wantShown(t, "the hub step", failed, "parse /kata/config.toml: bad")
	wantNotShown(t, "the hub step", failed, "loading hub daemons...")

	empty := step(t, out.State, model.FederationHubsLoaded{Request: load.Request}).State
	wantShown(t, "an empty catalog", empty, "no daemons in kata's catalog")
}

func TestTheHubProjectStepOffersTheSameNameRowAsUseOrCreateAndNeverListsTheSameNameProjectTwice(t *testing.T) {
	existing := atHubProjectStep(t, enrollPane(t), rowAcme, hubAcme, hubBeta)
	wantShown(t, "the step with the same-name Project on the hub", existing, `use existing hub project "acme"; enable federation if needed`, "\nbeta")
	text := shownText(t, existing)
	if strings.Count(text, "\nacme") != 0 {
		t.Errorf("the same-name Project is listed as a row of its own:\n%s", text)
	}
	if _, total := existing.Model.Position(); total != 2 {
		t.Errorf("%d rows, want the same-name row and beta", total)
	}
	wantShown(t, "the hub's reading", existing, "hub auth: token actor erin")

	creating := atHubProjectStep(t, enrollPane(t), rowAcme, hubBeta)
	wantShown(t, "the step with no same-name Project", creating, `create hub project "acme" and enable federation`)

	replica := atHubProjectStep(t, enrollPane(t), rowReplica, hubAcme, hubBeta)
	text = shownText(t, replica)
	if strings.Contains(text, "create hub project") || strings.Contains(text, "use existing hub project") {
		t.Errorf("a new replica is offered the same-name row:\n%s", text)
	}
	if _, total := replica.Model.Position(); total != 2 {
		t.Errorf("%d rows, want every hub Project", total)
	}
	wantShown(t, "the new replica's step", replica, "\nacme", "\nbeta")
}

func TestAHubThatCannotBeReadKeepsTheStepOrReturnsToTheHubStepWhenNothingWasAsked(t *testing.T) {
	hubs := atHubStep(t, enrollPane(t), rowAcme)
	chosen := step(t, cursorOn(t, hubs, 2), key("enter"))
	read, _ := asked[model.LoadFederationHubProjects](chosen.Cmds)

	early := step(t, chosen.State, model.FederationHubProjectsLoaded{Request: read.Request, Early: true, Err: errors.New(`daemon "hub": token_env "HUB_TOKEN" is unset or empty`)}).State
	if early.Model.Federation.Screen != model.FederationPickHub {
		t.Fatalf("a refused token left screen %v, want the hub step", early.Model.Federation.Screen)
	}
	wantShown(t, "the hub step", early, `daemon "hub": token_env "HUB_TOKEN" is unset or empty`)

	failed := step(t, chosen.State, model.FederationHubProjectsLoaded{Request: read.Request, Err: errors.New("hub unreachable")}).State
	if failed.Model.Federation.Screen != model.FederationPickHubProject {
		t.Fatalf("a failed read left screen %v", failed.Model.Federation.Screen)
	}
	wantShown(t, "the hub Project step", failed, "hub unreachable")
	wantNotShown(t, "the hub Project step", failed, "loading hub projects...")
}

// previewed presses Enter on the hub Project step and returns the preview.
func previewed(t *testing.T, s driver.State) driver.State {
	t.Helper()
	out := step(t, s, key("enter"))
	if out.State.Model.Federation.Screen != model.FederationPreview || len(out.Cmds) != 0 || out.State.Model.CurrentMode() != model.ModeFederationPreview {
		t.Fatalf("Enter gave screen %v, mode %q and %#v, want the preview and no daemon call", out.State.Model.Federation.Screen, out.State.Model.CurrentMode(), out.Cmds)
	}
	return out.State
}

func TestThePreviewNamesWhatAnAdoptionOfTheSameNameProjectDoes(t *testing.T) {
	s := previewed(t, atHubProjectStep(t, enrollPane(t), rowAcme, hubBeta))
	wantShown(t, "the preview", s, "Enrollment Preview", "selected project: acme", "Operation: adopt existing local project", "local spoke project: acme",
		"hub daemon: hub https://hub.example:7474", "hub auth: token actor erin", `hub project: create hub project "acme" and enable federation`,
		"requested actor: erin", "capabilities: pull,push,lease", "push enabled: true", "allow_insecure: true",
		`this federates local project "acme" INTO hub project "acme"`, "adoption warning: pre-adoption event history is replaced by snapshot events for federation")
	wantNotShown(t, "the preview", s, "Blocked")
	wantText(t, "the footer", footerText(t, s), "↵", "confirm", "back")

	existing := previewed(t, atHubProjectStep(t, enrollPane(t), rowAcme, hubAcme, hubBeta))
	wantShown(t, "the preview with the Project on the hub", existing, `hub project: use existing hub project "acme"; enable federation if needed`)
	if got := existing.Model.Federation.Draft.HubProjectID; got != 1 {
		t.Errorf("the draft's hub Project is %d, want the same-name Project, 1", got)
	}

	selected := previewed(t, step(t, atHubProjectStep(t, enrollPane(t), rowAcme, hubBeta), key("j")).State)
	wantShown(t, "the preview of a chosen hub Project", selected, "Operation: adopt existing local project into selected hub project", "hub project: beta",
		`this federates local project "acme" INTO hub project "beta"`)
}

func TestTheRequestedActorIsThePanesUnlessTheHubSaysWhoItActedAs(t *testing.T) {
	hubs := atHubStep(t, enrollPane(t), rowAcme)
	chosen := step(t, cursorOn(t, hubs, 2), key("enter"))
	read, _ := asked[model.LoadFederationHubProjects](chosen.Cmds)
	answer := hubProjectsAnswer(read, hubBeta)
	answer.Actor = ""
	s := previewed(t, step(t, chosen.State, answer).State)
	wantShown(t, "the preview", s, "requested actor: dana")
}

func TestTheNewReplicaPreviewBlocksWhenAProjectOfThatNameOrABindingIsThere(t *testing.T) {
	for name, c := range map[string]struct {
		hub     model.Project
		blocked string
	}{
		"a local Project of that name":           {model.Project{ID: 8, UID: "H-ACME2", Name: "acme"}, `Blocked: local project "acme" already exists`},
		"a bound local Project of that identity": {hubHubby, `Blocked: local project "hubby" already has federation binding as hub`},
	} {
		s := previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, c.hub))
		wantShown(t, name, s, "Enrollment Preview", "Operation: create new local replica from hub project", c.blocked)
		out := step(t, s, key("enter"))
		if len(out.Cmds) != 0 || out.State.Model.TopCard() != model.NoCard || out.State.Model.Federation.Screen != model.FederationPreview {
			t.Errorf("%s: Enter on a blocked preview gave screen %v, card %v and %#v", name, out.State.Model.Federation.Screen, out.State.Model.TopCard(), out.Cmds)
		}
	}
}

func TestTheAdoptionPreviewBlocksWhenTheProjectIsBoundOrAnIdentityIsUnknown(t *testing.T) {
	bound := previewed(t, atHubProjectStep(t, enrollPane(t), rowZeta, hubBeta))
	wantShown(t, "a bound Project", bound, `Blocked: local project "Zeta" already has federation binding as spoke`)
	if out := step(t, bound, key("enter")); len(out.Cmds) != 0 || out.State.Model.TopCard() != model.NoCard {
		t.Errorf("Enter on a blocked adoption gave card %v and %#v", out.State.Model.TopCard(), out.Cmds)
	}

	unknown := model.Project{ID: 2, Name: "beta"}
	s := previewed(t, step(t, atHubProjectStep(t, enrollPane(t), rowAcme, unknown), key("j")).State)
	wantShown(t, "a hub Project of unknown identity", s, `Blocked: hub project "beta" identity is unknown; refresh the hub project list and retry`)
}

func TestAProjectThatLeftTheFederationRejoinsWithoutTypingItsName(t *testing.T) {
	// Creating a replica of a hub Project whose identity a local Project holds.
	replica := previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, model.Project{ID: 1, UID: "P-ACME", Name: "acme"}))
	wantShown(t, "the new replica's preview", replica, "Operation: rejoin: resume syncing a local project that previously left this federation",
		"selected project: acme (rejoin)", `local project "acme" previously left this federation and still shares the hub project's identity;`,
		`rejoining resumes syncing with hub project "acme" (edits made while standalone sync to the hub)`)
	wantNotShown(t, "the new replica's preview", replica, "adoption warning", "Blocked", "the local project keeps its name")
	out := step(t, replica, key("enter"))
	run, ok := asked[model.EnrollFederation](out.Cmds)
	if !ok || out.State.Model.TopCard() != model.NoCard || run.Draft.Operation != model.FederationRejoin || run.Draft.SpokeProjectName != "acme" || run.Draft.AdoptExisting {
		t.Fatalf("Enter on a rejoin gave card %v and %#v, want the run with no typed confirmation", out.State.Model.TopCard(), out.Cmds)
	}

	// Adopting into a hub Project of another name that has the local Project's identity.
	adopt := previewed(t, step(t, atHubProjectStep(t, enrollPane(t), rowAcme, model.Project{ID: 2, UID: "P-ACME", Name: "other"}), key("j")).State)
	wantShown(t, "the adoption's preview", adopt, "Operation: rejoin", "the local project keeps its name; spoke and hub project names do not need to match")
	if got := step(t, adopt, key("enter")); got.State.Model.TopCard() != model.NoCard {
		t.Errorf("a rejoin asked to type the Project's name")
	}
}

func TestANewReplicaNeedsAHubProjectToReplicate(t *testing.T) {
	s := atHubProjectStep(t, enrollPane(t), rowReplica)
	wantShown(t, "the step", s, "no hub projects")
	out := step(t, s, key("enter"))
	if out.State.Model.Federation.Screen != model.FederationPickHubProject || len(out.Cmds) != 0 {
		t.Fatalf("Enter with no hub Project gave screen %v and %#v", out.State.Model.Federation.Screen, out.Cmds)
	}
	wantShown(t, "the step", out.State, "select an existing hub project to create a local replica")
}

func TestEscInThePreviewReturnsToTheHubProjectStepWithItsProjects(t *testing.T) {
	s := previewed(t, atHubProjectStep(t, enrollPane(t), rowAcme, hubBeta))
	back := step(t, s, key("escape")).State
	if back.Model.Federation.Screen != model.FederationPickHubProject {
		t.Fatalf("esc gave screen %v", back.Model.Federation.Screen)
	}
	wantShown(t, "the hub Project step", back, "Select hub project", `create hub project "acme" and enable federation`, "\nbeta")
	if again := step(t, s, key("backspace")).State; again.Model.Federation.Screen != model.FederationPickHubProject {
		t.Errorf("backspace gave screen %v", again.Model.Federation.Screen)
	}
}

// confirming is the adoption of acme in its preview with the typed confirmation open.
func confirming(t *testing.T) driver.State {
	t.Helper()
	s := previewed(t, atHubProjectStep(t, enrollPane(t), rowAcme, hubBeta))
	out := step(t, s, key("enter"))
	if out.State.Model.TopCard() != model.CardFederationAdopt || out.State.Model.CurrentMode() != model.ModeFederationAdopt || len(out.Cmds) != 0 {
		t.Fatalf("Enter on an adoption gave card %v, mode %q and %#v, want the confirmation card and no daemon call", out.State.Model.TopCard(), out.State.Model.CurrentMode(), out.Cmds)
	}
	return out.State
}

func typeText(t *testing.T, s driver.State, text string) driver.State {
	t.Helper()
	for _, r := range text {
		s = step(t, s, key(string(r))).State
	}
	return s
}

func TestAnAdoptionAsksToTypeTheLocalProjectsExactNameBeforeItRuns(t *testing.T) {
	s := confirming(t)
	wantShown(t, "the confirmation", s, "Confirm Adoption", `federate local project "acme" INTO hub project "acme"?`,
		`"acme"'s issues will be adopted into the hub project and its local event history rewritten as snapshots`, `type "acme" to confirm:`)
	if !strings.HasPrefix(s.Focus, layerTitled(t, s.Model, "Confirm Adoption")) {
		t.Errorf("text focus %q is not on the confirmation", s.Focus)
	}
	wantText(t, "the footer", footerText(t, s), "enter", "adopt", "esc", "back")

	for _, wrong := range []string{"", "ac", "ACME", "acmex"} {
		attempt := step(t, typeText(t, s, wrong), key("enter"))
		if len(attempt.Cmds) != 0 || attempt.State.Model.TopCard() != model.CardFederationAdopt {
			t.Fatalf("Enter after %q gave card %v and %#v", wrong, attempt.State.Model.TopCard(), attempt.Cmds)
		}
		wantShown(t, "the refusal of "+wrong, attempt.State, `type "acme" to confirm adoption`)
	}
	typedWrong := typeText(t, s, "acm")
	if !slices.Contains(strings.Split(shownText(t, typedWrong), "\n"), "acm") {
		t.Errorf("the typed text is not drawn:\n%s", shownText(t, typedWrong))
	}
	if fixed := step(t, typedWrong, key("backspace")).State; fixed.Model.Federation.AdoptInput != "ac" {
		t.Errorf("backspace left %q", fixed.Model.Federation.AdoptInput)
	}

	exact := step(t, typeText(t, s, "acme"), key("enter"))
	run, ok := asked[model.EnrollFederation](exact.Cmds)
	if !ok || run.Workspace != "/repo" || run.InstanceUID != "INSTANCE-1" || run.Attempt == 0 {
		t.Fatalf("the exact name asked %#v, want the enrollment", exact.Cmds)
	}
	d := run.Draft
	if d.Operation != model.FederationAdoptSameName || d.SpokeProjectName != "acme" || d.Hub.Name != "hub" || d.HubProjectID != 0 || d.RequestedActor != "erin" ||
		d.APICapabilities != "claim,pull,push" || !d.PushEnabled || !d.AllowInsecure || !d.AdoptExisting {
		t.Errorf("the run's draft %+v", d)
	}
	if exact.State.Model.TopCard() != model.NoCard {
		t.Error("the confirmation stayed open while the enrollment runs")
	}
	wantShown(t, "the preview while it runs", exact.State, "Enrollment Preview", "enrolling...")
}

func TestEscInTheConfirmationReturnsToThePreviewAndForgetsWhatWasTyped(t *testing.T) {
	s := typeText(t, confirming(t), "ac")
	back := step(t, s, key("escape"))
	if back.State.Model.TopCard() != model.NoCard || back.State.Model.Federation.Screen != model.FederationPreview || len(back.Cmds) != 0 {
		t.Fatalf("esc gave card %v, screen %v and %#v", back.State.Model.TopCard(), back.State.Model.Federation.Screen, back.Cmds)
	}
	again := step(t, back.State, key("enter")).State
	if again.Model.Federation.AdoptInput != "" || again.Model.Federation.AdoptErr != nil {
		t.Errorf("the reopened confirmation holds %q and %v", again.Model.Federation.AdoptInput, again.Model.Federation.AdoptErr)
	}
}

// running is the new replica of beta in its preview after Enter: the enrollment is out.
func running(t *testing.T) (driver.State, model.EnrollFederation) {
	t.Helper()
	s := previewed(t, atHubProjectStep(t, enrollPane(t), rowReplica, hubBeta))
	out := step(t, s, key("enter"))
	run, ok := asked[model.EnrollFederation](out.Cmds)
	if !ok {
		t.Fatalf("Enter asked %#v, want the enrollment", out.Cmds)
	}
	if d := run.Draft; d.Operation != model.FederationCreateReplica || d.HubProjectID != 2 || d.SpokeProjectName != "beta" || d.AdoptExisting {
		t.Fatalf("the run's draft %+v", d)
	}
	return out.State, run
}

func TestWhileAnEnrollmentRunsEnterAndEscDoNothing(t *testing.T) {
	s, _ := running(t)
	wantShown(t, "the preview", s, "enrolling...")
	if out := step(t, s, key("enter")); len(out.Cmds) != 0 || !out.State.Model.Federation.Enrolling {
		t.Errorf("Enter while it runs asked %#v", out.Cmds)
	}
	for _, back := range []string{"escape", "backspace"} {
		if out := step(t, s, key(back)); out.State.Model.Federation.Screen != model.FederationPreview || !out.State.Model.Federation.Enrolling {
			t.Errorf("%s while it runs left screen %v", back, out.State.Model.Federation.Screen)
		}
	}
}

func TestASuccessShowsTheResultReadsTheStatusAndEnterOrEscReturnToTheList(t *testing.T) {
	for name, c := range map[string]struct {
		outcome model.FederationEnrollOutcome
		want    []string
	}{
		"joined": {model.FederationEnrollOutcome{HubURL: "https://hub.example:7474", Actor: "erin", HubProjectID: 2, HubProjectUID: "H-BETA"},
			[]string{"status: joined", "snapshot count: 0"}},
		"adopted": {model.FederationEnrollOutcome{HubURL: "https://hub.example:7474", Actor: "erin", HubProjectID: 2, HubProjectUID: "H-BETA", Adopted: true, SnapshotCount: 4},
			[]string{"status: adopted", "snapshot count: 4"}},
	} {
		for _, leave := range []string{"enter", "escape", "backspace"} {
			s, run := running(t)
			done := step(t, s, model.FederationEnrolled{Attempt: run.Attempt, Outcome: c.outcome})
			m := done.State.Model
			if m.Federation.Screen != model.FederationEnrollDone || m.CurrentMode() != model.ModeFederationResult || m.Federation.Enrolling {
				t.Fatalf("%s: screen %v, mode %q", name, m.Federation.Screen, m.CurrentMode())
			}
			if read, asked := askedFederation(done.Cmds); !asked || read.Workspace != "/repo" {
				t.Errorf("%s: the result asked %#v, want a status read", name, done.Cmds)
			}
			wantShown(t, name+" result", done.State, append([]string{"Enrollment Result", "actor: erin", "hub URL: https://hub.example:7474", "hub project ID: 2", "hub project UID: H-BETA"}, c.want...)...)
			wantText(t, name+" footer", footerText(t, done.State), "list")
			back := step(t, done.State, key(leave)).State
			if back.Model.Federation.Screen != model.FederationList || back.Model.CurrentMode() != model.ModeFederation {
				t.Errorf("%s: %s gave screen %v, want the list", name, leave, back.Model.Federation.Screen)
			}
		}
	}
}

func TestAnAnswerToAnEarlierRunIsDropped(t *testing.T) {
	s, run := running(t)
	got := step(t, s, model.FederationEnrolled{Attempt: run.Attempt + 1, Outcome: model.FederationEnrollOutcome{}}).State
	if got.Model.Federation.Screen != model.FederationPreview || !got.Model.Federation.Enrolling {
		t.Errorf("an answer to another run gave screen %v, enrolling %v", got.Model.Federation.Screen, got.Model.Federation.Enrolling)
	}
}

func TestAFailureBeforeTheHubGrantedKeepsThePreviewWithTheErrorAndCanRunAgain(t *testing.T) {
	s, run := running(t)
	failed := step(t, s, model.FederationEnrolled{Attempt: run.Attempt, Err: errors.New("hub refused: not allowed")})
	m := failed.State.Model
	if m.Federation.Screen != model.FederationPreview || m.Federation.Enrolling || len(failed.Cmds) != 0 {
		t.Fatalf("a failure gave screen %v, enrolling %v and %#v", m.Federation.Screen, m.Federation.Enrolling, failed.Cmds)
	}
	wantShown(t, "the preview", failed.State, "Enrollment Preview", "hub refused: not allowed")
	wantNotShown(t, "the preview", failed.State, "enrolling...", "Recovery")
	again := step(t, failed.State, key("enter"))
	next, ok := asked[model.EnrollFederation](again.Cmds)
	if !ok || next.Attempt == run.Attempt {
		t.Fatalf("Enter after a failure asked %#v, want a new run", again.Cmds)
	}
	wantNotShown(t, "the preview of the new run", again.State, "hub refused: not allowed")
}

func recovery(stage string, guess bool) *model.FederationEnrollRecovery {
	return &model.FederationEnrollRecovery{HubName: "hub", SpokeName: "work", SpokeEndpoint: "https://spoke.example:7474", Stage: stage, TokenGuess: guess,
		Command: model.FederationJoinCommand{HubURL: "https://hub.example:7474", HubProjectID: 2, HubProjectUID: "H-BETA", ProjectName: "beta",
			ReplayHorizonEventID: 5, BaselineThroughEventID: 4, Token: "s3cret-token", Actor: "erin", Capabilities: "claim,pull,push", PushEnabled: true,
			AllowInsecure: true, SpokeEndpoint: "https://spoke.example:7474", SpokeToken: "spoke-token"}}
}

func TestAFailureAfterTheGrantKeepsARecoveryThatRevealsTheCommandOnlyOnR(t *testing.T) {
	s, run := running(t)
	lapsed := step(t, s, model.FederationEnrolled{Attempt: run.Attempt, Err: errors.New("spoke: join failed: conflict"), Recovery: recovery("join", true)})
	m := lapsed.State.Model
	if m.Federation.Screen != model.FederationRecovery || m.CurrentMode() != model.ModeFederationRecovery || m.Federation.Enrolling || len(lapsed.Cmds) != 0 {
		t.Fatalf("the failure gave screen %v, mode %q and %#v, want the recovery", m.Federation.Screen, m.CurrentMode(), lapsed.Cmds)
	}
	wantShown(t, "the recovery", lapsed.State, "Enrollment Recovery", "hub: enrollment created", "spoke: join failed", "spoke: join failed: conflict", "token: hidden",
		"the hub enrollment may be single-use, expired, revoked, or invalidated")
	wire := wireTextOf(t, lapsed.State) + shownText(t, lapsed.State)
	for _, secret := range []string{"s3cret-token", "spoke-token", "kata federation join", "secret-bearing"} {
		if strings.Contains(wire, secret) {
			t.Errorf("the recovery shows %q before R", secret)
		}
	}
	wantText(t, "the footer", footerText(t, lapsed.State), "R", "reveal recovery command", "back")

	revealed := step(t, lapsed.State, key("shift+r"))
	if len(revealed.Cmds) != 0 {
		t.Errorf("R asked %#v", revealed.Cmds)
	}
	wantShown(t, "the revealed recovery", revealed.State, "single-use/secret-bearing recovery command", "works only while the hub enrollment remains valid and not revoked",
		"spoke target: work https://spoke.example:7474",
		"KATA_SERVER=https://spoke.example:7474 KATA_AUTH_TOKEN=spoke-token kata federation join --hub-url https://hub.example:7474 --hub-project-id 2 --project beta "+
			"--token s3cret-token --actor erin --capabilities claim,pull,push --hub-project-uid H-BETA --replay-horizon 5 --baseline-through 4 --push --allow-insecure")

	for _, back := range []string{"escape", "backspace"} {
		out := step(t, revealed.State, key(back)).State
		if out.Model.Federation.Screen != model.FederationPreview || out.Model.Federation.Recovery.Reveal {
			t.Errorf("%s gave screen %v, revealed %v", back, out.Model.Federation.Screen, out.Model.Federation.Recovery.Reveal)
		}
		wantNotShown(t, back+" back at the preview", out, "s3cret-token")
	}
}

func TestTheRecoveryOfAMetadataFailureNamesTheHubAndOnlyGuessesTheTokenWhenItWasRefused(t *testing.T) {
	s, run := running(t)
	lapsed := step(t, s, model.FederationEnrolled{Attempt: run.Attempt, Err: errors.New("hub hub: enrollment metadata fetch failed: bad gateway"), Recovery: recovery("metadata", false)}).State
	wantShown(t, "the recovery", lapsed, "hub hub: enrollment metadata fetch failed", "hub hub: enrollment metadata fetch failed: bad gateway", "token: hidden")
	wantNotShown(t, "the recovery", lapsed, "spoke: join failed", "may be single-use")
}

func TestBOpensTheReadOnlyBrowseOfTheHubsProjects(t *testing.T) {
	s := enrollPane(t)
	out := step(t, s, key("b"))
	read, ok := asked[model.BrowseFederationHubs](out.Cmds)
	m := out.State.Model
	if !ok || read.Workspace != "/repo" || m.Federation.Screen != model.FederationBrowse || m.CurrentMode() != model.ModeFederationBrowse {
		t.Fatalf("b gave screen %v, mode %q and %#v, want the browse and a read of the hubs", m.Federation.Screen, m.CurrentMode(), out.Cmds)
	}
	wantShown(t, "the browse while it is read", out.State, "Browse catalog hub projects", "mode: read-only", "loading hub projects...")

	answer := model.FederationBrowsed{Request: read.Request, Hubs: []model.FederationHubProjects{
		{Hub: model.FederationHub{Name: "east", Endpoint: "https://east.example"}, Auth: "token actor erin", Projects: []model.Project{hubAcme, hubBeta}},
		{Hub: model.FederationHub{Name: "west", Endpoint: "https://west.example", AllowInsecure: true}, Auth: "token", Err: errors.New("hub down")},
	}}
	browsed := step(t, out.State, answer)
	wantShown(t, "the browse", browsed.State, "catalog hub: east https://east.example · auth token actor erin · allow_insecure: false", "east  1 acme", "east  2 beta",
		"catalog hub: west https://west.example · auth token · allow_insecure: true", "failed to load hub projects: hub down", "mode: read-only")
	wantNotShown(t, "the browse", browsed.State, "loading hub projects...")
	wantText(t, "the footer", footerText(t, browsed.State), "back")

	if sel, total := browsed.State.Model.Position(); sel != 1 || total != 2 {
		t.Errorf("position %d of %d", sel, total)
	}
	if moved := step(t, browsed.State, key("j")).State.Model.Federation.BrowseCursor; moved != 1 {
		t.Errorf("j left the cursor on %d", moved)
	}
	// Browsing is read only: Enter chooses nothing and sends nothing.
	entered := step(t, browsed.State, key("enter"))
	if len(entered.Cmds) != 0 || entered.State.Model.Federation.Screen != model.FederationBrowse || entered.State.Model.TopCard() != model.NoCard {
		t.Errorf("Enter in the browse gave screen %v, card %v and %#v", entered.State.Model.Federation.Screen, entered.State.Model.TopCard(), entered.Cmds)
	}
	for _, back := range []string{"escape", "backspace"} {
		if list := step(t, browsed.State, key(back)).State; list.Model.Federation.Screen != model.FederationList || list.Model.CurrentMode() != model.ModeFederation {
			t.Errorf("%s gave screen %v", back, list.Model.Federation.Screen)
		}
	}

	left := step(t, out.State, key("escape")).State
	if late := step(t, left, answer).State; late.Model.Federation.Screen != model.FederationList || len(late.Model.Federation.Browsed) != 0 {
		t.Errorf("a late browse answer gave screen %v with %d hubs", late.Model.Federation.Screen, len(late.Model.Federation.Browsed))
	}
}

func TestTheBrowseSaysWhenTheCatalogHasNoHubsOrCannotBeRead(t *testing.T) {
	out := step(t, enrollPane(t), key("b"))
	read, _ := asked[model.BrowseFederationHubs](out.Cmds)
	none := step(t, out.State, model.FederationBrowsed{Request: read.Request}).State
	wantShown(t, "an empty browse", none, "failed to load hub projects: no catalog hub daemons configured")
	wantNotShown(t, "an empty browse", none, "loading hub projects...")
	broken := step(t, out.State, model.FederationBrowsed{Request: read.Request, Err: errors.New("parse /kata/config.toml: bad")}).State
	wantShown(t, "an unreadable catalog", broken, "failed to load hub projects: parse /kata/config.toml: bad")
	one := step(t, out.State, model.FederationBrowsed{Request: read.Request, Hubs: []model.FederationHubProjects{{Hub: model.FederationHub{Name: "east", Endpoint: "https://east.example"}}}}).State
	wantShown(t, "a hub with no Projects", one, "no hub projects")
}

func TestNAndBOnlyStartFromTheListNotFromASpokesDetail(t *testing.T) {
	detail := step(t, enrollPane(t), key("enter")).State
	if detail.Model.Federation.Screen != model.FederationDetail {
		t.Fatalf("Enter gave screen %v", detail.Model.Federation.Screen)
	}
	for _, k := range []string{"n", "b"} {
		out := step(t, detail, key(k))
		if out.State.Model.Federation.Screen != model.FederationDetail || len(out.Cmds) != 0 {
			t.Errorf("%s in a detail gave screen %v and %#v", k, out.State.Model.Federation.Screen, out.Cmds)
		}
	}
}

func TestTheFederationListFootersAndHelpNameEnrollAndBrowse(t *testing.T) {
	s := enrollPane(t)
	wantText(t, "the footer", footerText(t, s), "n", "enroll", "b", "browse hubs")
	help := step(t, s, key("?")).State
	text := shownText(t, help)
	for _, want := range []string{"enroll a Project in a hub's federation", "browse the Projects of the catalog's hubs (read only)"} {
		if !strings.Contains(text, want) {
			t.Errorf("the help sheet lacks %q", want)
		}
	}
}

func TestEnrollingStartsFromAListWhoseStatusReadFailed(t *testing.T) {
	// The status read failed, so the daemon's instance is unknown; the enrollment still starts, and the gateway refuses a run in words.
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	s.Model = loadFirst(s.Model, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "P-WORK", Name: "work"}},
		model.SnapshotLoaded{Snapshot: model.Snapshot{Issues: []model.Issue{boardCard("a")}, Catalog: []model.CatalogProject{{Project: model.Project{ID: 7, UID: "P-WORK", Name: "work"}}}}, LocalDate: boardToday, TimeZone: "UTC"})
	out := federationOpened(t, feed(t, s).State)
	load, _ := askedFederation(out.Cmds)
	failed := step(t, out.State, model.FederationLoaded{Request: load.Request, Err: errors.New("down")}).State
	picking := step(t, failed, key("n")).State
	if picking.Model.Federation.Screen != model.FederationPickLocal {
		t.Fatalf("n gave screen %v", picking.Model.Federation.Screen)
	}
	rows := picking.Model.FederationLocalChoices()
	if len(rows) != 2 || !rows[0].CreateReplica || rows[1].Project.Name != "work" {
		t.Errorf("rows %+v, want the new replica and work", rows)
	}
	if got := picking.Model.FederationChoiceCursor(); got != 1 {
		t.Errorf("the cursor is on %d, want the Pane's Project", got)
	}
}
