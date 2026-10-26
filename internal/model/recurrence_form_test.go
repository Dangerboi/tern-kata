package model_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// recurrenceBase is the Issue list on Wednesday 2026-10-21 in Zurich, the Issue "parent" selected.
var errBoom = errors.New("boom")

func recurrenceBase() model.Model {
	m := clickBase()
	m.Calendar = model.NewPaneCalendar("2026-10-21", "Europe/Zurich", "")
	return m
}

func openRecurrenceCard(m model.Model) model.Model {
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+o"})
	return m
}

// fillField replaces what a text field of the open card holds, as typing does.
func fillField(m model.Model, field model.Field, text string) model.Model {
	m, _ = model.Update(m, model.FocusFieldMsg{Field: field})
	m, _ = model.Update(m, model.KeyMsg{Chord: "ctrl+u"})
	for _, r := range text {
		key := model.KeyMsg{Chord: string(r), Text: string(r)}
		if r == ' ' {
			key = model.KeyMsg{Chord: "space"}
		}
		m, _ = model.Update(m, key)
	}
	return m
}

// choose puts a choice field on the choice reached by pressing right until pick says it is shown.
func choose(m model.Model, field model.Field, pick func(model.RecurrenceFormState) bool) model.Model {
	m, _ = model.Update(m, model.FocusFieldMsg{Field: field})
	for range 16 {
		if pick(m.RecurrenceForm) {
			return m
		}
		m, _ = model.Update(m, model.KeyMsg{Chord: "right"})
	}
	return m
}

func frequency(f model.Frequency) func(model.RecurrenceFormState) bool {
	return func(s model.RecurrenceFormState) bool { return s.Frequency == f }
}

func ends(e model.EndKind) func(model.RecurrenceFormState) bool {
	return func(s model.RecurrenceFormState) bool { return s.Ends == e }
}

func saveRecurrence(m model.Model) (model.Model, []model.Cmd) {
	return model.Update(m, model.KeyMsg{Chord: "ctrl+o"})
}

func TestRecurrenceCardOpensWithWeeklyDefaults(t *testing.T) {
	m := openRecurrenceCard(recurrenceBase())
	if m.TopCard() != model.CardRecurrence {
		t.Fatalf("O opened card %d", m.TopCard())
	}
	m, cmds := saveRecurrence(m)
	want := model.CreateRecurrenceCmd{UID: "parent", Workspace: "/repo", RecurrenceTerms: model.RecurrenceTerms{Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=WE", Start: "2026-10-21", TimeZone: "Europe/Zurich"}}
	if len(cmds) != 1 || cmds[0] != want {
		t.Fatalf("saving the defaults sent %#v, want %#v", cmds, want)
	}
	if !m.RecurrenceForm.Saving {
		t.Fatal("the card does not show that it is saving")
	}
}

func TestRecurrenceCardOpensValidWhenThePaneZoneIsLocal(t *testing.T) {
	m := recurrenceBase()
	m.Calendar = model.NewPaneCalendar("2026-10-21", "Local", "")
	m = openRecurrenceCard(m)
	if m.RecurrenceForm.Zone == "Local" {
		t.Fatal("the card opened on the zone Local")
	}
	if _, cmds := saveRecurrence(m); len(cmds) != 1 {
		t.Fatalf("saving the defaults sent %#v", cmds)
	}
}

func TestRecurTermsRefuseTheLocalZone(t *testing.T) {
	target := model.RecurTarget{UID: "parent", Today: "2026-10-21", Zone: "Europe/Zurich"}
	for _, flags := range []map[string]string{
		{"zone": "Local", "frequency": "daily"},
		{"zone": "Local", "rule": "FREQ=DAILY"},
	} {
		if _, err := target.Terms(flags); err == nil || err.Error() != `Unknown time zone "Local".` {
			t.Fatalf("Terms(%v) error = %v", flags, err)
		}
	}
	if terms, err := (model.RecurTarget{UID: "parent", Today: "2026-10-21", Zone: "Local"}).Terms(map[string]string{"frequency": "daily"}); err != nil || terms.TimeZone == "Local" {
		t.Fatalf("a Local Pane zone gave %+v, %v", terms, err)
	}
}

func TestRecurrenceCardStartsOnTheIssuesScheduledDateAndFallsBackToUTC(t *testing.T) {
	m := recurrenceBase()
	m.Calendar = model.PaneCalendar{Date: m.Calendar.Date}
	scheduled := "2026-10-26"
	m, _ = model.Update(m, model.DetailLoaded{UID: "parent", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "parent", ScheduledOnDate: &scheduled}}})
	_, cmds := saveRecurrence(openRecurrenceCard(m))
	want := model.CreateRecurrenceCmd{UID: "parent", Workspace: "/repo", RecurrenceTerms: model.RecurrenceTerms{Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO", Start: scheduled, TimeZone: "UTC"}}
	if len(cmds) != 1 || cmds[0] != want {
		t.Fatalf("sent %#v, want %#v", cmds, want)
	}
}

func TestRecurrenceCardHidesFieldsThatDoNotApplyAndTabSkipsThem(t *testing.T) {
	all := []model.Field{model.RecurrenceFrequency, model.RecurrenceEvery, model.RecurrenceWeekdays, model.RecurrenceDay, model.RecurrenceMonth, model.RecurrenceStart, model.RecurrenceEnds, model.RecurrenceEnd, model.RecurrenceZone}
	without := func(hidden ...model.Field) []model.Field {
		return slices.DeleteFunc(slices.Clone(all), func(f model.Field) bool { return slices.Contains(hidden, f) })
	}
	for _, tt := range []struct {
		name string
		set  func(model.Model) model.Model
		want []model.Field
	}{
		{"daily", func(m model.Model) model.Model { return choose(m, model.RecurrenceFrequency, frequency(model.Daily)) }, without(model.RecurrenceWeekdays, model.RecurrenceDay, model.RecurrenceMonth, model.RecurrenceEnd)},
		{"weekly", func(m model.Model) model.Model { return m }, without(model.RecurrenceDay, model.RecurrenceMonth, model.RecurrenceEnd)},
		{"monthly", func(m model.Model) model.Model { return choose(m, model.RecurrenceFrequency, frequency(model.Monthly)) }, without(model.RecurrenceWeekdays, model.RecurrenceMonth, model.RecurrenceEnd)},
		{"yearly", func(m model.Model) model.Model { return choose(m, model.RecurrenceFrequency, frequency(model.Yearly)) }, without(model.RecurrenceWeekdays, model.RecurrenceEnd)},
		{"weekly ending on a date", func(m model.Model) model.Model { return choose(m, model.RecurrenceEnds, ends(model.EndUntil)) }, without(model.RecurrenceDay, model.RecurrenceMonth)},
		{"weekly ending after a count", func(m model.Model) model.Model { return choose(m, model.RecurrenceEnds, ends(model.EndCount)) }, without(model.RecurrenceDay, model.RecurrenceMonth)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.set(openRecurrenceCard(recurrenceBase()))
			m, _ = model.Update(m, model.FocusFieldMsg{Field: model.RecurrenceFrequency})
			var forward []model.Field
			for range tt.want {
				forward = append(forward, m.RecurrenceForm.Field)
				m, _ = clickPress(m, "tab")
			}
			if !slices.Equal(forward, tt.want) || m.RecurrenceForm.Field != tt.want[0] {
				t.Fatalf("tab visits %v and wraps to %q, want %v", forward, m.RecurrenceForm.Field, tt.want)
			}
			var back []model.Field
			for range tt.want {
				m, _ = clickPress(m, "shift+tab")
				back = append(back, m.RecurrenceForm.Field)
			}
			slices.Reverse(back)
			if !slices.Equal(back, tt.want) {
				t.Fatalf("shift+tab visits %v, want the same fields in reverse", back)
			}
		})
	}
}

func TestRecurrenceCardRefusesAnInvalidScheduleAndSendsNothing(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(model.Model) model.Model
		want string
	}{
		{"every zero", func(m model.Model) model.Model { return fillField(m, model.RecurrenceEvery, "0") }, "Every must be a whole number, 1 or more."},
		{"every empty", func(m model.Model) model.Model { return fillField(m, model.RecurrenceEvery, "") }, "Every must be a whole number, 1 or more."},
		{"no weekday", func(m model.Model) model.Model {
			m, _ = model.Update(m, model.FocusFieldMsg{Field: model.RecurrenceWeekdays})
			// The cursor starts on the start date's weekday, Wednesday: space turns it off.
			m, _ = clickPress(m, "space")
			return m
		}, "Pick at least one weekday."},
		{"day 32", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "32")
		}, "Day must be between 1 and 31."},
		{"day 0", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Yearly)), model.RecurrenceDay, "0")
		}, "Day must be between 1 and 31."},
		{"day empty", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "")
		}, "Day must not be empty"},
		{"yearly last day", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Yearly)), model.RecurrenceDay, "last")
		}, "The last day works for monthly schedules only."},
		{"day not a day", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "someday")
		}, "Day must be a number from 1 to 31"},
		{"count zero", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndCount)), model.RecurrenceEnd, "0")
		}, "Occurrences must be a whole number, 1 or more."},
		{"end before start", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndUntil)), model.RecurrenceEnd, "2026-10-20")
		}, "End date must not be before the start date."},
		{"end not a date", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndUntil)), model.RecurrenceEnd, "soon")
		}, "End date must be a date like"},
		{"start not a date", func(m model.Model) model.Model { return fillField(m, model.RecurrenceStart, "tomorrow") }, "Start date must be a date like"},
		{"unknown zone", func(m model.Model) model.Model { return fillField(m, model.RecurrenceZone, "Mars/Base") }, `Unknown time zone "Mars/Base".`},
		{"empty zone", func(m model.Model) model.Model { return fillField(m, model.RecurrenceZone, "") }, `Unknown time zone "".`},
		{"Local zone", func(m model.Model) model.Model { return fillField(m, model.RecurrenceZone, "Local") }, `Unknown time zone "Local".`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := saveRecurrence(tt.set(openRecurrenceCard(recurrenceBase())))
			if len(cmds) != 0 || m.RecurrenceForm.Saving {
				t.Fatalf("sent %#v (saving %v)", cmds, m.RecurrenceForm.Saving)
			}
			if err := m.RecurrenceForm.Err; err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRecurrenceCardWritesTheRuleTheFormDescribes(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(model.Model) model.Model
		rule string
	}{
		{"every 2 weeks on Mon, Wed and Thu", func(m model.Model) model.Model {
			m = fillField(m, model.RecurrenceEvery, "2")
			m, _ = model.Update(m, model.FocusFieldMsg{Field: model.RecurrenceWeekdays})
			m, _ = clickPress(m, "left", "left", "space")            // Wednesday back to Monday
			m, _ = clickPress(m, "right", "right", "right", "space") // on to Thursday
			return m
		}, "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE,TH"},
		{"monthly on day 15", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "15")
		}, "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=15"},
		{"monthly on the start date's day, prefilled", func(m model.Model) model.Model {
			return choose(m, model.RecurrenceFrequency, frequency(model.Monthly))
		}, "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=21"},
		{"monthly on the 2nd Tuesday", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "2nd tue")
		}, "FREQ=MONTHLY;INTERVAL=1;BYDAY=2TU"},
		{"monthly on the last Friday", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "last friday")
		}, "FREQ=MONTHLY;INTERVAL=1;BYDAY=-1FR"},
		{"monthly on the last day", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "last")
		}, "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=-1"},
		{"yearly in the start date's month on day 3", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Yearly)), model.RecurrenceDay, "3")
		}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=10;BYMONTHDAY=3"},
		{"yearly in March, day prefilled", func(m model.Model) model.Model {
			m = choose(m, model.RecurrenceFrequency, frequency(model.Yearly))
			return choose(m, model.RecurrenceMonth, func(s model.RecurrenceFormState) bool { return s.Month == 3 })
		}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=3;BYMONTHDAY=21"},
		{"yearly on the 2nd Tuesday", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Yearly)), model.RecurrenceDay, "2nd tue")
		}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=10;BYDAY=2TU"},
		{"daily until a date", func(m model.Model) model.Model {
			m = choose(m, model.RecurrenceFrequency, frequency(model.Daily))
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndUntil)), model.RecurrenceEnd, "2026-12-31")
		}, "FREQ=DAILY;INTERVAL=1;UNTIL=20261231"},
		{"daily 10 times", func(m model.Model) model.Model {
			m = choose(m, model.RecurrenceFrequency, frequency(model.Daily))
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndCount)), model.RecurrenceEnd, "10")
		}, "FREQ=DAILY;INTERVAL=1;COUNT=10"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := saveRecurrence(tt.set(openRecurrenceCard(recurrenceBase())))
			if len(cmds) != 1 {
				t.Fatalf("sent %#v; the card says %v", cmds, m.RecurrenceForm.Err)
			}
			if got := cmds[0].(model.CreateRecurrenceCmd).Rule; got != tt.rule {
				t.Fatalf("rule %q, want %q", got, tt.rule)
			}
		})
	}
}

func TestOnAStoppedRecurrenceOTellsSoAndWritesNothing(t *testing.T) {
	recurrence := int64(4)
	detail := &model.IssueDetailData{Issue: &model.Issue{UID: "parent", RecurrenceID: &recurrence}, Recurrence: &model.Recurrence{Stopped: true}}
	m, _ := model.Update(recurrenceBase(), model.DetailLoaded{UID: "parent", Selected: detail})
	m = openRecurrenceCard(m)
	f := m.RecurrenceForm
	if m.TopCard() != model.CardRecurrence || !f.Refused || f.Err == nil || !strings.Contains(f.Err.Error(), "stopped") {
		t.Fatalf("O on a stopped recurrence: card %d, form %+v, want a card saying it was stopped", m.TopCard(), f)
	}
	m, cmds := clickPress(m, "ctrl+o", "ctrl+x", "x")
	if len(cmds) != 0 || m.RecurrenceForm.Saving || m.RecurrenceForm.StopConfirm {
		t.Fatalf("the notice card wrote or asked: %#v %+v", cmds, m.RecurrenceForm)
	}
	if m, _ = clickPress(m, "escape"); m.TopCard() == model.CardRecurrence {
		t.Fatal("esc left the notice card open")
	}
}

func TestOOnARecurringIssueWithTheDetailHiddenLoadsItsRecurrenceThenOpensTheCard(t *testing.T) {
	recurrence := int64(4)
	detailWith := func(rule string) (model.DetailLoaded, *model.Recurrence) {
		rec := &model.Recurrence{UID: "rec-uid", RecurrenceTerms: model.RecurrenceTerms{Rule: rule, Start: "2026-10-19", TimeZone: "Europe/Zurich"}, Revision: 3}
		return model.DetailLoaded{UID: "parent", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "parent", RecurrenceID: &recurrence}, Recurrence: rec}}, rec
	}
	// The detail was loaded, then hidden; snapshots no longer refresh it, so what it holds may be old.
	old, _ := detailWith("FREQ=WEEKLY;INTERVAL=1;BYDAY=MO")
	m, _ := model.Update(recurrenceBase(), old)
	m, _ = clickPress(m, "shift+l")
	if !m.DetailHidden() {
		t.Fatal("the detail is still shown")
	}
	m, cmds := clickPress(m, "shift+o")
	if m.TopCard() == model.CardRecurrence || !slices.Contains(cmds, model.Cmd(model.LoadDetail{Workspace: "/repo", UID: "parent"})) {
		t.Fatalf("O with the detail hidden: card %d, sent %#v, want the detail asked for and no card yet", m.TopCard(), cmds)
	}
	fresh, rec := detailWith("FREQ=DAILY;INTERVAL=1")
	m, _ = model.Update(m, fresh)
	if f := m.RecurrenceForm; m.TopCard() != model.CardRecurrence || !f.Change || f.Frequency != model.Daily || f.Opened != *rec {
		t.Fatalf("after the detail arrived: card %d, form %+v, want Change recurrence prefilled from the fresh recurrence", m.TopCard(), f)
	}
}

func TestOOnARecurringIssueWhoseRecurrenceKataWithholdsSaysSo(t *testing.T) {
	recurrence := int64(4)
	m, _ := model.Update(recurrenceBase(), model.DetailLoaded{UID: "parent", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "parent", RecurrenceID: &recurrence}}})
	m = openRecurrenceCard(m)
	if f := m.RecurrenceForm; m.TopCard() != model.CardRecurrence || !f.Refused || f.Err == nil {
		t.Fatalf("O with no recurrence provided: card %d, form %+v, want a card saying why nothing can change", m.TopCard(), f)
	}
}

func TestRecurrenceCardClosesWhenSavedAndKeepsItselfOnAFailure(t *testing.T) {
	m, _ := saveRecurrence(openRecurrenceCard(recurrenceBase()))
	failed, _ := model.Update(m, model.RecurrenceResultMsg{UID: "parent", Err: errBoom})
	if failed.TopCard() != model.CardRecurrence || failed.RecurrenceForm.Saving || !errors.Is(failed.RecurrenceForm.Err, errBoom) {
		t.Fatalf("after a failure: card %d, form %+v", failed.TopCard(), failed.RecurrenceForm)
	}
	saved, _ := model.Update(m, model.RecurrenceResultMsg{UID: "parent"})
	if saved.TopCard() == model.CardRecurrence {
		t.Fatal("the card stayed open after the recurrence was made")
	}
}

// recurring is the Issue "parent" in a recurrence with the rule, as the detail shows it.
func recurring(rule string) model.Model {
	recurrence := int64(4)
	rec := &model.Recurrence{UID: "rec-uid", RecurrenceTerms: model.RecurrenceTerms{Rule: rule, Start: "2026-10-19", TimeZone: "Europe/Zurich"}, Revision: 3, Next: "2026-10-26"}
	m, _ := model.Update(recurrenceBase(), model.DetailLoaded{UID: "parent", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "parent", RecurrenceID: &recurrence}, Recurrence: rec}})
	return m
}

func TestRecurrenceCardOpensAsChangeRecurrencePrefilledFromTheSchedule(t *testing.T) {
	m := openRecurrenceCard(recurring("FREQ=MONTHLY;INTERVAL=2;BYDAY=2TU;COUNT=5"))
	f := m.RecurrenceForm
	if m.TopCard() != model.CardRecurrence || !f.Change || f.Raw != "" {
		t.Fatalf("O opened card %d, form %+v", m.TopCard(), f)
	}
	if f.Frequency != model.Monthly || f.Every != "2" || f.Day != "2 tue" || f.Ends != model.EndCount || f.Count != "5" || f.Start != "2026-10-19" || f.Zone != "Europe/Zurich" {
		t.Fatalf("prefill %+v", f)
	}
	// Saving what the card shows sends the same rule, to the recurrence it read, with what it read as the guard.
	_, cmds := saveRecurrence(m)
	terms := model.RecurrenceTerms{Rule: "FREQ=MONTHLY;INTERVAL=2;BYDAY=2TU;COUNT=5", Start: "2026-10-19", TimeZone: "Europe/Zurich"}
	want := model.ChangeRecurrenceCmd{Workspace: "/repo", UID: "parent", RecurrenceUID: "rec-uid", RecurrenceTerms: terms, Opened: terms}
	if len(cmds) != 1 || cmds[0] != want {
		t.Fatalf("saving sent %#v, want %#v", cmds, want)
	}
}

func TestRecurrenceCardShowsARawRuleAboveTheFormAndSavingTheFormReplacesIt(t *testing.T) {
	const raw = "FREQ=WEEKLY;BYDAY=MO;BYHOUR=9"
	m := openRecurrenceCard(recurring(raw))
	if m.RecurrenceForm.Raw != raw || !m.RecurrenceForm.Change {
		t.Fatalf("form %+v, want the raw rule kept for display", m.RecurrenceForm)
	}
	_, cmds := saveRecurrence(m)
	if len(cmds) != 1 {
		t.Fatalf("sent %#v", cmds)
	}
	got := cmds[0].(model.ChangeRecurrenceCmd)
	if got.Rule != "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO" || got.Opened.Rule != raw {
		t.Fatalf("saving sent rule %q guarded by %q, want the form's rule guarded by the raw one", got.Rule, got.Opened.Rule)
	}
}

func TestRecurrenceCardReloadsTheCurrentScheduleWhenItChangedMeanwhile(t *testing.T) {
	m, _ := saveRecurrence(openRecurrenceCard(recurring("FREQ=DAILY;INTERVAL=1")))
	current := model.Recurrence{UID: "rec-uid", RecurrenceTerms: model.RecurrenceTerms{Rule: "FREQ=WEEKLY;INTERVAL=3;BYDAY=FR", Start: "2026-11-02", TimeZone: "UTC"}, Revision: 5}
	m, cmds := model.Update(m, model.RecurrenceResultMsg{UID: "parent", Changed: &current})
	f := m.RecurrenceForm
	if m.TopCard() != model.CardRecurrence || f.Saving || f.Frequency != model.Weekly || f.Every != "3" || !f.Weekdays[time.Friday] || f.Start != "2026-11-02" || f.Zone != "UTC" || f.Opened != current {
		t.Fatalf("card %d, form %+v, want it reloaded with the current schedule", m.TopCard(), f)
	}
	if f.Err == nil || !strings.Contains(f.Err.Error(), "changed meanwhile") {
		t.Fatalf("message %v, want one saying the schedule changed meanwhile", f.Err)
	}
	if len(cmds) == 0 {
		t.Fatal("the detail was not refreshed")
	}
}

func TestRecurrenceCardStopAsksYesOrNoThenStops(t *testing.T) {
	m := openRecurrenceCard(recurring("FREQ=DAILY;INTERVAL=1"))
	asked, cmds := model.Update(m, model.KeyMsg{Chord: "ctrl+x"})
	if !asked.RecurrenceForm.StopConfirm || len(cmds) != 0 {
		t.Fatalf("ctrl+x: form %+v, sent %#v, want it to ask first", asked.RecurrenceForm, cmds)
	}
	if mode := asked.CurrentMode(); mode != model.ModeRecurrenceStop {
		t.Fatalf("mode %q while asking", mode)
	}
	for _, key := range []string{"n", "escape"} {
		kept, cmds := model.Update(asked, model.KeyMsg{Chord: key})
		if kept.RecurrenceForm.StopConfirm || !kept.RecurrenceForm.Open || len(cmds) != 0 {
			t.Fatalf("%s: form %+v, sent %#v, want the form back and nothing sent", key, kept.RecurrenceForm, cmds)
		}
	}
	swallowed, cmds := model.Update(asked, model.KeyMsg{Chord: "x", Text: "x"})
	if !swallowed.RecurrenceForm.StopConfirm || len(cmds) != 0 {
		t.Fatal("another key left the question")
	}
	stopping, cmds := model.Update(asked, model.KeyMsg{Chord: "y"})
	want := model.StopRecurrenceCmd{Workspace: "/repo", UID: "parent", RecurrenceUID: "rec-uid"}
	if len(cmds) != 1 || cmds[0] != want || !stopping.RecurrenceForm.Saving {
		t.Fatalf("y sent %#v (saving %v), want %#v", cmds, stopping.RecurrenceForm.Saving, want)
	}
	stopped, _ := model.Update(stopping, model.RecurrenceResultMsg{UID: "parent"})
	if stopped.TopCard() == model.CardRecurrence {
		t.Fatal("the card stayed open after the recurrence stopped")
	}
}

func TestRecurrenceCardOfANewRecurrenceHasNoStop(t *testing.T) {
	m, _ := model.Update(openRecurrenceCard(recurrenceBase()), model.KeyMsg{Chord: "ctrl+x"})
	if m.RecurrenceForm.StopConfirm {
		t.Fatal("ctrl+x asked to stop a recurrence that does not exist")
	}
}

func TestRecurrenceCardFillsAnEmptyDayWithTheStartDatesDay(t *testing.T) {
	// A rule the web UI writes without a day opens with the start date's day, and saving writes the explicit day.
	m := openRecurrenceCard(recurring("FREQ=MONTHLY;INTERVAL=1"))
	if m.RecurrenceForm.Day != "19" {
		t.Fatalf("Day %q, want the start date's day 19", m.RecurrenceForm.Day)
	}
	_, cmds := saveRecurrence(m)
	if got := cmds[0].(model.ChangeRecurrenceCmd).Rule; got != "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=19" {
		t.Fatalf("rule %q, want the explicit day", got)
	}
	// Changing the frequency fills a Day the user emptied.
	m = openRecurrenceCard(recurrenceBase())
	m = fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "")
	if m = choose(m, model.RecurrenceFrequency, frequency(model.Yearly)); m.RecurrenceForm.Day != "21" {
		t.Fatalf("Day %q after changing the frequency, want 21", m.RecurrenceForm.Day)
	}
}
