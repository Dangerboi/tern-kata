package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// enrollRig is two in-process kata daemons, a hub and a spoke, with a daemon catalog in the throwaway KATA_HOME that names the spoke
// as the active daemon, a local entry and the hub with a token of its own. The test service runs both over HTTP.
type enrollRig struct {
	hub, spoke *kataServer
}

func startEnrollRig(t *testing.T) enrollRig {
	t.Helper()
	rig := enrollRig{hub: startKata(t, "0.20.0"), spoke: startKata(t, "0.20.0")}
	t.Setenv("HUB_TOKEN", "hub-secret")
	rig.writeCatalog(t, fmt.Sprintf(`active_daemon = "spoke"

[[daemon]]
name = "spoke"
url = %q
allow_insecure = true

[[daemon]]
name = "laptop"
local = true

[[daemon]]
name = "hub"
url = %q
allow_insecure = true
token_env = "HUB_TOKEN"
`, rig.spoke.URL, rig.hub.URL))
	return rig
}

func (enrollRig) writeCatalog(t *testing.T, config string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (r enrollRig) hubGateway(t *testing.T) *Gateway {
	return newGateway(t, newLocator(r.hub.target()))
}

// spokeGateway is the gateway of the Pane, which talks to the spoke as a remote daemon.
func (r enrollRig) spokeGateway(t *testing.T) *Gateway {
	t.Helper()
	target := r.spoke.target()
	target.Remote = true
	return newGateway(t, newLocator(target))
}

func (r enrollRig) spokeInstanceUID(t *testing.T) string {
	t.Helper()
	instance, err := r.spoke.target().Client.InstanceWithResponse(context.Background())
	if err != nil || instance.JSON200 == nil {
		t.Fatalf("spoke instance: %v", err)
	}
	return instance.JSON200.InstanceUID
}

// hubEnrollments is what the hub lists as grants.
func (r enrollRig) hubEnrollments(t *testing.T) []g.FederationEnrollmentOut {
	t.Helper()
	listed, err := r.hub.target().Client.ListFederationEnrollmentsWithResponse(context.Background())
	if err != nil || listed.JSON200 == nil {
		t.Fatalf("list enrollments: %v", err)
	}
	return listed.JSON200.Enrollments
}

func (r enrollRig) spokeProjects(t *testing.T) []model.FederationSpoke {
	t.Helper()
	return as[model.FederationLoaded](t, run(t, r.spokeGateway(t), model.LoadFederation{Workspace: t.TempDir(), Request: 1})).Spokes
}

// call is a request a daemon saw: its method, path and the Authorization header it carried.
type call struct{ method, path, auth string }

type callLog struct {
	mu    sync.Mutex
	calls []call
}

// watch records every request the daemon gets. An answer, when given, may take a request over: it reports that it did.
func watch(s *kataServer, answer func(http.ResponseWriter, *http.Request) bool) *callLog {
	log := &callLog{}
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		log.mu.Lock()
		log.calls = append(log.calls, call{r.Method, r.URL.Path, r.Header.Get("Authorization")})
		log.mu.Unlock()
		return answer != nil && answer(w, r)
	}
	s.intercept.Store(&hook)
	return log
}

// seen are the calls whose path ends in suffix, in order.
func (l *callLog) seen(suffix string) []call {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []call
	for _, c := range l.calls {
		if strings.HasSuffix(c.path, suffix) {
			out = append(out, c)
		}
	}
	return out
}

func (l *callLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

// order is the position of each call among the calls to the given path suffixes, so a sequence reads as one list.
func (l *callLog) order(suffixes ...string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, c := range l.calls {
		for _, suffix := range suffixes {
			if strings.HasSuffix(c.path, suffix) {
				out = append(out, c.method+" "+suffix)
			}
		}
	}
	return out
}

func refuseWith(status int, code, message string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"status":%d,"error":{"code":%q,"message":%q}}`, status, code, message)
	}
}

// replicaDraft is the enrollment of a new local replica of the hub Project hub.
func replicaDraft(hub model.Project) model.FederationEnrollDraft {
	return model.FederationEnrollDraft{Operation: model.FederationCreateReplica, CreateReplica: true, SpokeProjectName: hub.Name, Hub: model.FederationHub{Name: "hub"},
		HubProjectID: hub.ID, HubProjectName: hub.Name, RequestedActor: "alice", APICapabilities: "claim,pull,push", DisplayCapabilities: "pull,push,lease",
		PushEnabled: true, AllowInsecure: true}
}

func (r enrollRig) enroll(t *testing.T, draft model.FederationEnrollDraft) model.FederationEnrolled {
	t.Helper()
	return as[model.FederationEnrolled](t, run(t, r.spokeGateway(t), model.EnrollFederation{Workspace: t.TempDir(), Attempt: 3, InstanceUID: r.spokeInstanceUID(t), Draft: draft}))
}

func TestTheHubStepListsKatasCatalogInFileOrderWithTheActiveDaemonMarked(t *testing.T) {
	rig := startEnrollRig(t)
	loaded := as[model.FederationHubsLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederationHubs{Workspace: t.TempDir(), Request: 4}))
	want := []model.FederationHub{
		{Name: "spoke", Endpoint: rig.spoke.URL, Auth: "no token", AllowInsecure: true, Active: true},
		{Name: "laptop", Endpoint: "local", Auth: "no token", Local: true},
		{Name: "hub", Endpoint: rig.hub.URL, Auth: "token", AllowInsecure: true},
	}
	if loaded.Err != nil || loaded.Request != 4 || !slices.Equal(loaded.Hubs, want) {
		t.Fatalf("hubs %+v (request %d, error %v), want %+v", loaded.Hubs, loaded.Request, loaded.Err, want)
	}

	rig.writeCatalog(t, "this is = not [valid toml")
	broken := as[model.FederationHubsLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederationHubs{Workspace: t.TempDir(), Request: 5}))
	if broken.Err == nil || !strings.Contains(broken.Err.Error(), "config.toml") || broken.Request != 5 {
		t.Errorf("an unreadable catalog answered %+v, want its error naming the file", broken)
	}
}

func TestReadingAHubSendsItsOwnTokenAndNeverTheGlobalOne(t *testing.T) {
	rig := startEnrollRig(t)
	t.Setenv("KATA_AUTH_TOKEN", "global-secret")
	hubGateway := rig.hubGateway(t)
	_, acme := project(t, hubGateway, "acme")
	_, beta := project(t, hubGateway, "beta")
	log := watch(rig.hub, nil)

	loaded := as[model.FederationHubProjectsLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederationHubProjects{Workspace: t.TempDir(), Request: 8, Hub: "hub"}))
	if loaded.Err != nil || loaded.Early || loaded.Request != 8 {
		t.Fatalf("read %+v", loaded)
	}
	got := map[string]model.Project{}
	for _, p := range loaded.Projects {
		got[p.Name] = p
	}
	if len(got) != 2 || got["acme"].UID != acme.UID || got["acme"].ID != acme.ID || got["beta"].UID != beta.UID {
		t.Errorf("Projects %+v, want acme and beta with their ids and uids", loaded.Projects)
	}
	if hub := loaded.Hub; hub.Name != "hub" || hub.Endpoint != rig.hub.URL || !hub.AllowInsecure || loaded.Auth != "token" {
		t.Errorf("hub %+v auth %q, want the catalog's hub with its token", hub, loaded.Auth)
	}
	reads := append(log.seen("/api/v1/instance"), log.seen("/api/v1/projects")...)
	if len(reads) < 2 {
		t.Fatalf("the hub saw %d reads, want its instance and its Projects", len(reads))
	}
	for _, c := range reads {
		if c.auth != "Bearer hub-secret" {
			t.Errorf("%s %s carried %q, want the hub's own token", c.method, c.path, c.auth)
		}
	}
}

func TestReadingAHubThatCannotBeAHubIsRefusedBeforeAnythingIsSent(t *testing.T) {
	rig := startEnrollRig(t)
	log := watch(rig.hub, nil)
	for name, c := range map[string]struct{ hub, want string }{
		"the active daemon":         {"spoke", "active daemon cannot be selected as hub"},
		"a local entry":             {"laptop", "local hub targets cannot be used for federation enrollment"},
		"a daemon not in a catalog": {"nowhere", `daemon "nowhere" is not in kata's daemon catalog`},
	} {
		loaded := as[model.FederationHubProjectsLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederationHubProjects{Workspace: t.TempDir(), Request: 1, Hub: c.hub}))
		if !loaded.Early || loaded.Err == nil || !strings.Contains(loaded.Err.Error(), c.want) {
			t.Errorf("%s: %+v, want an early refusal saying %q", name, loaded, c.want)
		}
	}
	t.Setenv("HUB_TOKEN", "")
	unset := as[model.FederationHubProjectsLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederationHubProjects{Workspace: t.TempDir(), Request: 1, Hub: "hub"}))
	if !unset.Early || unset.Err == nil || !strings.Contains(unset.Err.Error(), `token_env "HUB_TOKEN" is unset or empty`) {
		t.Errorf("an unset token_env answered %+v, want an early refusal naming it", unset)
	}
	if n := log.count(); n != 0 {
		t.Errorf("the hub saw %d requests for refused reads", n)
	}
}

func TestAHubThatIsDownIsAFailedReadNotARefusal(t *testing.T) {
	rig := startEnrollRig(t)
	rig.hub.Close()
	loaded := as[model.FederationHubProjectsLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederationHubProjects{Workspace: t.TempDir(), Request: 1, Hub: "hub"}))
	if loaded.Early || loaded.Err == nil {
		t.Errorf("a hub that is down answered %+v, want a failed read that was asked", loaded)
	}
}

func TestBrowsingReadsEveryHubThatIsNeitherActiveNorLocalAndKeepsAFailureInItsOwnRow(t *testing.T) {
	rig := startEnrollRig(t)
	second := startKata(t, "0.20.0")
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	t.Setenv("HUB_TOKEN", "hub-secret")
	rig.writeCatalog(t, fmt.Sprintf(`active_daemon = "spoke"

[[daemon]]
name = "spoke"
url = %q

[[daemon]]
name = "laptop"
local = true

[[daemon]]
name = "hub"
url = %q
allow_insecure = true
token_env = "HUB_TOKEN"

[[daemon]]
name = "gone"
url = %q
allow_insecure = true

[[daemon]]
name = "second"
url = %q
allow_insecure = true
`, rig.spoke.URL, rig.hub.URL, down.URL, second.URL))
	_, acme := project(t, rig.hubGateway(t), "acme")
	_, beta := project(t, newGateway(t, newLocator(second.target())), "beta")

	browsed := as[model.FederationBrowsed](t, run(t, rig.spokeGateway(t), model.BrowseFederationHubs{Workspace: t.TempDir(), Request: 6}))
	if browsed.Err != nil || browsed.Request != 6 || len(browsed.Hubs) != 3 {
		t.Fatalf("browse %+v, want the three hubs that are neither active nor local", browsed)
	}
	var names []string
	for _, h := range browsed.Hubs {
		names = append(names, h.Hub.Name)
	}
	if !slices.Equal(names, []string{"hub", "gone", "second"}) {
		t.Errorf("hubs %v, want file order without spoke and laptop", names)
	}
	hub, gone, other := browsed.Hubs[0], browsed.Hubs[1], browsed.Hubs[2]
	if hub.Err != nil || len(hub.Projects) != 1 || hub.Projects[0].UID != acme.UID || hub.Auth != "token" || hub.Hub.Endpoint != rig.hub.URL {
		t.Errorf("hub row %+v", hub)
	}
	if gone.Err == nil || len(gone.Projects) != 0 || gone.Hub.Name != "gone" {
		t.Errorf("a hub that is down kept %+v, want its failure in its own row", gone)
	}
	if other.Err != nil || len(other.Projects) != 1 || other.Projects[0].UID != beta.UID || other.Auth != "no token" {
		t.Errorf("second row %+v", other)
	}

	rig.writeCatalog(t, fmt.Sprintf("active_daemon = \"spoke\"\n\n[[daemon]]\nname = \"spoke\"\nurl = %q\n", rig.spoke.URL))
	none := as[model.FederationBrowsed](t, run(t, rig.spokeGateway(t), model.BrowseFederationHubs{Workspace: t.TempDir(), Request: 7}))
	if none.Err != nil || len(none.Hubs) != 0 {
		t.Errorf("a catalog with no other hub answered %+v, want no hubs and no error", none)
	}
}

func TestEnrollingANewReplicaEnablesFederationGrantsTheEnrollmentReadsMetadataWithItsTokenAndJoinsTheSpoke(t *testing.T) {
	rig := startEnrollRig(t)
	_, acme := project(t, rig.hubGateway(t), "acme")
	hubLog := watch(rig.hub, nil)

	got := rig.enroll(t, replicaDraft(acme))
	if got.Err != nil || got.Recovery != nil || got.Attempt != 3 {
		t.Fatalf("enrollment %+v, want success for attempt 3", got)
	}
	o := got.Outcome
	if o.Adopted || o.SnapshotCount != 0 || o.Actor != "alice" || o.HubURL != rig.hub.URL || o.HubProjectID != acme.ID || o.HubProjectUID != acme.UID {
		t.Errorf("outcome %+v", o)
	}

	if order := hubLog.order("/federation/enable", "/federation/enrollments", fmt.Sprintf("/projects/%d/federation/metadata", acme.ID)); !slices.Equal(order,
		[]string{"POST /federation/enable", "POST /federation/enrollments", fmt.Sprintf("GET /projects/%d/federation/metadata", acme.ID)}) {
		t.Errorf("the hub saw %v, want enable, enrollment, then the metadata read", order)
	}
	for _, c := range append(hubLog.seen("/federation/enable"), hubLog.seen("/federation/enrollments")...) {
		if c.auth != "Bearer hub-secret" {
			t.Errorf("%s %s carried %q, want the hub's own token", c.method, c.path, c.auth)
		}
	}
	metadata := hubLog.seen(fmt.Sprintf("/projects/%d/federation/metadata", acme.ID))
	if len(metadata) != 1 || metadata[0].auth == "" || metadata[0].auth == "Bearer hub-secret" {
		t.Errorf("the metadata read carried %+v, want the enrollment's token and not the admin one", metadata)
	}

	enrollments := rig.hubEnrollments(t)
	if len(enrollments) != 1 || enrollments[0].SpokeInstanceUID != rig.spokeInstanceUID(t) || enrollments[0].ProjectID != acme.ID || enrollments[0].Actor != "alice" ||
		enrollments[0].RevokedAt != nil || !strings.Contains(enrollments[0].Capabilities, "pull") || !strings.Contains(enrollments[0].Capabilities, "claim") {
		t.Errorf("the hub holds %+v, want one live enrollment of the spoke for acme", enrollments)
	}
	spokes := rig.spokeProjects(t)
	if len(spokes) != 1 || spokes[0].ProjectName != "acme" || spokes[0].HubURL != rig.hub.URL || spokes[0].HubProjectID != acme.ID || !spokes[0].PushEnabled || spokes[0].BoundActor != "alice" ||
		!spokes[0].AllowInsecure {
		t.Errorf("the spoke lists %+v, want acme as a replica of the hub's", spokes)
	}
}

func TestEnrollingAnExistingProjectAdoptsItIntoTheHubProjectOfTheSameNameCreatingItOnce(t *testing.T) {
	rig := startEnrollRig(t)
	spokeGateway := rig.spokeGateway(t)
	workspace, zeta := project(t, spokeGateway, "zeta")
	createIssue(t, spokeGateway, workspace, zeta, "first")
	createIssue(t, spokeGateway, workspace, zeta, "second")
	hubLog := watch(rig.hub, nil)

	draft := model.FederationEnrollDraft{Operation: model.FederationAdoptSameName, SpokeProjectID: zeta.ID, SpokeProjectName: "zeta", AdoptExisting: true, Hub: model.FederationHub{Name: "hub"},
		HubProjectName: "zeta", RequestedActor: "alice", APICapabilities: "claim,pull,push", DisplayCapabilities: "pull,push,lease", PushEnabled: true, AllowInsecure: true}
	got := rig.enroll(t, draft)
	if got.Err != nil || got.Recovery != nil {
		t.Fatalf("adoption %+v", got)
	}
	if !got.Outcome.Adopted || got.Outcome.SnapshotCount < 2 || got.Outcome.HubProjectID == 0 {
		t.Errorf("outcome %+v, want an adoption that wrote a snapshot for each Issue", got.Outcome)
	}
	if order := hubLog.order("/api/v1/projects", "/federation/enable", "/federation/enrollments"); len(order) < 3 || order[0] != "POST /api/v1/projects" {
		t.Errorf("the hub saw %v, want the Project ensured first", order)
	}
	spokes := rig.spokeProjects(t)
	if len(spokes) != 1 || spokes[0].ProjectName != "zeta" || spokes[0].HubProjectID != got.Outcome.HubProjectID {
		t.Errorf("the spoke lists %+v, want zeta bound to the hub's new Project", spokes)
	}
	var hubZeta []g.ProjectOut
	listed, err := rig.hub.target().Client.ListProjectsWithResponse(context.Background(), nil)
	if err != nil || listed.JSON200 == nil {
		t.Fatalf("list hub projects: %v", err)
	}
	for _, p := range listed.JSON200.Projects {
		if p.Name == "zeta" {
			hubZeta = append(hubZeta, p)
		}
	}
	if len(hubZeta) != 1 || hubZeta[0].ID != got.Outcome.HubProjectID {
		t.Errorf("the hub holds %+v, want one zeta, the one enrolled", hubZeta)
	}
}

func TestEnrollingIntoAHubProjectThatIsChosenEnsuresNothing(t *testing.T) {
	rig := startEnrollRig(t)
	_, acme := project(t, rig.hubGateway(t), "acme")
	spokeGateway := rig.spokeGateway(t)
	workspace, local := project(t, spokeGateway, "zeta")
	createIssue(t, spokeGateway, workspace, local, "only")
	hubLog := watch(rig.hub, nil)

	draft := model.FederationEnrollDraft{Operation: model.FederationAdoptSelectedHub, SpokeProjectID: local.ID, SpokeProjectName: "zeta", AdoptExisting: true, Hub: model.FederationHub{Name: "hub"},
		HubProjectID: acme.ID, HubProjectName: "acme", RequestedActor: "alice", APICapabilities: "claim,pull,push", PushEnabled: true, AllowInsecure: true}
	got := rig.enroll(t, draft)
	if got.Err != nil || !got.Outcome.Adopted || got.Outcome.HubProjectID != acme.ID {
		t.Fatalf("adoption %+v", got)
	}
	if init := hubLog.seen("/api/v1/projects"); len(init) != 0 {
		t.Errorf("the hub saw %+v, want no Project ensured for a chosen one", init)
	}
	if spokes := rig.spokeProjects(t); len(spokes) != 1 || spokes[0].ProjectName != "zeta" || spokes[0].HubProjectID != acme.ID {
		t.Errorf("the spoke lists %+v, want the local Project keeping its name, bound to acme", spokes)
	}
}

func TestAFailureReadingMetadataAfterTheGrantKeepsTheTokenAndWhatTheJoinNeeds(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		guess  bool
	}{"a server failure": {http.StatusBadGateway, false}, "a refused token": {http.StatusUnauthorized, true}} {
		rig := startEnrollRig(t)
		_, acme := project(t, rig.hubGateway(t), "acme")
		refuse := refuseWith(c.status, "hub_says_no", "the hub said no")
		hubLog := watch(rig.hub, func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, fmt.Sprintf("/projects/%d/federation/metadata", acme.ID)) {
				refuse(w, r)
				return true
			}
			return false
		})
		t.Setenv("KATA_AUTH_TOKEN", "spoke-secret")

		got := rig.enroll(t, replicaDraft(acme))
		rec := got.Recovery
		if got.Err == nil || rec == nil || got.Attempt != 3 {
			t.Fatalf("%s: %+v, want a failure with its recovery", name, got)
		}
		if !strings.HasPrefix(got.Err.Error(), "hub hub: enrollment metadata fetch failed: the hub said no") || rec.Err.Error() != got.Err.Error() {
			t.Errorf("%s: error %q, want the metadata failure in kata tui's words", name, got.Err)
		}
		enrollments := rig.hubEnrollments(t)
		if len(enrollments) != 1 || enrollments[0].RevokedAt != nil {
			t.Fatalf("%s: the hub holds %+v, want the grant left in place", name, enrollments)
		}
		if rec.Stage != "metadata" || rec.HubName != "hub" || rec.SpokeName != "spoke" || rec.SpokeEndpoint != rig.spoke.URL || rec.TokenGuess != c.guess {
			t.Errorf("%s: recovery %+v", name, rec)
		}
		command := rec.Command
		metadataAuth := hubLog.seen(fmt.Sprintf("/projects/%d/federation/metadata", acme.ID))[0].auth
		if command.Token == "" || metadataAuth != "Bearer "+command.Token || command.HubURL != rig.hub.URL || command.HubProjectID != acme.ID || command.ProjectName != "acme" ||
			command.Actor != "alice" || command.Capabilities != "claim,pull,push" || !command.PushEnabled || !command.AllowInsecure || command.AdoptExisting ||
			command.SpokeToken != "spoke-secret" || !command.SpokeAllowInsecure || command.HubProjectUID != "" {
			t.Errorf("%s: command %+v, want the enrollment's token and the draft's settings", name, command)
		}
		if line := command.String(); !strings.HasPrefix(line, "KATA_SERVER="+rig.spoke.URL+" KATA_ALLOW_INSECURE=1 KATA_AUTH_TOKEN=spoke-secret kata federation join --hub-url "+rig.hub.URL) {
			t.Errorf("%s: command line %q", name, line)
		}
		if spokes := rig.spokeProjects(t); len(spokes) != 0 {
			t.Errorf("%s: the spoke lists %+v after a failure before the join", name, spokes)
		}
	}
}

func TestAFailureJoiningTheSpokeAfterTheGrantKeepsTheMetadataInTheCommand(t *testing.T) {
	rig := startEnrollRig(t)
	_, acme := project(t, rig.hubGateway(t), "acme")
	refuse := refuseWith(http.StatusConflict, "project_exists", "local project already exists")
	watch(rig.spoke, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/federation/replicas") {
			refuse(w, r)
			return true
		}
		return false
	})

	got := rig.enroll(t, replicaDraft(acme))
	rec := got.Recovery
	if got.Err == nil || rec == nil {
		t.Fatalf("%+v, want a failure with its recovery", got)
	}
	if got.Err.Error() != "spoke: join failed: local project already exists" || rec.Stage != "join" || rec.TokenGuess {
		t.Errorf("error %q, recovery %+v", got.Err, rec)
	}
	command := rec.Command
	if command.HubProjectUID != acme.UID || command.ProjectName != "acme" || command.ReplayHorizonEventID == 0 && command.BaselineThroughEventID == 0 && command.HubProjectUID == "" {
		t.Errorf("command %+v, want the hub Project's uid and the metadata it read", command)
	}
	if line := command.String(); !strings.Contains(line, "--hub-project-uid "+acme.UID) || !strings.Contains(line, "--token "+command.Token) {
		t.Errorf("command line %q", line)
	}
	if enrollments := rig.hubEnrollments(t); len(enrollments) != 1 || enrollments[0].RevokedAt != nil {
		t.Errorf("the hub holds %+v, want the grant left in place", enrollments)
	}
}

func TestARunThatIsRefusedBeforeTheHubGrantedKeepsNoRecoveryAndSendsNothing(t *testing.T) {
	rig := startEnrollRig(t)
	_, acme := project(t, rig.hubGateway(t), "acme")
	hubLog := watch(rig.hub, nil)
	for name, c := range map[string]struct {
		change func(*model.FederationEnrollDraft)
		uid    string
		want   string
	}{
		"the active daemon as hub":  {func(d *model.FederationEnrollDraft) { d.Hub.Name = "spoke" }, "", "active daemon cannot be selected as hub"},
		"a local hub":               {func(d *model.FederationEnrollDraft) { d.Hub.Name = "laptop" }, "", "local hub targets cannot be used"},
		"a hub not in the catalog":  {func(d *model.FederationEnrollDraft) { d.Hub.Name = "nowhere" }, "", `daemon "nowhere" is not in kata's daemon catalog`},
		"an unknown spoke instance": {func(*model.FederationEnrollDraft) {}, " ", "spoke instance UID is not loaded; refresh federation status before enrollment"},
	} {
		draft := replicaDraft(acme)
		c.change(&draft)
		uid := c.uid
		if uid == "" {
			uid = rig.spokeInstanceUID(t)
		}
		got := as[model.FederationEnrolled](t, run(t, rig.spokeGateway(t), model.EnrollFederation{Workspace: t.TempDir(), Attempt: 9, InstanceUID: uid, Draft: draft}))
		if got.Err == nil || !strings.Contains(got.Err.Error(), c.want) || got.Recovery != nil || got.Attempt != 9 {
			t.Errorf("%s: %+v, want the refusal %q and no recovery", name, got, c.want)
		}
	}
	t.Setenv("HUB_TOKEN", "")
	unset := rig.enroll(t, replicaDraft(acme))
	if unset.Err == nil || !strings.Contains(unset.Err.Error(), `token_env "HUB_TOKEN" is unset or empty`) || unset.Recovery != nil {
		t.Errorf("an unset token_env gave %+v", unset)
	}
	if n := hubLog.count(); n != 0 {
		t.Errorf("the hub saw %d requests for refused runs", n)
	}
	if enrollments := rig.hubEnrollments(t); len(enrollments) != 0 {
		t.Errorf("the hub holds %+v after refused runs", enrollments)
	}
}

func TestARunThatFailsOnTheHubBeforeTheGrantGrantsNothingAndKeepsNoRecovery(t *testing.T) {
	rig := startEnrollRig(t)
	draft := replicaDraft(model.Project{ID: 4242, Name: "ghost"})
	got := rig.enroll(t, draft)
	if got.Err == nil || got.Recovery != nil {
		t.Fatalf("enrolling into a hub Project that is not there gave %+v, want a plain failure", got)
	}
	if enrollments := rig.hubEnrollments(t); len(enrollments) != 0 {
		t.Errorf("the hub holds %+v", enrollments)
	}
	if spokes := rig.spokeProjects(t); len(spokes) != 0 {
		t.Errorf("the spoke lists %+v", spokes)
	}
}

func TestAnEnrollmentWhenNoDaemonIsFoundFailsWithItsOwnAttempt(t *testing.T) {
	rig := startEnrollRig(t)
	_, acme := project(t, rig.hubGateway(t), "acme")
	locator := newLocator(rig.spoke.target())
	boom := errors.New("no daemon")
	locator.err.Store(&boom)
	got := as[model.FederationEnrolled](t, run(t, newGateway(t, locator), model.EnrollFederation{Workspace: t.TempDir(), Attempt: 5, InstanceUID: "X", Draft: replicaDraft(acme)}))
	if !errors.Is(got.Err, boom) || got.Attempt != 5 {
		t.Errorf("answer %+v, want the daemon's absence for attempt 5", got)
	}
}

func TestTheStatusReadKeepsEveryBindingWhateverItsRole(t *testing.T) {
	rig := startEnrollRig(t)
	_, acme := project(t, rig.hubGateway(t), "acme")
	if got := rig.enroll(t, replicaDraft(acme)); got.Err != nil {
		t.Fatalf("enrollment %+v", got)
	}
	hub := as[model.FederationLoaded](t, run(t, rig.hubGateway(t), model.LoadFederation{Workspace: t.TempDir(), Request: 1}))
	if hub.Err != nil || len(hub.Spokes) != 0 || !slices.Contains(hub.Bindings, model.FederationBinding{ProjectID: acme.ID, ProjectName: "acme", Role: "hub"}) {
		t.Errorf("the hub's status %+v, want acme bound as a hub and no spoke rows", hub)
	}
	spoke := as[model.FederationLoaded](t, run(t, rig.spokeGateway(t), model.LoadFederation{Workspace: t.TempDir(), Request: 1}))
	if len(spoke.Spokes) != 1 || len(spoke.Bindings) != 1 || spoke.Bindings[0].Role != "spoke" || spoke.Bindings[0].ProjectID != spoke.Spokes[0].ProjectID || spoke.Bindings[0].ProjectName != "acme" {
		t.Errorf("the spoke's status %+v, want acme bound as a spoke", spoke)
	}
}

// After a switch the connected daemon is the one every hub choice, exclusion and recovery command reads, not the entry
// active_daemon still names in the file.
func TestAfterASwitchTheHubStepsAndRecoveryFollowTheConnectedDaemon(t *testing.T) {
	rig := startEnrollRig(t)
	t.Setenv("KATA_AUTH_TOKEN", "")
	gateway := rig.spokeGateway(t)
	workspace := t.TempDir()
	if answer := switched(t, gateway, workspace, "hub"); answer.Err != nil {
		t.Fatal(answer.Err)
	}

	listed := as[model.FederationHubsLoaded](t, run(t, gateway, model.LoadFederationHubs{Workspace: workspace, Request: 1}))
	var active []string
	for _, hub := range listed.Hubs {
		if hub.Active {
			active = append(active, hub.Name)
		}
	}
	if !slices.Equal(active, []string{"hub"}) {
		t.Errorf("hubs marked active after the switch: %v, want the connected daemon only", active)
	}
	browsed := as[model.FederationBrowsed](t, run(t, gateway, model.BrowseFederationHubs{Workspace: workspace, Request: 2}))
	var read []string
	for _, row := range browsed.Hubs {
		read = append(read, row.Hub.Name)
	}
	if !slices.Equal(read, []string{"spoke"}) {
		t.Errorf("browsing after the switch read %v, want the previous daemon as a hub and not the connected one", read)
	}
	refused := as[model.FederationHubProjectsLoaded](t, run(t, gateway, model.LoadFederationHubProjects{Workspace: workspace, Request: 3, Hub: "hub"}))
	if !refused.Early || refused.Err == nil || !strings.Contains(refused.Err.Error(), "active daemon cannot be selected as hub") {
		t.Errorf("the connected daemon as a hub answered %+v, want a refusal", refused)
	}

	// Enrolling into the previous daemon, the recovery command carries the connected daemon's token, not the file's active entry's.
	_, acme := project(t, newGateway(t, newLocator(rig.spoke.target())), "acme")
	watch(rig.spoke, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, fmt.Sprintf("/projects/%d/federation/metadata", acme.ID)) {
			refuseWith(http.StatusBadGateway, "hub_says_no", "the hub said no")(w, r)
			return true
		}
		return false
	})
	instance, err := rig.hub.target().Client.InstanceWithResponse(context.Background())
	if err != nil || instance.JSON200 == nil {
		t.Fatalf("instance: %v", err)
	}
	draft := replicaDraft(acme)
	draft.Hub = model.FederationHub{Name: "spoke"}
	got := as[model.FederationEnrolled](t, run(t, gateway, model.EnrollFederation{Workspace: workspace, Attempt: 3, InstanceUID: instance.JSON200.InstanceUID, Draft: draft}))
	if got.Recovery == nil {
		t.Fatalf("enrollment answered %+v, want a failure with its recovery", got)
	}
	command := got.Recovery.Command
	if got.Recovery.SpokeName != "hub" || command.SpokeToken != "hub-secret" || !command.SpokeAllowInsecure {
		t.Errorf("recovery for spoke %q with token %q: want the connected daemon hub and its own token", got.Recovery.SpokeName, command.SpokeToken)
	}
}

func TestTheRecoveryCommandOfALocalSpokeNamesTheCatalogEntryItWasReachedThrough(t *testing.T) {
	// A plain `local = true` entry (no home/instance_uid) is never connected through active_daemon: kata falls through to the default
	// local daemon. Only a switch in the Pane (or a pin) reaches it by name.
	for name, c := range map[string]struct {
		active, switched string
		want             string
	}{
		"a local entry the Pane switched to":   {"", "laptop", "kata --daemon laptop federation join"},
		"a plain local entry as active_daemon": {"laptop", "", "kata federation join"},
		"plain discovery":                      {"", "", "kata federation join"},
	} {
		rig := startEnrollRig(t)
		_, acme := project(t, rig.hubGateway(t), "acme")
		refuse := refuseWith(http.StatusBadGateway, "hub_says_no", "the hub said no")
		watch(rig.hub, func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, fmt.Sprintf("/projects/%d/federation/metadata", acme.ID)) {
				refuse(w, r)
				return true
			}
			return false
		})
		active := ""
		if c.active != "" {
			active = fmt.Sprintf("active_daemon = %q\n", c.active)
		}
		rig.writeCatalog(t, active+fmt.Sprintf("\n[[daemon]]\nname = \"laptop\"\nlocal = true\n\n[[daemon]]\nname = \"hub\"\nurl = %q\nallow_insecure = true\ntoken_env = \"HUB_TOKEN\"\n", rig.hub.URL))
		gateway := newGateway(t, newLocator(rig.spoke.target()))
		gateway.mu.Lock()
		gateway.current = c.switched
		gateway.mu.Unlock()
		got := as[model.FederationEnrolled](t, run(t, gateway, model.EnrollFederation{Workspace: t.TempDir(), Attempt: 3, InstanceUID: rig.spokeInstanceUID(t), Draft: replicaDraft(acme)}))
		if got.Recovery == nil {
			t.Fatalf("%s: %+v, want a recovery", name, got)
		}
		if line := got.Recovery.Command.String(); !strings.Contains(line, c.want) || strings.Contains(line, "KATA_SERVER") {
			t.Errorf("%s: command line %q, want it to hold %q", name, line, c.want)
		}
	}
}
