package worker

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// The Supervision rule has one owner: the host half asks the worker, so these cases are the rule.
func TestSupervisedConfigIsOnlyAConfigWithoutTheMarker(t *testing.T) {
	cases := []struct {
		name         string
		config, mark bool
		wantOn       bool
	}{
		{"no config", false, false, false},
		{"config", true, false, true},
		{"config and marker", true, true, false},
		{"marker only", false, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Dirs{Config: filepath.Join(t.TempDir(), "tern-kata")}
			if err := os.MkdirAll(d.Config, 0o700); err != nil {
				t.Fatal(err)
			}
			if c.config {
				if err := os.WriteFile(d.ConfigPath(), []byte("executor = \"x\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if c.mark {
				if err := d.SetSupervision(false); err != nil {
					t.Fatal(err)
				}
			}
			text, on := d.SupervisedConfig()
			if on != c.wantOn || d.SupervisionOn() != c.wantOn {
				t.Fatalf("on = %v, SupervisionOn = %v, want %v", on, d.SupervisionOn(), c.wantOn)
			}
			if want := map[bool]string{true: "executor = \"x\"\n", false: ""}[c.wantOn && c.config]; text != want {
				t.Fatalf("text %q, want %q", text, want)
			}
		})
	}
}

// The host half starts a worker only when none answers the control socket. A worker from a release before the identity
// request still answers, refusing the request: it runs, and worker.lock names its process.
func TestRunningWorkerIsWhoeverAnswersTheControlSocket(t *testing.T) {
	root, err := os.MkdirTemp("", "tk") //nolint:usetesting // t.TempDir's path is too long for a unix socket
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	d := Dirs{Config: root, State: root}
	if _, running := RunningWorker(t.Context(), d); running {
		t.Fatal("no socket: a worker runs")
	}
	if err := os.WriteFile(d.Socket(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, running := RunningWorker(t.Context(), d); running {
		t.Fatal("a socket file nobody listens on: a worker runs")
	}
	if err := os.Remove(d.Socket()); err != nil {
		t.Fatal(err)
	}
	ln, err := new(net.ListenConfig).Listen(t.Context(), "unix", d.Socket())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req Request
			_ = json.NewDecoder(conn).Decode(&req)
			_ = json.NewEncoder(conn).Encode(Response{Error: "unknown request " + req.Op})
			conn.Close()
		}
	}()
	if err := os.WriteFile(d.Lock(), []byte("4242"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, running := RunningWorker(t.Context(), d)
	if !running || id != (Identity{PID: 4242}) {
		t.Fatalf("an older worker: %+v, running %v; want process 4242 with no version", id, running)
	}
}
