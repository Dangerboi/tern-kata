package data

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestOpenAttachmentShowsAWordDocumentInTernAndNeverHandsItToTheOS(t *testing.T) {
	f := newDocbankFixture(t)
	t.Setenv("TERN_PANE", "7")
	asked := ternScript(f.gateway, map[string]func() ([]byte, error){"open": ternOut("")})

	got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "plan.docx")))
	if want := [][]string{{"open", "--preview", got.Path}}; got.Err != nil || !slices.EqualFunc(*asked, want, slices.Equal) {
		t.Fatalf("opened %+v, tern was asked %v, want %v", got, *asked, want)
	}
	if len(*f.opened) != 0 || got.OutsideTern {
		t.Fatalf("the OS opener ran: %v (outside Tern %v)", *f.opened, got.OutsideTern)
	}
}

func TestOpenAttachmentOnlySavesAWordDocumentWhenTernCannotShowIt(t *testing.T) {
	for name, tc := range map[string]struct {
		env  func(*testing.T)
		runs func(context.Context, []string) ([]byte, error)
		note string
	}{
		"not in a Tern Pane": {func(*testing.T) {}, func(context.Context, []string) ([]byte, error) { t.Error("tern ran outside a Pane"); return nil, nil }, ""},
		"tern exits non-zero": {func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(_ context.Context, args []string) ([]byte, error) {
			if args[0] == "--version" {
				return []byte(modernTernVersion), nil
			}
			return []byte("no window answers"), errors.New("exit status 1")
		}, "no window answers"},
		"a Tern from before it draws them": {func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(_ context.Context, args []string) ([]byte, error) {
			if args[0] != "--version" {
				t.Errorf("tern ran %q on a Tern that does not draw Word documents", args)
			}
			return []byte("tern 0.9.20 (x)"), nil
		}, "Word documents need Tern 0.9.21 or newer"},
		"a version nobody can read": {func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(_ context.Context, args []string) ([]byte, error) {
			if args[0] != "--version" {
				t.Errorf("tern ran %q on a Tern of unknown version", args)
			}
			return []byte("tern next"), nil
		}, "could not tell whether this Tern draws Word documents"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankFixture(t)
			tc.env(t)
			f.gateway.runTern = tc.runs
			got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "plan.docx")))
			if got.Err == nil || got.Path == "" || !strings.Contains(got.Err.Error(), "saved at "+got.Path+" and not opened: tern-kata opens only images, video, audio, PDFs and text") {
				t.Fatalf("opened %+v", got)
			}
			if !strings.Contains(got.Err.Error(), tc.note) || !strings.Contains(got.Note, tc.note) {
				t.Fatalf("error %q and note %q, want Tern's reason %q in both", got.Err, got.Note, tc.note)
			}
			if _, err := os.Stat(got.Path); err != nil {
				t.Fatalf("the download was not kept: %v", err)
			}
			if len(*f.opened) != 0 || got.OutsideTern {
				t.Fatalf("the OS opener ran: %v (outside Tern %v)", *f.opened, got.OutsideTern)
			}
		})
	}
}

func TestOpenAttachmentWordDocumentWaysSystemRefusedBesideAllowed(t *testing.T) {
	f := newDocbankFixture(t)
	t.Setenv("TERN_PANE", "7")
	asked := ternScript(f.gateway, map[string]func() ([]byte, error){"open": ternOut("")})

	cmd := f.openCmd(t, "plan.docx")
	cmd.Way = model.WaySystem
	if got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd)); got.Err == nil || len(*f.opened) != 0 {
		t.Fatalf("system app: %+v, OS opener ran for %v", got, *f.opened)
	}
	cmd.Way = model.WayBeside
	got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if want := [][]string{{"open", "--split", "right", got.Path}}; got.Err != nil || !slices.EqualFunc(*asked, want, slices.Equal) || len(*f.opened) != 0 {
		t.Fatalf("beside: %+v, tern asked %v, OS opener %v", got, *asked, *f.opened)
	}
}

func TestOpenAttachmentBesideRefusesAWordDocumentOnATernThatDoesNotDrawIt(t *testing.T) {
	f := newDocbankFixture(t)
	t.Setenv("TERN_PANE", "7")
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) {
		if args[0] != "--version" {
			t.Errorf("tern ran %q", args)
		}
		return []byte("tern 0.9.20 (x)"), nil
	}

	cmd := f.openCmd(t, "plan.docx")
	cmd.Way = model.WayBeside
	got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "Word documents need Tern 0.9.21 or newer") {
		t.Fatalf("opened %+v, want the Tern version it needs", got)
	}
}
