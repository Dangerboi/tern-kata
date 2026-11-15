package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTernDrawsWordDocumentsButTheOSNeverOpensThem(t *testing.T) {
	for _, name := range []string{"plan.docx", "PLAN.DOCX", "plan.final.Docx"} {
		if !model.TernDraws(name) || model.OSOpens(name) {
			t.Errorf("%q: TernDraws %v, OSOpens %v, want true and false", name, model.TernDraws(name), model.OSOpens(name))
		}
		a := model.Attachment{Name: name}
		if got := model.AttachmentRefusal(a, model.WayBeside); got != "" {
			t.Errorf("%q beside refused: %s", name, got)
		}
		if model.AttachmentRefusal(a, model.WaySystem) == "" {
			t.Errorf("%q was allowed for the system app", name)
		}
	}
	for _, name := range []string{"plan.doc", "plan.docm", "plan.docx.", "plan.docx ", "plan.xlsx", "plan.pptx", "plan.odt"} {
		if model.TernDraws(name) {
			t.Errorf("TernDraws(%q) = true", name)
		}
	}
}

func TestTernDrawsKeepsTheExistingKinds(t *testing.T) {
	for name, want := range map[string]bool{
		"a.png": true, "a.JPG": true, "a.jpeg": true, "a.gif": true, "a.txt": true, "a.md": true, "a.json": true,
		"a.webp": false, "a.pdf": false, "a.mp4": false, "a.csv": false, "a.log": false, "a.bat": false,
	} {
		if got := model.TernDraws(name); got != want {
			t.Errorf("TernDraws(%q) = %v, want %v", name, got, want)
		}
	}
}
