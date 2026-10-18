package model_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// recurrenceBase is the Issue list on Wednesday 2026-10-21 in Zurich, the Issue "parent" selected.
var errBoom = errors.New("boom")

func recurrenceBase() model.Model {
	m := clickBase()
	m.LocalDate, m.TimeZone = "2026-10-21", "Europe/Zurich"
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
	want := model.CreateRecurrenceCmd{UID: "parent", Workspace: "/repo", Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=WE", Start: "2026-10-21", TimeZone: "Europe/Zurich"}
	if len(cmds) != 1 || cmds[0] != want {
		t.Fatalf("saving the defaults sent %#v, want %#v", cmds, want)
	}
	if !m.RecurrenceForm.Saving {
		t.Fatal("the card does not show that it is saving")
	}
}

func TestRecurrenceCardStartsOnTheIssuesScheduledDateAndFallsBackToUTC(t *testing.T) {
	m := recurrenceBase()
	m.TimeZone = ""
	scheduled := "2026-10-26"
	m, _ = model.Update(m, model.DetailLoaded{UID: "parent", Selected: &model.IssueDetailData{Issue: &model.Issue{UID: "parent", ScheduledOnDate: &scheduled}}})
	_, cmds := saveRecurrence(openRecurrenceCard(m))
	want := model.CreateRecurrenceCmd{UID: "parent", Workspace: "/repo", Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO", Start: scheduled, TimeZone: "UTC"}
	if len(cmds) != 1 || cmds[0] != want {
		t.Fatalf("sent %#v, want %#v", cmds, want)
	}
}

func TestRecurrenceCardHidesFieldsThatDoNotApplyAndTabSkipsThem(t *testing.T) {
	all := []model.Field{model.RecurrenceFrequency, model.RecurrenceEvery, model.RecurrenceWeekdays, model.RecurrenceDay, model.RecurrenceMonth, model.RecurrenceEnds, model.RecurrenceEnd, model.RecurrenceStart, model.RecurrenceZone}
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
		{"day not a day", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "someday")
		}, "Day must be a number from 1 to 31"},
		{"count zero", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndCount)), model.RecurrenceEnd, "0")
		}, "Count must be a whole number, 1 or more."},
		{"end before start", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndUntil)), model.RecurrenceEnd, "2026-10-20")
		}, "End date must not be before the start date."},
		{"end not a date", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceEnds, ends(model.EndUntil)), model.RecurrenceEnd, "soon")
		}, "End date must be a date like"},
		{"start not a date", func(m model.Model) model.Model { return fillField(m, model.RecurrenceStart, "tomorrow") }, "Start date must be a date like"},
		{"unknown zone", func(m model.Model) model.Model { return fillField(m, model.RecurrenceZone, "Mars/Base") }, `Unknown time zone "Mars/Base".`},
		{"empty zone", func(m model.Model) model.Model { return fillField(m, model.RecurrenceZone, "") }, `Unknown time zone "".`},
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
			m, _ = clickPress(m, "left", "left", "space") // Wednesday back to Monday
			m, _ = clickPress(m, "right", "right", "right", "space") // on to Thursday
			return m
		}, "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE,TH"},
		{"monthly on day 15", func(m model.Model) model.Model {
			return fillField(choose(m, model.RecurrenceFrequency, frequency(model.Monthly)), model.RecurrenceDay, "15")
		}, "FREQ=MONTHLY;INTERVAL=1;BYMONTHDAY=15"},
		{"monthly on the start date's day", func(m model.Model) model.Model {
			return choose(m, model.RecurrenceFrequency, frequency(model.Monthly))
		}, "FREQ=MONTHLY;INTERVAL=1"},
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
		{"yearly in March", func(m model.Model) model.Model {
			m = choose(m, model.RecurrenceFrequency, frequency(model.Yearly))
			return choose(m, model.RecurrenceMonth, func(s model.RecurrenceFormState) bool { return s.Month == 3 })
		}, "FREQ=YEARLY;INTERVAL=1;BYMONTH=3"},
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

func TestRecurrenceCardOnlyMakesAnIssueRecurThatDoesNot(t *testing.T) {
	recurrence := int64(4)
	for _, tt := range []struct {
		name   string
		detail *model.IssueDetailData
	}{
		{"recurring", &model.IssueDetailData{Issue: &model.Issue{UID: "parent", RecurrenceID: &recurrence}, Recurrence: &model.Recurrence{UID: "r", Rule: "FREQ=DAILY;INTERVAL=1"}}},
		{"stopped", &model.IssueDetailData{Issue: &model.Issue{UID: "parent", RecurrenceID: &recurrence}, Recurrence: &model.Recurrence{Stopped: true}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := model.Update(recurrenceBase(), model.DetailLoaded{UID: "parent", Selected: tt.detail})
			if m = openRecurrenceCard(m); m.TopCard() == model.CardRecurrence {
				t.Fatal("O opened a card to make a second recurrence")
			}
		})
	}
	listed := recurrenceBase()
	for i := range listed.Issues {
		listed.Issues[i].RecurrenceID = &recurrence
	}
	listed, _ = model.Update(listed, model.DetailLoaded{UID: "parent", Selected: nil})
	if listed = openRecurrenceCard(listed); listed.TopCard() == model.CardRecurrence {
		t.Fatal("O opened a card for a listed Issue that recurs")
	}
}

func TestRecurrenceCardClosesWhenSavedAndKeepsItselfOnAFailure(t *testing.T) {
	m, _ := saveRecurrence(openRecurrenceCard(recurrenceBase()))
	failed, _ := model.Update(m, model.RecurrenceResultMsg{UID: "parent", Err: errBoom})
	if failed.TopCard() != model.CardRecurrence || failed.RecurrenceForm.Saving || failed.RecurrenceForm.Err != errBoom {
		t.Fatalf("after a failure: card %d, form %+v", failed.TopCard(), failed.RecurrenceForm)
	}
	saved, _ := model.Update(m, model.RecurrenceResultMsg{UID: "parent"})
	if saved.TopCard() == model.CardRecurrence {
		t.Fatal("the card stayed open after the recurrence was made")
	}
}

func TestHelpSheetListsMakeRecurring(t *testing.T) {
	for _, group := range model.HelpSheet() {
		for _, line := range group.Lines {
			if line.Text == "make recurring" && slices.Equal(line.Keys, []string{"O"}) {
				return
			}
		}
	}
	t.Fatal("the help sheet has no line for O")
}
