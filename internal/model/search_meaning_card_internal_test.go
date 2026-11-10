package model

import (
	"testing"
	"time"
)

func askedStatus(cmds []Cmd) (LoadEmbeddingsStatus, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(LoadEmbeddingsStatus); ok {
			return load, true
		}
	}
	return LoadEmbeddingsStatus{}, false
}

// openEmbeddingsCard is what a place outside the Config Page calls: with no status known it reads one first, and the card asks its
// question once the answer says kata would take a turn-on.
func TestOpenEmbeddingsCardWithNoStatusKnownReadsItFirst(t *testing.T) {
	m := Model{Workspace: "/repo", Daemon: Daemon{Name: "home"}}
	m, cmds := openEmbeddingsCard(m)
	read, ok := askedStatus(cmds)
	if !ok || read.After != 0 {
		t.Fatalf("opening the card sent %#v, want a read of the status at once", cmds)
	}
	if !m.EmbeddingsCard.Open || m.EmbeddingsCard.Asking || m.TopCard() != CardEmbeddings || m.CurrentMode() != ModeEmbeddings {
		t.Fatalf("before the answer: open %v, asking %v, mode %s", m.EmbeddingsCard.Open, m.EmbeddingsCard.Asking, m.CurrentMode())
	}
	m, cmds = Update(m, EmbeddingsStatusLoaded{Request: read.Request, Status: EmbeddingsStatus{Provider: EmbeddingsOff, DownloadBytes: 290_000_000, CanChange: true}})
	if len(cmds) != 0 || !m.EmbeddingsCard.Asking || m.CurrentMode() != ModeEmbeddingsAsk {
		t.Errorf("after the answer: cmds %#v, asking %v, mode %s, want the question", cmds, m.EmbeddingsCard.Asking, m.CurrentMode())
	}
}

func TestOpenEmbeddingsCardWithAKnownIdleStatusReadsNothingAndAsks(t *testing.T) {
	m := Model{Workspace: "/repo", Embeddings: EmbeddingsSetup{Read: true, Status: EmbeddingsStatus{Provider: EmbeddingsOff, OllamaAvailable: true, CanChange: true}}}
	m, cmds := openEmbeddingsCard(m)
	if len(cmds) != 0 || !m.EmbeddingsCard.Asking {
		t.Errorf("cmds %#v, asking %v, want no read and the question", cmds, m.EmbeddingsCard.Asking)
	}
}

// Off the Config Page the card alone keeps a busy status read; closing it stops that.
func TestTheCardPollsABusyStatusOffTheConfigPageAndStopsWhenClosed(t *testing.T) {
	busy := EmbeddingsStatus{Provider: EmbeddingsOff, CanChange: true, Job: &EmbeddingsJob{State: EmbeddingsDownloading, Downloaded: 1, Total: 2}}
	m := Model{Workspace: "/repo", Embeddings: EmbeddingsSetup{Read: true, Status: busy}}
	if m.OnConfig() {
		t.Fatal("fixture: on the Config Page")
	}
	m, cmds := openEmbeddingsCard(m)
	read, ok := askedStatus(cmds)
	if !ok || read.After != 0 || m.EmbeddingsCard.Asking {
		t.Fatalf("a card opened on a running job sent %#v, asking %v; want a read at once and no question", cmds, m.EmbeddingsCard.Asking)
	}
	m, cmds = Update(m, EmbeddingsStatusLoaded{Request: read.Request, Status: busy})
	next, ok := askedStatus(cmds)
	if !ok || next.After != 2*time.Second {
		t.Fatalf("the shown card sent %#v after a busy answer, want a read after 2s", cmds)
	}
	m, _ = Update(m, KeyMsg{Chord: "escape"})
	if m.EmbeddingsCard.Open {
		t.Fatal("esc did not close the card")
	}
	if _, cmds = Update(m, EmbeddingsStatusLoaded{Request: next.Request, Status: busy}); len(cmds) != 0 {
		t.Errorf("a closed card off the Config Page went on polling: %#v", cmds)
	}
}
