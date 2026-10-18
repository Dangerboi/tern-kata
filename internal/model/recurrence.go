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
