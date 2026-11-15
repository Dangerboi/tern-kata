package data

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ternSubcommandsVersion is the first Tern known to have `tern download` and `tern inspect`. A Tern without a subcommand reads
// its first word as a file to open and hands it to the running window, so these are asked only of a Tern at least this new.
var ternSubcommandsVersion = [3]int{0, 9, 21}

// ternVersionCache holds the answer of `tern --version` once Tern has given one. A failed ask is not kept: the next call asks again.
type ternVersionCache struct {
	mu       sync.Mutex
	answered bool
	known    bool
}

// ternVersionTimeout bounds the version ask, which runs detached from the caller's context so a nearly expired one cannot fail it.
const ternVersionTimeout = 5 * time.Second

// ternHasSubcommands reports whether this Tern is known to have `tern download` and `tern inspect`. A version that cannot be
// asked is an error, not kept: the next call asks again. One that comes back unreadable or old is (false, nil), and that answer
// is kept.
func (g *Gateway) ternHasSubcommands(ctx context.Context) (bool, error) {
	c := &g.ternVersion
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.answered {
		return c.known, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ternVersionTimeout)
	defer cancel()
	out, err := g.runTern(ctx, []string{"--version"})
	if err != nil {
		return false, fmt.Errorf("tern --version: %w", err)
	}
	v, ok := parseTernVersion(string(out))
	c.answered, c.known = true, ok && !versionBefore(v, ternSubcommandsVersion)
	return c.known, nil
}

// ternKnownToHaveSubcommands is ternHasSubcommands for a caller that treats a failed ask as not known.
func (g *Gateway) ternKnownToHaveSubcommands(ctx context.Context) bool {
	has, err := g.ternHasSubcommands(ctx)
	return has && err == nil
}

// parseTernVersion reads the numbers of `tern X.Y.Z (commit)`.
func parseTernVersion(text string) ([3]int, bool) {
	var v [3]int
	fields := strings.Fields(text)
	if len(fields) < 2 || fields[0] != "tern" {
		return v, false
	}
	parts := strings.Split(fields[1], ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func versionBefore(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
