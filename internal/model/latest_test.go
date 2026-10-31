package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestLatestAppliesOnlyTheNewestReadsAnswer(t *testing.T) {
	var l model.Latest
	first, second := l.Ask(), l.Ask()
	if current, again := l.Answer(first); current || again {
		t.Errorf("the superseded read's answer: current %v again %v, want neither", current, again)
	}
	if current, again := l.Answer(second); !current || again {
		t.Errorf("the newest read's answer: current %v again %v, want current only", current, again)
	}
	if current, _ := l.Answer(second + 1); current {
		t.Error("an answer to a read never asked for was current")
	}
	var fresh model.Latest
	if current, _ := fresh.Answer(0); current {
		t.Error("an answer before any read was current")
	}
}

func TestLatestQueuesOneReadWhileOneIsOut(t *testing.T) {
	var l model.Latest
	n, ok := l.Queue()
	if !ok {
		t.Fatal("the first read was queued")
	}
	if _, ok := l.Queue(); ok {
		t.Error("a second read was numbered while one was out")
	}
	if _, ok := l.Queue(); ok {
		t.Error("a third read was numbered while one was out")
	}
	current, again := l.Answer(n)
	if !current || !again {
		t.Fatalf("the answer: current %v again %v, want both", current, again)
	}
	next, ok := l.Queue()
	if !ok || next == n {
		t.Fatalf("after the answer Queue gave %d, %v, want a new number", next, ok)
	}
	if current, again := l.Answer(next); !current || again {
		t.Errorf("the queued read's answer: current %v again %v, want current only", current, again)
	}
}

func TestLatestSupersedeDropsTheReadOutAndAsksForAnother(t *testing.T) {
	var l model.Latest
	n := l.Ask()
	l.Supersede()
	current, again := l.Answer(n)
	if current || !again {
		t.Fatalf("the answer sent before: current %v again %v, want dropped with a re-ask", current, again)
	}
	if _, again := l.Answer(n); again {
		t.Error("a second stale answer asked again")
	}
	answered := l.Ask()
	l.Answer(answered)
	l.Supersede()
	if _, again := l.Answer(n); again {
		t.Error("superseding with no read out asked for one")
	}
}
