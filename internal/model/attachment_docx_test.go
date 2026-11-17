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

func TestTernDrawsVideoAudioAndPDFsSinceV0926ButTheOSStillOpensThem(t *testing.T) {
	for _, name := range []string{"a.mp4", "A.MOV", "a.m4v", "a.webm", "a.mp3", "a.wav", "a.ogg", "a.flac", "a.m4a", "a.pdf"} {
		kind, since := model.TernNeeds(name)
		if !model.TernDraws(name) || !model.OSOpens(name) || since != "v0.9.26" || kind == "" {
			t.Errorf("%q: TernDraws %v, OSOpens %v, needs %q %q, want true, true, v0.9.26", name, model.TernDraws(name), model.OSOpens(name), kind, since)
		}
		a := model.Attachment{Name: name}
		if model.AttachmentRefusal(a, model.WayBeside) != "" || model.AttachmentRefusal(a, model.WaySystem) != "" {
			t.Errorf("%q: beside %q, system %q, want both allowed", name, model.AttachmentRefusal(a, model.WayBeside), model.AttachmentRefusal(a, model.WaySystem))
		}
	}
	for _, name := range []string{"a.avi", "a.mkv"} {
		if model.TernDraws(name) || !model.OSOpens(name) || model.AttachmentRefusal(model.Attachment{Name: name}, model.WayBeside) == "" {
			t.Errorf("%q: TernDraws %v, OSOpens %v, beside %q, want false, true, a refusal", name, model.TernDraws(name), model.OSOpens(name), model.AttachmentRefusal(model.Attachment{Name: name}, model.WayBeside))
		}
	}
}
