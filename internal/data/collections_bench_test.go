package data

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
)

// seedWords make each seeded Issue's title and body its own, so kata's look-alike check lets every create through.
var seedWords = strings.Fields("parser lane board daemon cursor snapshot render label owner token worker cron job " +
	"stream detail comment link parent child blocker footer column filter search picker card field editor reply " +
	"history event catalog project status priority deadline schedule calendar zone timer import export queue retry")

// seedPhrase is the given number of words, picked from seedWords by i: no two i below a few million share one.
func seedPhrase(i, words int) string {
	picked := make([]string, words)
	x := uint64(i)*2654435761 + 1
	for w := range picked {
		x = x*6364136223846793005 + 1442695040888963407
		picked[w] = seedWords[(x>>33)%uint64(len(seedWords))]
	}
	return strings.Join(picked, " ")
}

// seedSnapshot is a Project on kata holding n Issues shaped like a working one: most in a Lane, some with other labels, an
// owner or a priority, every tenth the parent of the nine after it. It returns the workspace and the read the Pane sends for
// the Project's list.
func seedSnapshot(b *testing.B, gateway *Gateway, n int) (string, model.ReadSnapshot) {
	b.Helper()
	workspace, p := project(b, gateway, "work")
	lanes := model.DefaultLanes()
	var parent string
	for i := range n {
		cmd := model.CreateIssueCmd{Workspace: workspace, Actor: "tester", ProjectID: p.ID, Key: fmt.Sprintf("seed-%d", i),
			Title: fmt.Sprintf("%d %s", i, seedPhrase(i, 7)), Body: seedPhrase(i+n, 30)}
		if i%5 != 0 {
			cmd.Labels = append(cmd.Labels, lanes[i%len(lanes)])
		}
		if i%3 == 0 {
			cmd.Labels = append(cmd.Labels, "bug")
		}
		if i%3 == 1 {
			cmd.Owner = "agent"
		}
		if i%2 == 0 {
			cmd.Priority = new(int64(i % 5))
		}
		if i%10 != 0 {
			cmd.ParentUID = parent
		}
		created := as[model.IssueCreated](b, run(b, gateway, cmd))
		if created.Err != nil {
			b.Fatal(created.Err)
		}
		if i%10 == 0 {
			parent = created.UID
		}
	}
	query := model.CollectionQuery(model.ProjectCollection(p.UID, p.Name), model.Filters{})
	return workspace, model.ReadSnapshot{Workspace: workspace, Query: query}
}

// unixTarget serves kata's handler on a Unix socket as well, the way a local daemon is reached, and is a Target for it.
func unixTarget(b *testing.B, kata *kataServer) Target {
	b.Helper()
	socket := filepath.Join(b.TempDir(), "kata.sock")
	listener, err := new(net.ListenConfig).Listen(b.Context(), "unix", socket)
	if err != nil {
		b.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(kata.Config.Handler)
	srv.Listener = listener
	srv.Start()
	b.Cleanup(srv.Close)
	c, err := client.New("unix://" + socket)
	if err != nil {
		b.Fatal(err)
	}
	return Target{Client: c}
}

// BenchmarkReadSnapshot is the Collection read the Pane sends on its first load and every live refresh, through the gateway
// as model commands run, over a Unix socket (how a local daemon is reached) and over TCP. "kata" answers from kata's own
// service, so it includes kata's database work and encoding; "recorded" replays that answer's bytes, which leaves the round
// trip, the client's decode and the translation to the model. body-B is the answer's size on the wire.
func BenchmarkReadSnapshot(b *testing.B) {
	for _, n := range []int{50, 500, 2000} {
		kata := startKata(b, "0.20.0")
		_, read := seedSnapshot(b, newGateway(b, newLocator(kata.target())), n)
		for _, transport := range []string{"unix", "tcp"} {
			target := kata.target()
			if transport == "unix" {
				target = unixTarget(b, kata)
			}
			gateway := newGateway(b, newLocator(target))
			var recorded atomic.Pointer[httptest.ResponseRecorder]
			replay := func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/api/v1/ui/snapshot" {
					return false
				}
				answer := recorded.Load()
				if answer == nil {
					answer = httptest.NewRecorder()
					kata.handler.ServeHTTP(answer, r)
					recorded.Store(answer)
				}
				for key, values := range answer.Header() {
					w.Header()[key] = values
				}
				w.WriteHeader(answer.Code)
				_, _ = w.Write(answer.Body.Bytes())
				return true
			}
			check := func(b *testing.B) {
				loaded := as[model.SnapshotLoaded](b, run(b, gateway, read))
				if len(loaded.Snapshot.Issues) != n {
					b.Fatalf("read %d Issues, want %d", len(loaded.Snapshot.Issues), n)
				}
			}
			b.Run(fmt.Sprintf("transport=%s/source=kata/n=%d", transport, n), func(b *testing.B) {
				check(b)
				b.ReportAllocs()
				for b.Loop() {
					run(b, gateway, read)
				}
			})
			b.Run(fmt.Sprintf("transport=%s/source=recorded/n=%d", transport, n), func(b *testing.B) {
				kata.intercept.Store(&replay)
				defer kata.intercept.Store(nil)
				check(b)
				b.ReportAllocs()
				for b.Loop() {
					run(b, gateway, read)
				}
				b.ReportMetric(float64(recorded.Load().Body.Len()), "body-B")
			})
		}
	}
}
