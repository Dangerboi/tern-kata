package model

import (
	"fmt"
	"regexp"
	"strings"
)

// A spoke's name is a label the hub keeps for an enrolled instance, so hub event history and its enrollment list can say which spoke
// is meant. It is set while enrolling (`N` on the preview) or afterwards (`N` on a spoke, in the list or its detail), where an empty
// name clears it. The card checks kata's rule for a name itself before sending it; the hub refuses only a name another spoke holds.

// SetFederationSpokeName sets, or with an empty Name clears, the name the hub at HubURL gives the instance InstanceUID. Attempt
// numbers the runs: an answer to an older one is dropped.
type SetFederationSpokeName struct {
	Workspace     string
	Attempt       int64
	HubURL        string
	AllowInsecure bool
	InstanceUID   string
	Name          string
}

func (SetFederationSpokeName) cmd() {}

// FederationSpokeNamed answers SetFederationSpokeName: the name now held for the hub at Hub (empty once cleared), or why the hub
// refused. Err is kata's own wording.
type FederationSpokeNamed struct {
	Attempt int64
	Hub     string
	Name    string
	Err     error
}

func (FederationSpokeNamed) msg() {}

// FederationName is the card asking for a spoke's name. Rename says it names a spoke already enrolled, which runs on the hub when
// confirmed; otherwise it names the enrollment being previewed. HubURL and AllowInsecure are the spoke's hub, kept for the run.
type FederationName struct {
	Open          bool
	Rename        bool
	HubURL        string
	AllowInsecure bool
	Input         string
	Caret         Caret
	Saving        bool
	Err           error
}

// SpokeLabel is the spoke's name as the list and detail show it: the name, "-" when the hub gives none.
func (s FederationSpoke) SpokeLabel() string { return federationDash(oneLine(s.SpokeName)) }

// openFederationNameForSpoke is N on a spoke, in the list or its detail.
func openFederationNameForSpoke(m Model) (Model, []Cmd) {
	spoke := m.SelectedSpoke()
	if spoke == nil || (m.Federation.Screen != FederationList && m.Federation.Screen != FederationDetail) {
		return m, nil
	}
	m.Federation.Name = FederationName{Open: true, Rename: true, HubURL: spoke.HubURL, AllowInsecure: spoke.AllowInsecure, Input: spoke.SpokeName}
	return m, nil
}

// openFederationNameForEnroll is N on the enrollment preview.
func openFederationNameForEnroll(m Model) (Model, []Cmd) {
	f := &m.Federation
	if f.Enrolling {
		return m, nil
	}
	f.Name = FederationName{Open: true, Input: f.Draft.SpokeName}
	return m, nil
}

// spokeNamePattern is kata's rule for a spoke name (kata's own check runs before it writes, and so does ours).
var spokeNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ValidateSpokeName reports whether the hub will take name for a spoke, in kata's words. An empty name is fine: it names nothing, or
// clears the name.
func ValidateSpokeName(name string) error {
	if name == "" || spokeNamePattern.MatchString(name) {
		return nil
	}
	return fmt.Errorf("invalid federation spoke name: %q must match [a-z][a-z0-9-]{0,31}", name)
}

// federationNameKey is one key for the name card: Enter saves, esc closes, every other key is typed. Nothing but typing and moving
// the caret is taken while a rename runs.
func federationNameKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.Federation
	n := &f.Name
	if n.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeFederationName)
	switch {
	case bound && binding.Action == FederationNameSave:
		name := strings.TrimSpace(n.Input)
		if err := ValidateSpokeName(name); err != nil {
			n.Err = err
			return m, nil
		}
		if !n.Rename {
			f.Draft.SpokeName = name
			*n = FederationName{}
			return m, nil
		}
		n.Saving, n.Err = true, nil
		return m, []Cmd{SetFederationSpokeName{Workspace: m.Workspace, Attempt: f.attempts.Ask(), HubURL: n.HubURL, AllowInsecure: n.AllowInsecure,
			InstanceUID: strings.TrimSpace(f.InstanceUID), Name: name}}
	case bound && binding.Action == FederationNameCancel:
		*n = FederationName{}
	default:
		var edited bool
		if n.Input, edited = editText(n.Input, &n.Caret, "", bound, msg); edited {
			n.Err = nil
		}
	}
	return m, nil
}

// federationSpokeNamed lands a rename's answer: the card closes and every spoke of that hub shows the name, since the hub keeps one
// per instance; a refusal keeps the card with kata's reason under it. A status read still out was asked before the rename took
// effect, so its answer would bring the old name back: it is dropped and read again.
func federationSpokeNamed(m Model, msg FederationSpokeNamed) (Model, []Cmd) {
	f := &m.Federation
	if !f.attempts.IsCurrent(msg.Attempt) || !f.Name.Saving {
		return m, nil
	}
	if msg.Err != nil {
		f.Name.Saving, f.Name.Err = false, msg.Err
		return m, nil
	}
	for i := range f.Spokes {
		if f.Spokes[i].HubURL == msg.Hub {
			f.Spokes[i].SpokeName = msg.Name
		}
	}
	f.Name = FederationName{}
	if f.Loading {
		return askFederation(m)
	}
	return m, nil
}
