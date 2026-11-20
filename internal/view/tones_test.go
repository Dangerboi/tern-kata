package view

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func TestIssueStateToneIsNeverAccentOrSuccess(t *testing.T) {
	if got := issueStateTone(false); got != ui.ToneNeutral {
		t.Errorf("open = %q, want neutral", got)
	}
	if got := issueStateTone(true); got != ui.ToneMuted {
		t.Errorf("closed = %q, want muted", got)
	}
}

func TestPriorityRamp(t *testing.T) {
	for _, tc := range []struct {
		p    *int64
		text string
		tone ui.Tone
	}{
		{nil, "", ui.ToneMuted},
		{ptrTo(0), "P0", ui.ToneError},
		{ptrTo(1), "P1", ui.ToneUser},
		{ptrTo(2), "P2", ui.ToneWarning},
		{ptrTo(3), "P3", ui.ToneInfo},
		{ptrTo(4), "P4", ui.ToneMuted},
	} {
		if text, tone := priorityText(tc.p); text != tc.text || tone != tc.tone {
			t.Errorf("priorityText(%v) = %q, %q; want %q, %q", tc.p, text, tone, tc.text, tc.tone)
		}
	}
}

func ptrTo(n int64) *int64 { return &n }

func TestLaneToneIsByNameAndCustomLanesCycle(t *testing.T) {
	for lane, want := range map[string]ui.Tone{
		"needs-triage":    ui.ToneWarning,
		"needs-info":      ui.ToneUser,
		"ready-for-agent": ui.ToneInfo,
		"ready-for-human": ui.TonePending,
		"waiting":         ui.ToneError,
		model.LaneDone:    ui.ToneSuccess,
		model.LaneNone:    ui.ToneMuted,
		"not-a-lane":      ui.ToneNeutral,
	} {
		if got := laneTone(model.DefaultLanes(), lane); got != want {
			t.Errorf("laneTone(%q) = %q, want %q", lane, got, want)
		}
	}
	custom := []string{"a", "b", "waiting", "c", "d", "e", "f"}
	for lane, want := range map[string]ui.Tone{
		"a": ui.ToneInfo, "b": ui.ToneUser, "waiting": ui.ToneError, "c": ui.TonePending,
		"d": ui.ToneWarning, "e": ui.ToneError, "f": ui.ToneInfo,
	} {
		if got := laneTone(custom, lane); got != want {
			t.Errorf("custom laneTone(%q) = %q, want %q", lane, got, want)
		}
	}
}

func TestOwnerSpanIsPlainAndUnassignedMuted(t *testing.T) {
	if got := ownerSpan("dana"); !reflect.DeepEqual(got, (ui.Span{T: "dana"})) {
		t.Errorf("owner = %+v", got)
	}
	if got := ownerSpan(""); !reflect.DeepEqual(got, (ui.Span{T: "unassigned", S: "muted"})) {
		t.Errorf("no owner = %+v", got)
	}
}

func TestLabelChipTakesALaneLabelsLaneTone(t *testing.T) {
	lanes := model.DefaultLanes()
	for _, lane := range lanes {
		if got := labelChip(lane, lanes).Tone; got != laneTone(lanes, lane) {
			t.Errorf("chip %q tone = %q, want its Lane's %q", lane, got, laneTone(lanes, lane))
		}
	}
	if chip := labelChip("bug", lanes); chip.Tone != ui.ToneNeutral || chip.Text != "bug" {
		t.Errorf("chip = %+v", chip)
	}
	if chip := labelChip("waiting", []string{"in-review"}); chip.Tone != ui.ToneNeutral {
		t.Errorf("a label that is not one of the Project's Lanes = %+v", chip)
	}
}

func TestRunStateTone(t *testing.T) {
	for state, want := range map[string]ui.Tone{
		"succeeded": ui.ToneSuccess, "failed": ui.ToneError,
		"held": ui.ToneWarning, "paused": ui.ToneWarning, "expired": ui.ToneWarning,
		"running": ui.TonePending, "pending": ui.TonePending,
		"cancelled": ui.ToneMuted, "something-new": ui.ToneMuted,
	} {
		if got := runStateTone(state); got != want {
			t.Errorf("runStateTone(%q) = %q, want %q", state, got, want)
		}
	}
}

// relativeLuminance and contrast are the WCAG 2.x formulas smoke/steps/36-accent.sh uses.
func relativeLuminance(hex string) float64 {
	channel := func(s string) float64 {
		n, _ := strconv.ParseUint(s, 16, 8)
		c := float64(n) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	hex = strings.TrimPrefix(hex, "#")
	return 0.2126*channel(hex[0:2]) + 0.7152*channel(hex[2:4]) + 0.0722*channel(hex[4:6])
}

func contrast(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// The two accent fills measured in dark-catppuccin and Tern's light theme both take white text at AA.
func TestWhiteOnAccentFillPassesAA(t *testing.T) {
	for name, fill := range map[string]string{"dark-catppuccin": "#9f5f34", "light": "#537979"} {
		if got := contrast("#ffffff", fill); got < 4.5 {
			t.Errorf("%s: white on %s = %.2f, want at least 4.5", name, fill, got)
		}
	}
}

func TestSelectedRowsShareOneOnAccentToken(t *testing.T) {
	if strings.Contains(CSS, "#fff") {
		t.Error("the style sheet hard-codes #fff")
	}
	for _, want := range []string{"--kata-sel-fill: var(--accent-fill)", "--kata-sel-text: white"} {
		if !strings.Contains(CSS, want) {
			t.Errorf("the style sheet lacks %q", want)
		}
	}
	for _, rule := range []string{SelectedCSS, TwoLineRowCSS} {
		if !strings.Contains(rule, "var(--kata-sel-text)") || !strings.Contains(rule, "var(--kata-sel-fill)") {
			t.Errorf("a selected-row rule does not read the token pair: %s", rule)
		}
	}
	if !strings.Contains(CSS, "[data-role~='"+ActiveTabRole+"']") {
		t.Error("the style sheet lacks the active tab rule")
	}
}

// Tern's surface stylesheet parser rejects at-rules, and a rejected sheet keeps the Pane from starting.
func TestStyleSheetHasNoAtRules(t *testing.T) {
	if strings.Contains(CSS, "@") {
		t.Error("the style sheet contains an at-rule, which Tern's surface parser refuses")
	}
}
