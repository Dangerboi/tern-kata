package model

import (
	"errors"
	"fmt"
)

const (
	ModeDatesForm Mode   = "dates-form"
	DatesSubmit   Action = "dates-submit"
	DatesCancel   Action = "dates-cancel"
	OpenDates     Action = "open-dates"
)

// The Dates card's fields.
const (
	DatesScheduled Field = "scheduled"
	DatesDeadline  Field = "deadline"
)

// DatesFields is the Dates card's field list, in tab order.
var DatesFields = []FieldSpec{{DatesScheduled, true}, {DatesDeadline, true}}

// The metadata keys kata keeps the two dates under.
const (
	ScheduledKey = "scheduled_on"
	DeadlineKey  = "deadline_on"
)

// DatesLabel is the name the Dates card and its messages give a field.
func DatesLabel(field Field) string {
	if field == DatesDeadline {
		return "Deadline"
	}
	return "Scheduled"
}

func datesKey(field Field) string {
	if field == DatesDeadline {
		return DeadlineKey
	}
	return ScheduledKey
}

// DatesPair holds one string for each date of the Dates card.
type DatesPair struct{ Scheduled, Deadline string }

func (p *DatesPair) at(field Field) *string {
	switch field {
	case DatesScheduled:
		return &p.Scheduled
	case DatesDeadline:
		return &p.Deadline
	}
	return nil
}

// Of is the string held for the field called name, "" for a name that is not a Dates field.
func (p DatesPair) Of(name Field) string {
	if s := p.at(name); s != nil {
		return *s
	}
	return ""
}

// DatesFormState is the Dates card. Text is what each field holds as typed, Prefill what it was opened with, and Held what kata
// held under the metadata key when it opened ("" for none): the value a save guards on. Prefill differs from Held only for a
// date kata projects without holding it in metadata.
type DatesFormState struct {
	Open, Saving bool
	Field        Field
	UID          string
	ProjectID    int64
	Text         DatesPair
	Prefill      DatesPair
	Held         DatesPair
	Caret        Caret
	Err          error
}

// Value is the text the field called name holds.
func (f DatesFormState) Value(name Field) string { return f.Text.Of(name) }

// DateChange is one date key a save writes: Value is the new day, "" to clear the key, and Was what the key held when the card
// opened, "" for absent.
type DateChange struct {
	Key, Label, Value, Was string
}

// SetDatesCmd describes a save of the Dates card in plain values: one patch of each changed key, each guarded on Was, in order,
// stopping at the first failure. The Daemon gateway turns it into kata's requests.
type SetDatesCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	Changes               []DateChange
}

func (SetDatesCmd) cmd() {}

// DatesResultMsg answers a SetDatesCmd. Applied lists the changes written before Err, if any, so a failed save can still be
// retried for the rest.
type DatesResultMsg struct {
	UID     string
	Applied []DateChange
	Err     error
}

func (DatesResultMsg) msg() {}

// DatesChangedError is the refusal for a key whose value moved since the card opened.
func DatesChangedError(label string) error {
	return fmt.Errorf("%s changed elsewhere since the card opened; it was not overwritten. Close the card and open it again to see the new date.", label)
}

// datePreview is the muted line under a date field: the day text resolves to with its weekday, "no date" for blank, and nothing
// for text that is not a date (saving refuses it).
func datePreview(text, today string) string {
	day, err := ParseDateInput(text, today)
	switch {
	case err != nil:
		return ""
	case day == "":
		return "no date"
	}
	return DateWithWeekday(day)
}

// dateDay is the day text resolves to, "" for blank, or an error naming the field by label.
func dateDay(label, text, today string) (string, error) {
	day, err := ParseDateInput(text, today)
	if err != nil {
		return "", fmt.Errorf("%s %s", label, err.Error())
	}
	return day, nil
}

// DatesPreview is the muted line under a field: the day its text resolves to with its weekday, "no date" for blank, and nothing
// for text that is not a date (saving refuses it). A prefilled value kata holds with a time shows its day.
func (m Model) DatesPreview(name Field) string {
	f := m.DatesForm
	text := f.Text.Of(name)
	if text == f.Prefill.Of(name) && text != "" {
		return DateWithWeekday(text[:min(len(text), len(dayLayout))])
	}
	return datePreview(text, m.Calendar.Date)
}

// openDates is `d`: the card prefilled with the Issue's two dates as kata holds them.
func openDates(m Model) Model {
	if m.Stale || m.DatesForm.Saving {
		return m
	}
	issue := closeIssue(m)
	if issue == nil {
		return m
	}
	f := DatesFormState{Open: true, Field: DatesScheduled, UID: issue.UID, ProjectID: issue.ProjectID}
	for _, field := range []Field{DatesScheduled, DatesDeadline} {
		key := datesKey(field)
		held, _ := issue.Metadata[key].(string)
		projected := ""
		if field == DatesScheduled && issue.ScheduledOnDate != nil {
			projected = *issue.ScheduledOnDate
		} else if field == DatesDeadline && issue.DeadlineOnDate != nil {
			projected = *issue.DeadlineOnDate
		}
		*f.Held.at(field) = held
		prefill := held
		if prefill == "" {
			prefill = projected
		}
		*f.Prefill.at(field), *f.Text.at(field) = prefill, prefill
	}
	m.DatesForm = f
	return m
}

// datesChanges reads the card into the keys a save writes; an unchanged field writes nothing, so a date kata holds with a time
// stays as it is. An input that is not a date is refused with an error naming its field.
func datesChanges(f DatesFormState, today string) ([]DateChange, error) {
	var changes []DateChange
	for _, spec := range DatesFields {
		name := spec.Name
		text, prefill := f.Text.Of(name), f.Prefill.Of(name)
		if text == prefill {
			continue
		}
		day, err := dateDay(DatesLabel(name), text, today)
		if err != nil {
			return nil, err
		}
		if day == prefill || (day == "" && prefill == "") {
			continue
		}
		changes = append(changes, DateChange{Key: datesKey(name), Label: DatesLabel(name), Value: day, Was: f.Held.Of(name)})
	}
	return changes, nil
}

func submitDates(m Model) (Model, []Cmd) {
	f := &m.DatesForm
	if f.Saving {
		return m, nil
	}
	if m.Stale {
		f.Err = errors.New("Daemon disconnected; reconnect before saving dates.")
		return m, nil
	}
	changes, err := datesChanges(*f, m.Calendar.Date)
	if err != nil {
		f.Err = err
		return m, nil
	}
	if len(changes) == 0 {
		m.DatesForm = DatesFormState{}
		return m, nil
	}
	f.Saving, f.Err = true, nil
	return m, []Cmd{SetDatesCmd{Workspace: m.Workspace, UID: f.UID, ProjectID: f.ProjectID, Actor: m.Actor, Changes: changes}}
}

// updateDates applies the answer to a save. A failure keeps the card open with kata's reason; changes written before it are
// taken as the new opened values, so saving again guards on what is there now, and the Issue is read again.
func updateDates(m Model, msg DatesResultMsg) (Model, []Cmd) {
	f := &m.DatesForm
	if !f.Open || f.UID != msg.UID {
		return m, nil
	}
	f.Saving = false
	for _, c := range msg.Applied {
		for _, spec := range DatesFields {
			if datesKey(spec.Name) == c.Key {
				*f.Held.at(spec.Name), *f.Prefill.at(spec.Name) = c.Value, c.Value
			}
		}
	}
	if msg.Err != nil {
		f.Err = msg.Err
		if len(msg.Applied) == 0 {
			return m, nil
		}
	} else {
		m.DatesForm = DatesFormState{}
	}
	return snapshotChanged(m.issueWritten(msg.UID))
}

func updateDatesFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	form := &m.DatesForm
	if form.Saving {
		return m, nil
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeDatesForm)
	switch {
	case bound && binding.Action == DatesSubmit:
		return submitDates(m)
	case bound && binding.Action == DatesCancel:
		m.DatesForm = DatesFormState{}
	default:
		text := form.Text.at(form.Field)
		var edited bool
		if *text, edited = editText(*text, &form.Caret, form.Field, binding, bound, msg); edited {
			form.Err = nil
		}
	}
	return m, nil
}
