package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestAttachmentPointerRoundTripsThroughMeta(t *testing.T) {
	want := model.Attachment{Name: "a.png", Host: "box", Path: "/s/a.png"}
	meta := map[string]any{model.AttachmentKey(want.Name): want.Pointer()}
	if got := model.Attachments(meta); !reflect.DeepEqual(got, []model.Attachment{want}) {
		t.Fatalf("got %v", got)
	}
	if !reflect.DeepEqual(meta["attachment.a.png"], map[string]any{"host": "box", "path": "/s/a.png"}) {
		t.Fatalf("meta %v", meta)
	}
}

func TestAttachmentOnHost(t *testing.T) {
	for _, c := range []struct {
		pointer, host string
		want          bool
	}{{"Box", "box", true}, {"box", "BOX", true}, {"box", "other", false}, {"", "", false}, {"", "box", false}, {"box", "", false}} {
		if got := (model.Attachment{Host: c.pointer}).OnHost(c.host); got != c.want {
			t.Errorf("OnHost(%q) of pointer host %q = %v", c.host, c.pointer, got)
		}
	}
}
