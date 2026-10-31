package model

// Latest tells a Page which answer to a read is the one to show. Each read it asks for gets a number, carried by the command and
// echoed by its answer; only the answer to the newest number is current. The first read is number 0, so the zero value of a Page's
// Latest needs no setup.
//
// A Page that wants at most one read out uses Queue: a request made while a read is out costs one more read after its answer.
// A Page whose reads are independent uses Ask, which supersedes whatever is out. A change that makes every read out older than
// its own effect uses Supersede, which drops those answers and asks for one more read.
type Latest struct {
	// next is the number the next read gets; the newest read asked for is next-1.
	next int64
	// out says the newest read has not answered.
	out bool
	// queued says a read is wanted once the one out answers.
	queued bool
}

// Ask numbers a new read, which supersedes any read out.
func (l *Latest) Ask() int64 {
	n := l.next
	l.next++
	l.out, l.queued = true, false
	return n
}

// Queue numbers a new read when none is out. When one is out it asks for none and remembers that one is wanted, which Answer
// reports when the read out answers.
func (l *Latest) Queue() (n int64, ok bool) {
	if l.out {
		l.queued = true
		return 0, false
	}
	return l.Ask(), true
}

// Supersede makes every read asked for so far stale. When one is out its answer is dropped and asks for a fresh read.
func (l *Latest) Supersede() {
	l.next++
	l.queued = l.queued || l.out
	l.out = false
}

// Discard makes every read asked for so far stale and wants none: a Page that starts over drops what is out without asking again.
func (l *Latest) Discard() {
	l.next++
	l.out, l.queued = false, false
}

// Asked says some read has been asked for.
func (l Latest) Asked() bool { return l.next > 0 }

// Answer says what an answer to read n calls for: current says it is the newest read's answer and is to be applied; again says
// a fresh read is wanted now, which the Page asks for with Ask or Queue.
func (l *Latest) Answer(n int64) (current, again bool) {
	if l.next > 0 && n == l.next-1 {
		again = l.queued
		l.out, l.queued = false, false
		return true, again
	}
	if l.queued && !l.out {
		l.queued = false
		return false, true
	}
	return false, false
}

// Newest is the number of the newest read asked for; 0 before any.
func (l Latest) Newest() int64 { return max(l.next-1, 0) }
