package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"go.kenn.io/kit/consolewindow"
	"golang.org/x/mod/semver"
)

// ternTimeout bounds a tern command that has no timeout of its own.
const ternTimeout = 30 * time.Second

// Every Tern release that a feature of tern-kata waits for. A version is compared by semver ordering, so a pre-release of a floor
// (0.9.21-dev) is older than the floor.
const (
	// ternSubcommandsFloor is the first Tern known to have `tern download` and `tern inspect`. A Tern without a subcommand reads
	// its first word as a file to open and hands it to the running window, so these are asked only of a Tern at least this new.
	ternSubcommandsFloor = "v0.9.21"
)

// RunTern runs the tern on the PATH and returns its stdout (ternCommand with no Gateway).
func RunTern(ctx context.Context, timeout time.Duration, args []string) ([]byte, error) {
	return ternCommand(ctx, execTern, timeout, args)
}

func (g *Gateway) ternOutput(ctx context.Context, timeout time.Duration, args []string) ([]byte, error) {
	return ternCommand(ctx, g.runTern, timeout, args)
}

// ternCommand runs one tern command with run and returns its stdout. A failure carries what the command printed, stderr first:
// Tern's own words say why. timeout bounds the command when it is above zero.
func ternCommand(ctx context.Context, run func(context.Context, []string) ([]byte, error), timeout time.Duration, args []string) ([]byte, error) {
	caller := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	out, err := run(ctx, args)
	if err == nil {
		return out, nil
	}
	if timeout > 0 && caller.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("the tern command did not answer within %s: %w", timeout, context.DeadlineExceeded)
	}
	text := string(bytes.TrimSpace(out))
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		if stderr := string(bytes.TrimSpace(exit.Stderr)); stderr != "" {
			text = stderr
		}
	}
	if text != "" {
		err = fmt.Errorf("%w: %s", err, text)
	}
	return out, err
}

// execTern runs the tern on the PATH with the Pane's environment, which carries the window it answers to. A failed exit leaves what
// tern wrote to stderr in the *exec.ExitError.
func execTern(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tern", args...)
	consolewindow.Hide(cmd)
	return cmd.Output()
}

// ternVersionCache holds the version `tern --version` gave once Tern has given a readable one. A failed ask is not kept: the next
// call asks again. Callers that ask while one runs wait for it, each only until its own ctx ends.
type ternVersionCache struct {
	mu      sync.Mutex
	version string
	flight  *ternVersionFlight
}

type ternVersionFlight struct {
	done    chan struct{}
	version string
	err     error
}

// ternVersionTimeout bounds the version ask, which runs detached from the caller's context so one that ends early does not fail the
// ask of the callers that wait with it.
const ternVersionTimeout = 5 * time.Second

// ternAtLeast reports whether this Tern is at least floor, a semver with a leading v. A version that cannot be asked or read is an
// error, never "older": a caller that would act on an old Tern (prune, skip a subcommand) must not take a failed ask for one.
func (g *Gateway) ternAtLeast(ctx context.Context, floor string) (bool, error) {
	version, err := g.ternVersionOf(ctx)
	if err != nil {
		return false, err
	}
	return semver.Compare(version, floor) >= 0, nil
}

func (g *Gateway) ternVersionOf(ctx context.Context) (string, error) {
	c := &g.ternVersion
	c.mu.Lock()
	if c.version != "" {
		defer c.mu.Unlock()
		return c.version, nil
	}
	f := c.flight
	if f == nil {
		f = &ternVersionFlight{done: make(chan struct{})}
		c.flight = f
		go g.askTernVersion(context.WithoutCancel(ctx), f)
	}
	c.mu.Unlock()
	select {
	case <-f.done:
		return f.version, f.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (g *Gateway) askTernVersion(ctx context.Context, f *ternVersionFlight) {
	out, err := g.ternOutput(ctx, ternVersionTimeout, []string{"--version"})
	if err != nil {
		f.err = fmt.Errorf("tern --version: %w", err)
	} else {
		f.version, f.err = parseTernVersion(string(out))
	}
	c := &g.ternVersion
	c.mu.Lock()
	if f.err == nil {
		c.version = f.version
	}
	c.flight = nil
	c.mu.Unlock()
	close(f.done)
}

// parseTernVersion reads the semver of `tern X.Y.Z (commit)`: a leading v, and a pre-release or dev suffix, are accepted.
func parseTernVersion(text string) (string, error) {
	fields := strings.Fields(text)
	if len(fields) < 2 || fields[0] != "tern" {
		return "", fmt.Errorf("tern --version printed %q, not `tern <version>`", strings.TrimSpace(text))
	}
	version := "v" + strings.TrimPrefix(fields[1], "v")
	if !semver.IsValid(version) {
		return "", fmt.Errorf("tern --version names %q, which is no version", fields[1])
	}
	return version, nil
}

type ternSession struct {
	name   string
	blocks []string
}

// ternListing is what one window's `tern ls --json` shows: its sessions' blocks, and the ids of its parked panes, which no
// session holds but whose programs run on.
type ternListing struct {
	sessions []ternSession
	parked   []string
}

func (l ternListing) holds(block string) bool {
	return slices.Contains(l.parked, block) || slices.ContainsFunc(l.sessions, func(s ternSession) bool { return slices.Contains(s.blocks, block) })
}

// ternWindowLs reads `tern ls --json` for the window with key window; "" is the window the command runs in. Tern shows one window's
// sessions to each ask, and a block moved to another window is not in this window's. Block ids are kept as written, for they carry
// a host tag and can pass 2^53.
func (g *Gateway) ternWindowLs(ctx context.Context, window string) (ternListing, error) {
	args := []string{"ls", "--json"}
	if window != "" {
		args = []string{"ls", "--window", window, "--json"}
	}
	out, err := g.ternOutput(ctx, ternTimeout, args)
	if err != nil {
		return ternListing{}, err
	}
	var ls struct {
		Sessions []struct {
			Name string `json:"name"`
			Tabs []struct {
				Blocks []struct {
					ID json.Number `json:"id"`
				} `json:"blocks"`
			} `json:"tabs"`
		} `json:"sessions"`
		Detached []struct {
			ID json.Number `json:"id"`
		} `json:"detached"`
	}
	if err := json.Unmarshal(out, &ls); err != nil {
		return ternListing{}, fmt.Errorf("tern ls --json: %w", err)
	}
	listing := ternListing{sessions: make([]ternSession, 0, len(ls.Sessions))}
	for _, s := range ls.Sessions {
		session := ternSession{name: s.Name}
		for _, t := range s.Tabs {
			for _, b := range t.Blocks {
				session.blocks = append(session.blocks, b.ID.String())
			}
		}
		listing.sessions = append(listing.sessions, session)
	}
	for _, d := range ls.Detached {
		listing.parked = append(listing.parked, d.ID.String())
	}
	return listing, nil
}

// ternWindowKeys is the key of every window Tern shows, from `tern inspect --json`: a client of kind "window", or "remote" (a
// window on another machine attached to this host, with a key of its own). Command-line clients are no window, and the Pane's own
// window, which has no key to name, is no entry. A key that `tern ls --window` refuses fails the whole answer (windowListings), so
// nothing is pruned on a guess; so does a version that cannot be read, and so does a failed or unreadable `tern inspect` on a Tern
// known to have it, for then it is unknown whether other windows exist. A Tern older than ternSubcommandsFloor is not asked (it
// would open "inspect" as a file); it and one whose inspect parses but lists no clients give nil, and nothing is known of other
// windows.
func (g *Gateway) ternWindowKeys(ctx context.Context) ([]string, error) {
	has, err := g.ternAtLeast(ctx, ternSubcommandsFloor)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	out, err := g.ternOutput(ctx, ternTimeout, []string{"inspect", "--json"})
	if err != nil {
		return nil, fmt.Errorf("tern inspect: %w", err)
	}
	var inspected struct {
		Clients []struct {
			Window string `json:"window"`
			Kind   string `json:"kind"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(out, &inspected); err != nil {
		return nil, fmt.Errorf("tern inspect: %w", err)
	}
	var keys []string
	for _, c := range inspected.Clients {
		if (c.Kind == "window" || c.Kind == "remote") && c.Window != "" && !slices.Contains(keys, c.Window) {
			keys = append(keys, c.Window)
		}
	}
	return keys, nil
}

// windowListing is one window's ls. own marks the one Tern gives a command that names no window: the Pane's.
type windowListing struct {
	window string
	own    bool
	ternListing
}

// windowListings lists the Pane's own window first, then every other open window by key. A window that cannot be listed fails the
// whole answer: a block missing from it would look closed.
func (g *Gateway) windowListings(ctx context.Context) ([]windowListing, error) {
	own, err := g.ternWindowLs(ctx, "")
	if err != nil {
		return nil, err
	}
	all := []windowListing{{own: true, ternListing: own}}
	keys, err := g.ternWindowKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not tell which Tern windows exist: %w", err)
	}
	for _, key := range keys {
		listing, err := g.ternWindowLs(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("window %q: %w", key, err)
		}
		all = append(all, windowListing{window: key, ternListing: listing})
	}
	return all, nil
}

// otherWindowOf is the key of the window other than the Pane's that holds block, and false when the Pane's own window holds it,
// no window does, or the windows cannot be listed. The Pane's own window comes first in the listings, so a block it holds is never
// placed in another window.
func (g *Gateway) otherWindowOf(ctx context.Context, block string) (string, bool) {
	listings, err := g.windowListings(ctx)
	if err != nil {
		return "", false
	}
	i := slices.IndexFunc(listings, func(l windowListing) bool { return l.holds(block) })
	if i < 0 || listings[i].own {
		return "", false
	}
	return listings[i].window, true
}

// ternOnBlock runs a command that names block. Tern looks for the block in the Pane's window only, so when that fails and
// another window holds the block, the command runs again naming that window. When no other window holds it the first error stands.
func (g *Gateway) ternOnBlock(ctx context.Context, block string, args []string) ([]byte, error) {
	out, err := g.ternOutput(ctx, ternTimeout, args)
	if err == nil {
		return out, nil
	}
	if window, ok := g.otherWindowOf(ctx, block); ok {
		return g.ternOutput(ctx, ternTimeout, append(slices.Clone(args), "--window", window))
	}
	return out, err
}

// inOtherWindow tells whether block is held by a window other than the Pane's, which no placing command can reach.
func (g *Gateway) inOtherWindow(ctx context.Context, block string) bool {
	_, ok := g.otherWindowOf(ctx, block)
	return ok
}

// ParseTernBlock is the block id in what `tern split|new tab --json` printed: {"session": 1, "tab": 2, "block": 3}. Ids carry a
// host tag and can pass 2^53, so the digits are kept as written.
func ParseTernBlock(out []byte) (string, error) {
	start := bytes.IndexByte(out, '{')
	if start < 0 {
		return "", fmt.Errorf("tern printed no JSON: %q", strings.TrimSpace(string(out)))
	}
	var found struct {
		Block json.Number `json:"block"`
	}
	if err := json.NewDecoder(bytes.NewReader(out[start:])).Decode(&found); err != nil {
		return "", fmt.Errorf("tern's JSON: %w", err)
	}
	if found.Block == "" {
		return "", errors.New("tern's JSON names no block")
	}
	return found.Block.String(), nil
}

const liveBlocksMaxAge = 10 * time.Second

const liveBlocksTimeout = 30 * time.Second

// liveBlocksCache guards live, the blocks the last `tern ls --json` listed, at, when it was asked, and flight, the ask now running.
type liveBlocksCache struct {
	mu     sync.Mutex
	live   map[string]bool
	at     time.Time
	flight *lsFlight
}

type lsFlight struct {
	done  chan struct{}
	begun time.Time
	live  map[string]bool
	err   error
}

// liveBlocks is the set of blocks `tern ls --json` lists in every open window (askLiveBlocks). It is cached for liveBlocksMaxAge, read by g.now; fresh asks tern
// again and replaces the cache. cached says the answer may be older than the call: it came from the cache, or from a `tern ls`
// another caller started before this one asked. Callers that need an answer while one `tern ls` runs share it, each waiting
// only until its own ctx ends; a fresh caller never joins a run that began before it asked.
func (g *Gateway) liveBlocks(ctx context.Context, fresh bool) (live map[string]bool, cached bool, err error) {
	c := &g.liveCache
	c.mu.Lock()
	if !fresh && c.live != nil && g.now().Sub(c.at) < liveBlocksMaxAge {
		live = c.live
		c.mu.Unlock()
		return live, true, nil
	}
	f := c.flight
	joined := f != nil && !fresh
	if !joined {
		f = &lsFlight{done: make(chan struct{}), begun: g.now()}
		c.flight = f
		go g.runLiveBlocks(context.WithoutCancel(ctx), f)
	}
	c.mu.Unlock()
	select {
	case <-f.done:
		return f.live, joined, f.err
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

// runLiveBlocks asks tern for the blocks, caches a good answer, stamped with when the run began, unless a later run's answer
// is cached already, and wakes the callers waiting on f.
func (g *Gateway) runLiveBlocks(ctx context.Context, f *lsFlight) {
	ctx, cancel := context.WithTimeout(ctx, liveBlocksTimeout)
	defer cancel()
	f.live, f.err = g.askLiveBlocks(ctx)
	c := &g.liveCache
	c.mu.Lock()
	if f.err == nil && !f.begun.Before(c.at) {
		c.live, c.at = f.live, f.begun
	}
	if c.flight == f {
		c.flight = nil
	}
	c.mu.Unlock()
	close(f.done)
}

// askLiveBlocks is every block Tern holds in any open window: in a session, or parked. A block that exited but still shows
// (kept open) counts, as it did in `tern ls`, for its entry and badge are the agent's record until the block closes.
func (g *Gateway) askLiveBlocks(ctx context.Context) (map[string]bool, error) {
	listings, err := g.windowListings(ctx)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, l := range listings {
		for _, b := range l.parked {
			live[b] = true
		}
		for _, s := range l.sessions {
			for _, b := range s.blocks {
				live[b] = true
			}
		}
	}
	return live, nil
}
