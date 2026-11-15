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
	var asked [][]string
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) { asked = append(asked, args); return nil, nil }

	got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "plan.docx")))
	if want := [][]string{{"open", "--preview", got.Path}}; got.Err != nil || !slices.EqualFunc(asked, want, slices.Equal) {
		t.Fatalf("opened %+v, tern was asked %v, want %v", got, asked, want)
	}
	if len(*f.opened) != 0 || got.OutsideTern {
		t.Fatalf("the OS opener ran: %v (outside Tern %v)", *f.opened, got.OutsideTern)
	}
}

func TestOpenAttachmentOnlySavesAWordDocumentWhenTernCannotShowIt(t *testing.T) {
	for name, tc := range map[string]struct {
		env  func(*testing.T)
		runs func(context.Context, []string) ([]byte, error)
	}{
		"not in a Tern Pane": {func(*testing.T) {}, func(context.Context, []string) ([]byte, error) { t.Error("tern ran outside a Pane"); return nil, nil }},
		"tern exits non-zero": {func(t *testing.T) { t.Setenv("TERN_PANE", "7") }, func(context.Context, []string) ([]byte, error) {
			return []byte("no window answers"), errors.New("exit status 1")
		}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newDocbankFixture(t)
			tc.env(t)
			f.gateway.runTern = tc.runs
			got := as[model.AttachmentOpened](t, run(t, f.gateway, f.openCmd(t, "plan.docx")))
			if got.Err == nil || got.Path == "" || !strings.Contains(got.Err.Error(), "saved at "+got.Path+" and not opened: tern-kata opens only images, video, audio, PDFs and text") {
				t.Fatalf("opened %+v", got)
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
	var asked [][]string
	f.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) { asked = append(asked, args); return nil, nil }

	cmd := f.openCmd(t, "plan.docx")
	cmd.Way = model.WaySystem
	if got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd)); got.Err == nil || len(*f.opened) != 0 {
		t.Fatalf("system app: %+v, OS opener ran for %v", got, *f.opened)
	}
	cmd.Way = model.WayBeside
	got := as[model.AttachmentOpened](t, run(t, f.gateway, cmd))
	if want := [][]string{{"open", "--split", "right", got.Path}}; got.Err != nil || !slices.EqualFunc(asked, want, slices.Equal) || len(*f.opened) != 0 {
		t.Fatalf("beside: %+v, tern asked %v, OS opener %v", got, asked, *f.opened)
	}
}
