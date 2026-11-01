package worker_test

import (
	"fmt"
	"strings"
	"sync"
)

// logLines collects the Worker's log.
type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (l *logLines) count(sub string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.lines {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}
