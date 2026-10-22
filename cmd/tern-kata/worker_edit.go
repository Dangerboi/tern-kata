package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"context"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// problemPrefix starts each line of the refusal that `worker edit` puts at the top of a reopened file; the lines are
// stripped before the file is checked again.
const problemPrefix = "#! "

// editWorkerConfig is `worker edit`: worker.toml goes through $VISUAL or $EDITOR in a temporary file, and the result
// replaces it only when the shared strict check accepts it. A refusal reopens the text with the problem at the top;
// an unchanged or empty result leaves the file as it was.
func editWorkerConfig(ctx context.Context, env workerEnv, out io.Writer) error {
	path := env.Dirs.ConfigPath()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return subcommandError{fmt.Errorf("no worker config at %s; run `tern-kata worker init` first", path)}
	}
	if err != nil {
		return subcommandError{err}
	}
	editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		return subcommandError{errors.New("set $VISUAL or $EDITOR to edit worker.toml")}
	}
	tmp, err := os.CreateTemp("", "tern-kata-worker-*.toml")
	if err != nil {
		return subcommandError{err}
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	original := string(raw)
	offered, problem := original, ""
	for {
		body := offered
		if problem != "" {
			body = commentProblem(problem) + offered
		}
		if err := os.WriteFile(tmp.Name(), []byte(body), 0o600); err != nil {
			return subcommandError{err}
		}
		process := editorCommand(editor, tmp.Name())
		process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := process.Run(); err != nil {
			return subcommandError{fmt.Errorf("editor: %w; worker.toml is unchanged", err)}
		}
		edited, err := os.ReadFile(tmp.Name())
		if err != nil {
			return subcommandError{err}
		}
		text := string(edited)
		if problem != "" {
			text = stripProblem(text)
		}
		if strings.TrimSpace(text) == "" || text == offered {
			_, err := fmt.Fprintf(out, "No change: %s is as it was.\n", path)
			return err
		}
		if err := worker.CheckConfig(text); err != nil {
			offered, problem = text, err.Error()
			continue
		}
		if err := worker.WriteConfig(path, text); err != nil {
			return subcommandError{err}
		}
		return reportEdit(ctx, env, path, out)
	}
}

func commentProblem(problem string) string {
	var b strings.Builder
	b.WriteString(problemPrefix + "worker.toml was not saved: it would not load.\n")
	for _, line := range strings.Split(problem, "\n") {
		b.WriteString(problemPrefix + line + "\n")
	}
	b.WriteString(problemPrefix + "Fix it and save; save it unchanged or empty to cancel.\n")
	return b.String()
}

func stripProblem(text string) string {
	for strings.HasPrefix(text, problemPrefix) {
		_, rest, _ := strings.Cut(text, "\n")
		text = rest
	}
	return text
}

// reportEdit says what the write means for a worker that may be running.
func reportEdit(ctx context.Context, env workerEnv, path string, out io.Writer) error {
	if _, err := worker.Ask(ctx, env.Dirs, worker.Request{Op: "status"}); errors.Is(err, worker.ErrNoWorker) {
		_, err = fmt.Fprintf(out, "Wrote %s. No worker is running; the next one reads it.\n", path)
		return err
	}
	_, err := fmt.Fprintf(out, "Wrote %s. A running worker reads it only at start: `tern-kata worker stop` makes it reload (the plugin starts a fresh one).\n", path)
	return err
}
