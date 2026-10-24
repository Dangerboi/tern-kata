package data

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// federatedPair is two in-process kata daemons: a hub with a federated Project called acme, and a spoke holding a replica of it that
// the hub has granted an enrollment. The test service runs both over HTTP; the replica is made by the join call, which does not
// contact the hub, so no events flow between them.
type federatedPair struct {
	hub, spoke *kataServer
	// hubProjectID is acme on the hub; enrollmentID is the hub's grant to the spoke.
	hubProjectID, enrollmentID int64
	spokeInstanceUID           string
	// spokeProject is the replica's id on the spoke.
	spokeProject int64
}

func startPair(t *testing.T) federatedPair {
	t.Helper()
	ctx := context.Background()
	pair := federatedPair{hub: startKata(t, "0.20.0"), spoke: startKata(t, "0.20.0")}
	hub, spoke := pair.hub.target().Client, pair.spoke.target().Client

	instance, err := spoke.InstanceWithResponse(ctx)
	if err != nil || instance.JSON200 == nil {
		t.Fatalf("spoke instance: %v", err)
	}
	pair.spokeInstanceUID = instance.JSON200.InstanceUID

	hubGateway := newGateway(t, newLocator(pair.hub.target()))
	_, acme := project(t, hubGateway, "acme")
	pair.hubProjectID = acme.ID
	if _, err := hub.EnableProjectFederationWithResponse(ctx, &g.EnableProjectFederationRequestOptions{
		PathParams: &g.EnableProjectFederationPath{ProjectID: acme.ID}, Body: &g.EnableProjectFederationBody{Actor: new("alice")}}); err != nil {
		t.Fatalf("enable federation on the hub: %v", err)
	}
	enrolled, err := hub.CreateFederationEnrollmentWithResponse(ctx, &g.CreateFederationEnrollmentRequestOptions{Body: &g.CreateFederationEnrollmentBody{
		Actor: new("alice"), Capabilities: "pull,push", ProjectID: acme.ID, SpokeInstanceUID: pair.spokeInstanceUID}})
	if err != nil || enrolled.JSON200 == nil {
		t.Fatalf("create the enrollment: %v", err)
	}
	pair.enrollmentID = enrolled.JSON200.ID

	body := g.CreateFederationReplicaBody{ProjectName: "acme", HubURL: pair.hub.URL, HubProjectID: acme.ID, HubProjectUID: acme.UID, ReplayHorizonEventID: 1,
		Actor: new("alice"), Token: new("hub-token"), Capabilities: new("pull,push"), AllowInsecure: new(true)}
	if _, err := spoke.CreateFederationReplicaWithResponse(ctx, &g.CreateFederationReplicaRequestOptions{Body: &body}); err != nil {
		t.Fatalf("join the spoke: %v", err)
	}
	status := as[model.FederationLoaded](t, run(t, newGateway(t, newLocator(pair.spoke.target())), model.LoadFederation{Workspace: t.TempDir(), Request: 1}))
	if status.Err != nil || len(status.Spokes) != 1 {
		t.Fatalf("spoke status %+v, want the one replica", status)
	}
	pair.spokeProject = status.Spokes[0].ProjectID
	return pair
}

func (p federatedPair) draft() model.FederationLeaveDraft {
	return model.FederationLeaveDraft{ProjectID: p.spokeProject, ProjectName: "acme", HubURL: p.hub.URL, HubProjectID: p.hubProjectID,
		InstanceUID: p.spokeInstanceUID, AllowInsecure: true, Actor: "dana"}
}

func (p federatedPair) leave(t *testing.T, draft model.FederationLeaveDraft) model.FederationLeft {
	t.Helper()
	return as[model.FederationLeft](t, run(t, newGateway(t, newLocator(p.spoke.target())), model.LeaveFederation{Workspace: t.TempDir(), Attempt: 7, Draft: draft}))
}

// enrollments is what the hub lists as grants: id to whether it is revoked.
func (p federatedPair) enrollments(t *testing.T) map[int64]bool {
	t.Helper()
	listed, err := p.hub.target().Client.ListFederationEnrollmentsWithResponse(context.Background())
	if err != nil || listed.JSON200 == nil {
		t.Fatalf("list enrollments: %v", err)
	}
	out := map[int64]bool{}
	for _, e := range listed.JSON200.Enrollments {
		out[e.ID] = e.RevokedAt != nil
	}
	return out
}

func (p federatedPair) spokes(t *testing.T) []model.FederationSpoke {
	t.Helper()
	return as[model.FederationLoaded](t, run(t, newGateway(t, newLocator(p.spoke.target())), model.LoadFederation{Workspace: t.TempDir(), Request: 1})).Spokes
}

func TestLeaveRevokesTheSpokesGrantOnTheHubThenDetachesTheReplica(t *testing.T) {
	pair := startPair(t)
	left := pair.leave(t, pair.draft())
	if left.Err != nil || left.Attempt != 7 {
		t.Fatalf("leave %+v, want attempt 7 and no error", left)
	}
	if o := left.Outcome; o.Revoked != 1 || o.Archived || o.SkippedRevoke || o.ProviderReleased || len(o.GlobalEnrollmentIDs) != 0 || o.Draft.ProjectName != "acme" {
		t.Errorf("outcome %+v, want one revoke, detached", o)
	}
	if revoked, ok := pair.enrollments(t)[pair.enrollmentID]; !ok || !revoked {
		t.Errorf("hub enrollments %v, want %d revoked", pair.enrollments(t), pair.enrollmentID)
	}
	if spokes := pair.spokes(t); len(spokes) != 0 {
		t.Errorf("spoke still lists %+v after the leave", spokes)
	}
	// Detached: the Project stays on the spoke, standalone.
	projects, err := pair.spoke.target().Client.ListProjectsWithResponse(context.Background(), nil)
	if err != nil || projects.JSON200 == nil {
		t.Fatalf("list projects: %v", err)
	}
	kept := false
	for _, p := range projects.JSON200.Projects {
		kept = kept || p.Name == "acme"
	}
	if !kept {
		t.Error("detaching removed the Project from the spoke")
	}
}

func TestLeaveArchivesTheReplicaWhenAsked(t *testing.T) {
	pair := startPair(t)
	draft := pair.draft()
	draft.Archive = true
	left := pair.leave(t, draft)
	if left.Err != nil || !left.Outcome.Archived || left.Outcome.Revoked != 1 {
		t.Fatalf("leave %+v, want an archive after one revoke", left)
	}
	if spokes := pair.spokes(t); len(spokes) != 0 {
		t.Errorf("spoke still lists %+v after the archive", spokes)
	}
}

func TestLocalOnlyLeaveTouchesNothingOnTheHub(t *testing.T) {
	pair := startPair(t)
	// A hub that answers nothing proves the hub is not asked.
	refuse := func(w http.ResponseWriter, r *http.Request) bool {
		t.Errorf("a local-only leave sent %s %s to the hub", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		return true
	}
	pair.hub.intercept.Store(&refuse)
	draft := pair.draft()
	draft.LocalOnly = true
	left := pair.leave(t, draft)
	if left.Err != nil || !left.Outcome.SkippedRevoke || left.Outcome.Revoked != 0 {
		t.Fatalf("leave %+v, want a local-only leave with no revoke", left)
	}
	pair.hub.intercept.Store(nil)
	if revoked := pair.enrollments(t)[pair.enrollmentID]; revoked {
		t.Error("a local-only leave revoked the hub's grant")
	}
	if spokes := pair.spokes(t); len(spokes) != 0 {
		t.Errorf("spoke still lists %+v", spokes)
	}
}

func TestLeaveStopsBeforeTheHubWhenThePreflightIsRefused(t *testing.T) {
	pair := startPair(t)
	// A Project the spoke no longer has (the status the Pane showed is old) is refused by the preflight, before anything is revoked.
	hubAsked := false
	spy := func(http.ResponseWriter, *http.Request) bool { hubAsked = true; return false }
	pair.hub.intercept.Store(&spy)
	draft := pair.draft()
	draft.ProjectID = pair.spokeProject + 100
	left := pair.leave(t, draft)
	if left.Err == nil || !strings.HasPrefix(left.Err.Error(), "spoke: leave preflight failed: ") {
		t.Fatalf("leave error %v, want kata's preflight refusal", left.Err)
	}
	if hubAsked {
		t.Error("the hub was asked after a refused preflight")
	}
	if left.Attempt != 7 {
		t.Errorf("failure answers attempt %d, want 7", left.Attempt)
	}
	if pair.enrollments(t)[pair.enrollmentID] {
		t.Error("a refused preflight revoked the grant")
	}
}

func TestLeaveReportsAnUnreachableOrRefusingHubWithTheLocalOnlyAdvice(t *testing.T) {
	pair := startPair(t)
	refuse := func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/federation/enrollments" {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":401,"error":{"code":"unauthorized","message":"token required"}}`))
		return true
	}
	pair.hub.intercept.Store(&refuse)
	left := pair.leave(t, pair.draft())
	want := "hub revoke failed: token required; retry with local-only to tear down locally, then revoke on the hub later"
	if left.Err == nil || left.Err.Error() != want {
		t.Fatalf("leave error %v, want %q", left.Err, want)
	}
	if spokes := pair.spokes(t); len(spokes) != 1 {
		t.Errorf("spoke lists %+v after a failed hub step, want the replica still joined", spokes)
	}

	pair.hub.intercept.Store(nil)
	draft := pair.draft()
	draft.HubURL = "http://127.0.0.1:1"
	if gone := pair.leave(t, draft); gone.Err == nil || !strings.HasPrefix(gone.Err.Error(), "hub revoke failed: ") ||
		!strings.HasSuffix(gone.Err.Error(), "; retry with local-only to tear down locally, then revoke on the hub later") {
		t.Errorf("an unreachable hub gave %v", gone.Err)
	}
	if spokes := pair.spokes(t); len(spokes) != 1 {
		t.Errorf("spoke lists %+v after an unreachable hub", spokes)
	}
}

func TestLeaveRefusesWithoutTheSpokesInstanceUIDBeforeRevoking(t *testing.T) {
	pair := startPair(t)
	draft := pair.draft()
	draft.InstanceUID = "  "
	left := pair.leave(t, draft)
	if left.Err == nil || left.Err.Error() != "spoke instance UID is not loaded; refresh federation status before leaving" {
		t.Fatalf("leave error %v", left.Err)
	}
	if pair.enrollments(t)[pair.enrollmentID] {
		t.Error("a refused leave revoked the grant")
	}
}

func TestLeaveBlocksOnAGrantForAnotherInstanceAndAdvisesTheHubCommand(t *testing.T) {
	pair := startPair(t)
	draft := pair.draft()
	draft.InstanceUID = "01HZNQ7VFPK1XGD8R5MABCD4EX"
	left := pair.leave(t, draft)
	if left.Err == nil {
		t.Fatal("a leave went ahead with only another instance's grant")
	}
	for _, want := range []string{"no active enrollment matches this spoke's instance UID", "enrollment(s) #" + strconv.FormatInt(pair.enrollmentID, 10) + " still authorize the hub project",
		"`kata federation revoke <id>`", "retry with local-only"} {
		if !strings.Contains(left.Err.Error(), want) {
			t.Errorf("error %q lacks %q", left.Err, want)
		}
	}
	if pair.enrollments(t)[pair.enrollmentID] {
		t.Error("the other instance's grant was revoked")
	}
	if spokes := pair.spokes(t); len(spokes) != 1 {
		t.Errorf("spoke lists %+v, want it still joined", spokes)
	}
}

func TestLeaveRevokesTheHubGrantWithTheTokenOfTheCatalogEntryForTheHubsURL(t *testing.T) {
	pair := startPair(t)
	var bearer string
	spy := func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/federation/enrollments" {
			bearer = r.Header.Get("Authorization")
		}
		return false
	}
	pair.hub.intercept.Store(&spy)
	t.Setenv("HUB_ADMIN_TOKEN", "hub-secret")
	config := "[[daemon]]\nname = \"hub\"\nurl = \"" + pair.hub.URL + "/\"\ntoken_env = \"HUB_ADMIN_TOKEN\"\n\n" +
		"[[daemon]]\nname = \"elsewhere\"\nurl = \"https://other.example\"\ntoken_env = \"OTHER_TOKEN\"\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	draft := pair.draft()
	if left := pair.leave(t, draft); left.Err != nil || left.Outcome.Revoked != 1 {
		t.Fatalf("leave %+v", left)
	}
	if bearer != "Bearer hub-secret" {
		t.Errorf("the hub saw %q, want the catalog entry's token despite the trailing slash", bearer)
	}
}

func TestLeaveSendsNoTokenToAHubTheCatalogDoesNotName(t *testing.T) {
	pair := startPair(t)
	var bearer = "unset"
	spy := func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/federation/enrollments" {
			bearer = r.Header.Get("Authorization")
		}
		return false
	}
	pair.hub.intercept.Store(&spy)
	t.Setenv("KATA_AUTH_TOKEN", "global-secret")
	t.Setenv("OTHER_TOKEN", "other-secret")
	config := "[[daemon]]\nname = \"elsewhere\"\nurl = \"https://other.example\"\ntoken_env = \"OTHER_TOKEN\"\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if left := pair.leave(t, pair.draft()); left.Err != nil {
		t.Fatalf("leave %+v", left)
	}
	if bearer != "" {
		t.Errorf("the hub saw authorization %q, want none", bearer)
	}
}

func TestLeaveNamesAMissingCatalogTokenAsAHubFailure(t *testing.T) {
	pair := startPair(t)
	config := "[[daemon]]\nname = \"hub\"\nurl = \"" + pair.hub.URL + "\"\ntoken_env = \"HUB_TOKEN_NOT_SET\"\n"
	if err := os.WriteFile(filepath.Join(os.Getenv("KATA_HOME"), "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	left := pair.leave(t, pair.draft())
	if left.Err == nil || !strings.HasPrefix(left.Err.Error(), "hub revoke failed: daemon \"hub\": token_env \"HUB_TOKEN_NOT_SET\" is unset or empty; retry with local-only") {
		t.Errorf("leave error %v", left.Err)
	}
}

func TestMatchLeaveEnrollmentsSortsGrantsAsKataDoes(t *testing.T) {
	revoked := func(id int64, project int64, instance string) g.FederationEnrollmentOut {
		e := g.FederationEnrollmentOut{ID: id, ProjectID: project, SpokeInstanceUID: instance}
		e.RevokedAt = &e.CreatedAt
		return e
	}
	active := func(id int64, project int64, instance string) g.FederationEnrollmentOut {
		return g.FederationEnrollmentOut{ID: id, ProjectID: project, SpokeInstanceUID: instance}
	}
	ids, globals, foreign := matchLeaveEnrollments([]g.FederationEnrollmentOut{
		active(1, 12, "me"), revoked(2, 12, "me"), active(3, 0, "me"), active(4, 0, "you"), active(5, 99, "me"), active(6, 12, "you"), active(7, 12, "me"),
	}, "me", 12)
	if got := enrollmentIDs(ids); got != "#1, #7" {
		t.Errorf("revoked ids %s, want #1, #7", got)
	}
	if got := enrollmentIDs(globals); got != "#3" {
		t.Errorf("global ids %s, want #3 (another instance's global grant is not this spoke's)", got)
	}
	if got := enrollmentIDs(foreign); got != "#6" {
		t.Errorf("foreign ids %s, want #6", got)
	}
}
