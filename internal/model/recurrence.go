package model

// Recurrence is the kata recurrence an Issue belongs to, as the Issue detail shows it. The gateway translates it from the
// Collection snapshot. A Recurrence with Stopped set has only that: kata no longer lists a deleted recurrence, so its other
// fields are unknown.
type Recurrence struct {
	UID      string
	Rule     string // kata's rule text; Schedule reads it
	Start    string // YYYY-MM-DD
	TimeZone string
	Next     string // YYYY-MM-DD of the next Occurrence; empty once the series is exhausted
	Revision int64
	Stopped  bool
}

// Schedule is the rule as the form shape, or raw.
func (r Recurrence) Schedule() Schedule { return ParseSchedule(r.Rule) }

// Text is what the Issue detail's Recurring property reads: the schedule in words and the next Occurrence's date, the rule
// itself when it is raw, or "stopped".
func (r Recurrence) Text() string {
	s := r.Schedule()
	switch {
	case r.Stopped:
		return "stopped"
	case s.IsRaw() || r.Next == "":
		return s.Words()
	}
	return s.Words() + " · next " + r.Next
}

// CreateRecurrenceCmd makes the Issue the first Occurrence of a new recurrence; the Daemon gateway builds the template from the
// Issue as it stands.
type CreateRecurrenceCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	Rule, Start, TimeZone string
}

func (CreateRecurrenceCmd) cmd() {}

// ChangeRecurrenceCmd is a Guarded write of the schedule: the Daemon gateway sends the rule, start date and time zone at the
// recurrence's current revision, and writes nothing when these differ from the Opened ones the card read. The template is
// never sent.
type ChangeRecurrenceCmd struct {
	Workspace, UID, Actor               string
	ProjectID                           int64
	RecurrenceUID                       string
	Rule, Start, TimeZone               string
	OpenedRule, OpenedStart, OpenedZone string
}

func (ChangeRecurrenceCmd) cmd() {}

// StopRecurrenceCmd deletes the recurrence at its current revision; the Occurrences stay.
type StopRecurrenceCmd struct {
	Workspace, UID, Actor string
	ProjectID             int64
	RecurrenceUID         string
}

func (StopRecurrenceCmd) cmd() {}

// RecurrenceResultMsg is the result of a recurrence write for the Issue UID; Err is nil when it landed. Changed is the
// current recurrence when a change found the schedule moved meanwhile: nothing was written and Err is nil.
type RecurrenceResultMsg struct {
	UID     string
	Err     error
	Changed *Recurrence
}

func (RecurrenceResultMsg) msg() {}
