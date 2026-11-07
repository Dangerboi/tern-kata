package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// envWithout returns environ without the entries named in names, in one pass, with room for the two entries the callers add;
// names compare case-insensitively when caseInsensitive (Windows).
func envWithout(environ []string, caseInsensitive bool, names ...string) []string {
	env := make([]string, 0, len(environ)+2)
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.ContainsFunc(names, func(dropped string) bool { return envNamed(name, dropped, caseInsensitive) }) {
			env = append(env, entry)
		}
	}
	return env
}

// envNamed says an environment entry called name is the variable called want; names compare case-insensitively when
// caseInsensitive (Windows).
func envNamed(name, want string, caseInsensitive bool) bool {
	return name == want || caseInsensitive && strings.EqualFold(name, want)
}

// childEnv returns environ for a child with KATA_AUTHOR set to actor, dropping every existing spelling first;
// names compare case-insensitively on Windows. An empty actor leaves environ unchanged.
func childEnv(environ []string, actor string, caseInsensitive bool) []string {
	if actor == "" {
		return environ
	}
	return append(envWithout(environ, caseInsensitive, "KATA_AUTHOR"), "KATA_AUTHOR="+actor)
}

// routedEnv returns environ with the agent's kata daemon set from route: the names route.Env drops are removed (names compare
// case-insensitively on Windows), then its entries are added. A zero route returns environ unchanged. A refused route is a
// subcommandError (exit 1): the command line was understood and the agent never started, which is not the Pane's start failure.
func routedEnv(environ []string, route data.AgentRoute, caseInsensitive bool) ([]string, error) {
	set, unset, err := route.Env("")
	if err != nil {
		return nil, subcommandError{fmt.Errorf("agent: %w", err)}
	}
	for _, kv := range set {
		name, _, _ := strings.Cut(kv, "=")
		unset = append(unset, name)
	}
	if len(unset) == 0 {
		return environ, nil
	}
	return append(envWithout(environ, caseInsensitive, unset...), set...), nil
}

// agentEnv returns environ for the agent plan runs: KATA_AUTHOR set to plan.Actor as childEnv does (left as inherited when it is
// empty), no inherited KATA_REF, KATA_INBOX_USER or KATA_TEAMMATE (the person's own inbox and teammate are not the agent's), and
// KATA_REF set to plan.AgentRef when it is not empty. An Agent address prefix (the agent loads tern-kata's omp extension) sets
// KATA_OMP_LIVE_INBOX=0, so kata-omp leaves live delivery to the extension, and in a block KATA_INBOX_USER to the agent's Agent
// address, from the block id in TERN_PANE, and KATA_TEAMMATE to <name>-<block id>, so its comments and requests carry its own
// teammate name; a name kata would refuse leaves KATA_TEAMMATE unset. plan.AgentForwardComments sets
// TERN_KATA_FORWARD_COMMENTS=1, which the extension reads, only when KATA_TEAMMATE is set (without it the agent's own comments
// would be forwarded back to it); an inherited value is dropped.
func agentEnv(environ []string, plan LaunchPlan, caseInsensitive bool) []string {
	dropped := []string{"KATA_REF", "KATA_INBOX_USER", "KATA_TEAMMATE"}
	if plan.Actor != "" {
		dropped = append(dropped, "KATA_AUTHOR")
	}
	block := ""
	if plan.AgentAddressPrefix != "" {
		dropped = append(dropped, "KATA_OMP_LIVE_INBOX", "TERN_KATA_FORWARD_COMMENTS")
		block = strings.TrimSpace(envValue(environ, "TERN_PANE", caseInsensitive))
	}
	env := envWithout(environ, caseInsensitive, dropped...)
	if plan.Actor != "" {
		env = append(env, "KATA_AUTHOR="+plan.Actor)
	}
	if plan.AgentRef != "" {
		env = append(env, "KATA_REF="+plan.AgentRef)
	}
	if plan.AgentAddressPrefix != "" {
		env = append(env, "KATA_OMP_LIVE_INBOX=0")
		if block != "" {
			env = append(env, "KATA_INBOX_USER="+data.AgentAddress(plan.AgentAddressPrefix, block))
			name := plan.AgentAddressPrefix[strings.LastIndex(plan.AgentAddressPrefix, "/")+1:]
			if teammate := name + "-" + block; model.ValidTeammate(teammate) {
				env = append(env, "KATA_TEAMMATE="+teammate)
				if plan.AgentForwardComments {
					env = append(env, "TERN_KATA_FORWARD_COMMENTS=1")
				}
			}
		}
	}
	return env
}

// envValue is the value of the last entry named name in environ, "" when there is none; names compare case-insensitively when
// caseInsensitive (Windows).
func envValue(environ []string, name string, caseInsensitive bool) string {
	value := ""
	for _, entry := range environ {
		if key, v, ok := strings.Cut(entry, "="); ok && envNamed(key, name, caseInsensitive) {
			value = v
		}
	}
	return value
}

func editorOptions() tern.Options {
	return tern.Options{App: "tern-kata", Version: buildVersion(), Features: []string{tern.FeatureEdit, tern.FeatureUndo}}
}

// editorSurfaceOptions are the Pane's surface options: Tern's sans face, in which machine strings take the mono span token.
// closable lets Tern close the Pane without asking; it is set only when kata's [tui].confirm_quit is false, so a Pane that
// asks before q quits is not lost to a close without warning.
func editorSurfaceOptions(closable bool) tern.SurfaceOptions {
	return tern.SurfaceOptions{ID: "kata", Mode: tern.Screen, Title: "tern-kata", Font: tern.FontSans, Discard: true, Closable: closable}
}

// openSurface opens the window's surface and installs the plugin's style sheet on it.
func openSurface(session *tern.Session, closable bool) (*tern.Surface, error) {
	surface, err := session.Open(editorSurfaceOptions(closable))
	if err != nil {
		return nil, err
	}
	if err := surface.Stylesheet("tern-kata", view.CSS); err != nil {
		surface.Close()
		return nil, err
	}
	return surface, nil
}

// resolveEditor is the editor command: $VISUAL, else $EDITOR, else vi as kata's own TUI falls back to. With none set and
// no vi on PATH it refuses, naming the variables to set.
func resolveEditor() (string, error) {
	if editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR")); editor != "" {
		return editor, nil
	}
	if _, err := exec.LookPath("vi"); err != nil {
		return "", errors.New("no editor to open: set $VISUAL or $EDITOR")
	}
	return "vi", nil
}

// editInTerminal gives the terminal to the editor for the Draft and resumes the Pane. With no editor to run the Pane stays
// as it is and the refusal comes back as the result. When resuming fails after the editor succeeded, the edited text
// stays in its file, whose path is printed and named in the error.
func editInTerminal(t *terminal, draft model.OpenDraftEditor, build func(editor, file string) *exec.Cmd) (model.DraftEditorClosed, error) {
	editor, err := resolveEditor()
	if err != nil {
		return model.DraftEditorClosed{UID: draft.UID, Text: draft.Text, Err: err}, nil //nolint:nilerr // the refusal is the edit's result for the card to show; the Pane carries on
	}
	var result model.DraftEditorClosed
	var kept string
	err = t.Suspend(func() { result, kept = editDraft(draft, editor, build) })
	if err != nil && kept != "" {
		fmt.Fprintf(os.Stderr, "tern-kata: could not reconnect to Tern; your edited Draft is kept in %s\n", kept)
		return result, fmt.Errorf("%w; your edited Draft is kept in %s", err, kept)
	}
	if kept != "" {
		os.Remove(kept)
	}
	return result, err
}

// editDraft round-trips the Draft text through a temp file edited by the command build returns for editor. It returns
// the file's path only when the editor succeeded; the caller removes it once the text is safe.
func editDraft(cmd model.OpenDraftEditor, editor string, build func(editor, file string) *exec.Cmd) (model.DraftEditorClosed, string) {
	result := model.DraftEditorClosed{UID: cmd.UID, Text: cmd.Text}
	suffix := cmp.Or(cmd.Suffix, ".md")
	file, err := os.CreateTemp("", "tern-kata-Draft-*"+suffix)
	if err != nil {
		result.Err = err
		return result, ""
	}
	_, err = file.WriteString(cmd.Text)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(file.Name())
		result.Err = err
		return result, ""
	}
	process := build(editor, file.Name())
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		os.Remove(file.Name())
		result.Err = fmt.Errorf("editor %s: %w; original text kept", editor, err)
		return result, ""
	}
	text, err := os.ReadFile(file.Name())
	if err != nil {
		os.Remove(file.Name())
		result.Err = err
		return result, ""
	}
	result.Text = string(text)
	return result, file.Name()
}
