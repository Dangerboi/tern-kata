package model

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The Job card is the Popup card that makes a Cron job and edits one. What a card offers depends on the job's kind and trigger,
// and both are tables: a kind (jobKinds) says which fields it shows, how it loads from a job and builds the rest of the
// definition, and a trigger (jobTriggers) says its own fields and how they build a CronTrigger. Adding an agent prompt,
// workflow or notify kind, or another trigger, is adding an entry to a table and its fields to JobFields.

const (
	ModeJobForm     Mode = "job-form"
	ModeJobConflict Mode = "job-conflict"
)

const (
	NewJobAction      Action = "new-job"
	NewIssueJobAction Action = "new-issue-job"
	EditJobAction     Action = "edit-job"
	PauseJobAction    Action = "pause-job"
	ArchiveJobAction  Action = "archive-job"
	RestoreJobAction  Action = "restore-job"

	JobNext           Action = "job-next-field"
	JobPrevious       Action = "job-previous-field"
	JobNextChoice     Action = "job-next-choice"
	JobPreviousChoice Action = "job-previous-choice"
	JobSave           Action = "job-save"
	JobCancel         Action = "job-cancel"
	JobOverwrite      Action = "job-overwrite"
	JobTakeNewer      Action = "job-take-newer"
	JobDiscard        Action = "job-discard"
)

// The Job card's fields.
const (
	JobFieldKind      Field = "kind"
	JobFieldName      Field = "name"
	JobFieldTrigger   Field = "trigger"
	JobFieldCron      Field = "cron"
	JobFieldZone      Field = "zone"
	JobFieldInterval  Field = "interval"
	JobFieldParent    Field = "parent"
	JobFieldTitle     Field = "title"
	JobFieldBody      Field = "body"
	JobFieldRecipient Field = "recipient"
	JobFieldMessage   Field = "message"
	JobFieldCatchup   Field = "catchup"
	JobFieldGrace     Field = "grace"
	JobFieldEnabled   Field = "enabled"
)

// JobFields is the Job card's field list in tab order: the fields of every kind and trigger. Which of them apply to the card as
// it stands is JobFormState.Shows; tab skips the others.
var JobFields = []FieldSpec{
	{JobFieldKind, false}, {JobFieldName, true}, {JobFieldTrigger, false},
	{JobFieldCron, true}, {JobFieldZone, true}, {JobFieldInterval, true},
	{JobFieldParent, true}, {JobFieldTitle, true}, {JobFieldBody, true}, {JobFieldRecipient, true}, {JobFieldMessage, true},
	{JobFieldCatchup, false}, {JobFieldGrace, true}, {JobFieldEnabled, false},
}

// jobFieldTitles are the prompts of the fields. The part before the first " (" is the field's name in a message.
var jobFieldTitles = map[Field]string{
	JobFieldKind:      "Kind",
	JobFieldName:      "Name",
	JobFieldTrigger:   "Trigger",
	JobFieldCron:      "Cron expression (5 fields, or @daily)",
	JobFieldZone:      "Time zone (IANA name, blank is UTC)",
	JobFieldInterval:  "Interval (seconds, or 15m, 2h; at least 60 s)",
	JobFieldParent:    "Parent Issue (short id)",
	JobFieldTitle:     "Child title",
	JobFieldBody:      "Child body (one line)",
	JobFieldRecipient: "Inbox recipient (blank: current owner or author)",
	JobFieldMessage:   "Inbox message",
	JobFieldCatchup:   "Catch up",
	JobFieldGrace:     "Grace (seconds)",
	JobFieldEnabled:   "Enabled",
}

// JobPrompt is the prompt of the Job card's field called name.
func JobPrompt(name Field) string { return jobFieldTitles[name] }

func jobFieldName(name Field) string {
	title, _, _ := strings.Cut(jobFieldTitles[name], " (")
	return title
}

// jobFieldError is a refusal that names the field: the card puts focus there.
type jobFieldError struct {
	field Field
	text  string
}

func (e jobFieldError) Error() string { return e.text }

func fieldError(field Field, format string, args ...any) error {
	return jobFieldError{field, jobFieldName(field) + ": " + fmt.Sprintf(format, args...)}
}

// Catchup policies, as kata names them.
const (
	CatchupSkip   = "skip"
	CatchupLatest = "latest"
	CatchupAll    = "all"
)

// jobCatchups are the choices of the Catch up field in the order the card shows them.
var jobCatchups = []struct{ id, label string }{
	{CatchupLatest, "Latest missed run"}, {CatchupAll, "Every missed run"}, {CatchupSkip, "Skip runs older than the grace"},
}

// jobKind is one kind of job the card can make. The card builds a job of the kind from its fields and reads one back into them.
type jobKind struct {
	id, label string
	// triggers lists the trigger ids the kind offers, in choice order; the first is the default.
	triggers []string
	// owns says the card can edit the job as this kind.
	owns func(CronJob) bool
	// shown says the field beyond the common ones (kind, name, trigger, enabled) and the trigger's own applies to the form.
	shown func(f JobFormState, name Field) bool
	// load reads the kind's fields from a job it owns.
	load func(f *JobFormState, job CronJob)
	// checkTrigger is the kind's limits on a trigger the form built.
	checkTrigger func(t CronTrigger) error
	// build is the kind's part of the definition (action, Issue policy, overlap, catchup, grace) and, for a new job, the reference
	// of its parent. Name, trigger and enabled are the card's.
	build func(f JobFormState) (def CronDefinition, parentRef string, err error)
}

// jobTrigger is one kind of trigger the card can make, with the fields it adds.
type jobTrigger struct {
	id, label string
	fields    []Field
	load      func(f *JobFormState, t CronTrigger)
	build     func(f JobFormState) (CronTrigger, error)
}

var jobKinds = []jobKind{nativeJobKind}

var jobTriggers = []jobTrigger{cronJobTrigger, intervalJobTrigger}

// nativeMinInterval is the shortest interval kata's daemon delivers a native job at.
const nativeMinInterval = 60

// nativeChildTitleMax is how long a native job's child title may be: kata appends " — " and a 30-character stamp to it and
// bounds the whole at 1024 characters.
const nativeChildTitleMax = 991

var nativeJobKind = jobKind{
	id: "native", label: "Recurring child",
	triggers: []string{TriggerCron, TriggerInterval},
	owns:     CronJob.Native,
	shown: func(f JobFormState, name Field) bool {
		switch name {
		case JobFieldParent:
			return !f.Change
		case JobFieldGrace:
			return f.Catchup == CatchupSkip
		}
		return slices.Contains([]Field{JobFieldTitle, JobFieldBody, JobFieldRecipient, JobFieldMessage, JobFieldCatchup}, name)
	},
	load: func(f *JobFormState, job CronJob) {
		d := job.Definition
		if job.Parent != nil && job.Parent.ShortID != "" {
			f.Parent = job.Parent.ShortID
		} else {
			f.Parent = d.Issue.ParentUID
		}
		f.Title, f.Body = d.Issue.Title, strings.Join(strings.Fields(d.Issue.Body), " ")
		if d.Action.Recipient != recipientDefault {
			f.Recipient = d.Action.Recipient
		}
		f.Message, f.Catchup = d.Action.Message, d.Catchup
		if d.GraceSeconds > 0 {
			f.Grace = strconv.FormatInt(d.GraceSeconds, 10)
		}
	},
	checkTrigger: func(t CronTrigger) error {
		if t.Kind == TriggerInterval && t.IntervalSeconds < nativeMinInterval {
			return fieldError(JobFieldInterval, "must be at least %d seconds.", nativeMinInterval)
		}
		return nil
	},
	build: func(f JobFormState) (CronDefinition, string, error) {
		def := CronDefinition{Overlap: "allow", Catchup: f.Catchup, Action: CronAction{Kind: ActionNotify, Recipient: recipientDefault}}
		policy := CronIssuePolicy{Kind: IssuePerRun}
		parentRef := ""
		if f.Change {
			policy.ParentUID = f.Opened.Definition.Issue.ParentUID
		} else if parentRef = strings.TrimPrefix(strings.TrimSpace(f.Parent), "#"); parentRef == "" {
			return def, "", fieldError(JobFieldParent, "name the Issue the children hang from, by its short id.")
		}
		if policy.Title = strings.TrimSpace(f.Title); policy.Title == "" {
			return def, "", fieldError(JobFieldTitle, "must not be blank.")
		} else if utf8.RuneCountInString(policy.Title) > nativeChildTitleMax {
			return def, "", fieldError(JobFieldTitle, "at most %d characters.", nativeChildTitleMax)
		}
		policy.Body = strings.TrimSpace(f.Body)
		if recipient := strings.TrimSpace(f.Recipient); recipient != "" {
			if !exactRecipient(recipient) {
				return def, "", fieldError(JobFieldRecipient, "use an exact address like alice or alice/reviewer, or leave it blank for the current owner or author.")
			}
			def.Action.Recipient = recipient
		}
		if len(f.Message) > 1024 {
			return def, "", fieldError(JobFieldMessage, "at most 1024 bytes.")
		}
		def.Action.Message = strings.TrimSpace(f.Message)
		if f.Catchup == CatchupSkip {
			grace, err := strconv.ParseInt(strings.TrimSpace(f.Grace), 10, 64)
			if err != nil || grace <= 0 {
				return def, "", fieldError(JobFieldGrace, "skip needs a whole number of seconds, 1 or more.")
			}
			def.GraceSeconds = grace
		}
		def.Issue = &policy
		return def, parentRef, nil
	},
}

// recipientDefault is the inbox recipient that reads the parent's owner at delivery, else its author.
const recipientDefault = "current-owner-or-author"

// exactRecipient is kata's rule for an inbox address: at most 128 bytes, an actor or actor/teammate with no empty part, space
// or control character.
func exactRecipient(s string) bool {
	if s == "" || len(s) > 128 || strings.Count(s, "/") > 1 || !utf8.ValidString(s) {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == "" {
			return false
		}
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
}

var cronJobTrigger = jobTrigger{
	id: TriggerCron, label: "Cron expression", fields: []Field{JobFieldCron, JobFieldZone},
	load: func(f *JobFormState, t CronTrigger) { f.Zone, f.Cron = t.zoneAndExpression() },
	build: func(f JobFormState) (CronTrigger, error) {
		expression, zone := strings.TrimSpace(f.Cron), strings.TrimSpace(f.Zone)
		switch {
		case expression == "":
			return CronTrigger{}, fieldError(JobFieldCron, "enter 5 fields like 0 9 * * 1-5, or a descriptor like @daily.")
		case strings.HasPrefix(expression, "TZ=") || strings.HasPrefix(expression, "CRON_TZ="):
			return CronTrigger{}, fieldError(JobFieldCron, "put the zone in the Time zone field, not in a TZ= prefix.")
		case strings.HasPrefix(expression, "@every"):
			return CronTrigger{}, fieldError(JobFieldCron, "use the Interval trigger instead of @every.")
		}
		if zone == "" {
			zone = "UTC"
		}
		if _, err := time.LoadLocation(zone); err != nil || zone == "Local" {
			return CronTrigger{}, fieldError(JobFieldZone, "unknown time zone %q; use an IANA name like Europe/Berlin or UTC.", zone)
		}
		t := CronTrigger{Kind: TriggerCron, Cron: expression, Timezone: zone}
		if _, ok := t.cronSchedule(); !ok {
			return CronTrigger{}, fieldError(JobFieldCron, "%q is not a valid expression; use 5 fields like 0 9 * * 1-5, or a descriptor like @daily.", expression)
		}
		return t, nil
	},
}

var intervalJobTrigger = jobTrigger{
	id: TriggerInterval, label: "Interval", fields: []Field{JobFieldInterval},
	load: func(f *JobFormState, t CronTrigger) { f.Interval = strconv.FormatInt(t.IntervalSeconds, 10) },
	build: func(f JobFormState) (CronTrigger, error) {
		text := strings.TrimSpace(f.Interval)
		seconds, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			d, durationErr := time.ParseDuration(text)
			if durationErr != nil || d%time.Second != 0 {
				return CronTrigger{}, fieldError(JobFieldInterval, "enter a whole number of seconds, or a duration like 15m or 2h.")
			}
			seconds = int64(d / time.Second)
		}
		if seconds <= 0 {
			return CronTrigger{}, fieldError(JobFieldInterval, "must be positive.")
		}
		return CronTrigger{Kind: TriggerInterval, IntervalSeconds: seconds}, nil
	},
}

// JobFormState is the Job card. The text fields hold what was typed and the card reads them into a definition only when it
// saves or previews, so every message comes from one place.
type JobFormState struct {
	Open, Saving bool
	Field        Field
	Caret        Caret
	ProjectID    int64
	// UID is the job's: for a new one the card minted it, so every send of the create is the same create.
	UID string
	// Change says the card edits Opened, the job as it was read, instead of making a new one. Its definition_event_uid is the
	// guard of the save.
	Change bool
	Opened CronJob
	// Conflict is the newer job a save found: nothing was written, the draft stays, and the card asks what to do.
	Conflict *CronJob
	// Kind and Trigger are the ids of the kind and the trigger of the job.
	Kind, Trigger                                               string
	Name, Cron, Zone, Interval, Parent, Title, Body, Recipient  string
	Message, Grace                                              string
	Catchup                                                     string
	Enabled                                                     bool
	Err                                                         error
}

func (f JobFormState) kind() jobKind {
	for _, k := range jobKinds {
		if k.id == f.Kind {
			return k
		}
	}
	return jobKinds[0]
}

func (f JobFormState) trigger() jobTrigger {
	for _, t := range jobTriggers {
		if t.id == f.Trigger {
			return t
		}
	}
	return jobTriggers[0]
}

// InTrigger says the field is one of the current trigger's own: the schedule preview comes after them.
func (f JobFormState) InTrigger(name Field) bool { return slices.Contains(f.trigger().fields, name) }

// Shows says the field applies to the card as it stands; the others are hidden and tab skips them.
func (f JobFormState) Shows(name Field) bool {
	switch name {
	case JobFieldKind, JobFieldName, JobFieldTrigger, JobFieldEnabled:
		return true
	}
	return f.kind().shown(f, name) || slices.Contains(f.trigger().fields, name)
}

// text is the single-line field called name, nil for a choice field.
func (f *JobFormState) text(name Field) *string {
	switch name {
	case JobFieldName:
		return &f.Name
	case JobFieldCron:
		return &f.Cron
	case JobFieldZone:
		return &f.Zone
	case JobFieldInterval:
		return &f.Interval
	case JobFieldParent:
		return &f.Parent
	case JobFieldTitle:
		return &f.Title
	case JobFieldBody:
		return &f.Body
	case JobFieldRecipient:
		return &f.Recipient
	case JobFieldMessage:
		return &f.Message
	case JobFieldGrace:
		return &f.Grace
	}
	return nil
}

// Text is what the card holds in the text field called name.
func (f JobFormState) Text(name Field) string {
	if p := f.text(name); p != nil {
		return *p
	}
	return ""
}

// JobChoice is one choice of a choice field.
type JobChoice struct {
	Label  string
	Chosen bool
}

// JobChoices are the choices of the choice field called name, in the order the card shows and picks them by index.
func (f JobFormState) JobChoices(name Field) []JobChoice {
	var choices []JobChoice
	switch name {
	case JobFieldKind:
		for _, k := range jobKinds {
			choices = append(choices, JobChoice{k.label, k.id == f.Kind})
		}
	case JobFieldTrigger:
		for _, id := range f.kind().triggers {
			choices = append(choices, JobChoice{jobTriggerOf(id).label, id == f.Trigger})
		}
	case JobFieldCatchup:
		for _, c := range jobCatchups {
			choices = append(choices, JobChoice{c.label, c.id == f.Catchup})
		}
	case JobFieldEnabled:
		choices = []JobChoice{{"Yes", f.Enabled}, {"No", !f.Enabled}}
	}
	return choices
}

func jobTriggerOf(id string) jobTrigger {
	for _, t := range jobTriggers {
		if t.id == id {
			return t
		}
	}
	return jobTriggers[0]
}

// choose gives the focused choice field its choice at index i; an index out of range changes nothing.
func (f *JobFormState) choose(i int) {
	choices := f.JobChoices(f.Field)
	if i < 0 || i >= len(choices) {
		return
	}
	switch f.Field {
	case JobFieldKind:
		f.Kind = jobKinds[i].id
		if !slices.Contains(f.kind().triggers, f.Trigger) {
			f.Trigger = f.kind().triggers[0]
		}
	case JobFieldTrigger:
		f.Trigger = f.kind().triggers[i]
	case JobFieldCatchup:
		f.Catchup = jobCatchups[i].id
	case JobFieldEnabled:
		f.Enabled = i == 0
	}
	f.Err = nil
}

// step moves the focused choice field one choice on; back goes the other way.
func (f *JobFormState) step(back bool) {
	choices := f.JobChoices(f.Field)
	if len(choices) == 0 {
		return
	}
	d := 1
	if back {
		d = -1
	}
	at := slices.IndexFunc(choices, func(c JobChoice) bool { return c.Chosen })
	f.choose(((at+d)%len(choices) + len(choices)) % len(choices))
}

// newJobForm is the card for a job that does not exist yet: a native job running every day at nine in the Pane's zone,
// catching up with the latest missed run. parent is the reference of its parent Issue, blank when the user names it.
func newJobForm(m Model, projectID int64, parent string) Model {
	m, uid := m.jobUID()
	zone := m.Calendar.Zone()
	if zone == "Local" || zone == "" {
		zone = "UTC"
	}
	k := jobKinds[0]
	m.JobForm = JobFormState{Open: true, Field: JobFieldName, ProjectID: projectID, UID: uid, Kind: k.id, Trigger: k.triggers[0],
		Cron: "0 9 * * *", Zone: zone, Interval: "3600", Parent: parent, Catchup: CatchupLatest, Enabled: true}
	return m
}

// changeJobForm is the card for job, prefilled from it; false when no kind of the card can edit it.
func changeJobForm(m Model, projectID int64, job CronJob) (JobFormState, bool) {
	for _, k := range jobKinds {
		if !k.owns(job) {
			continue
		}
		f := JobFormState{Open: true, Field: JobFieldName, ProjectID: projectID, UID: job.UID, Change: true, Opened: job, Kind: k.id,
			Trigger: job.Definition.Trigger.Kind, Name: job.Name, Cron: "0 9 * * *", Zone: "UTC", Interval: "3600", Enabled: job.Definition.Enabled}
		if !slices.Contains(k.triggers, f.Trigger) {
			return JobFormState{}, false
		}
		jobTriggerOf(f.Trigger).load(&f, job.Definition.Trigger)
		k.load(&f, job)
		return f, true
	}
	return JobFormState{}, false
}

// openNewJob is `n` on the Jobs Page: a card for a new job, whose parent the user names.
func openNewJob(m Model) (Model, []Cmd) {
	switch {
	case !m.OnJobs() || m.Jobs.ProjectID == 0:
		return m, nil
	case m.Stale:
		return refuse(m, "Daemon disconnected; reconnect before making a job.")
	case m.Jobs.ProjectArchived:
		return refuse(m, "This Project is archived; it can't get a job.")
	}
	return newJobForm(m, m.Jobs.ProjectID, ""), nil
}

// openIssueJob is `J` on an Issue: a card for a new native job with the Issue as its parent.
func openIssueJob(m Model) (Model, []Cmd) {
	issue := closeIssue(m)
	switch {
	case m.Stale:
		return refuse(m, "Daemon disconnected; reconnect before making a job.")
	case issue == nil:
		return m, nil
	}
	ref := cmpOr(issue.ShortID, issue.UID)
	return newJobForm(m, issue.ProjectID, ref), nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// openEditJob is `e` on the Jobs Page: the card on the selected job, prefilled.
func openEditJob(m Model) (Model, []Cmd) {
	m, job, ok := writableJob(m)
	if !ok {
		return m, nil
	}
	if job.Archived() {
		return refuse(m, "The job is archived; restore it before editing.")
	}
	f, editable := changeJobForm(m, m.Jobs.ProjectID, job)
	if !editable {
		return refuse(m, fmt.Sprintf("The Job card edits recurring-child jobs; this one is a %s job.", job.Kind()))
	}
	m.JobForm = f
	return m, nil
}

// buildTrigger reads the card's trigger and checks it against the kind's limits.
func (f JobFormState) buildTrigger() (CronTrigger, error) {
	t, err := f.trigger().build(f)
	if err != nil {
		return CronTrigger{}, err
	}
	return t, f.kind().checkTrigger(t)
}

// JobTerms is what the card sends once it reads clean.
type JobTerms struct {
	Name       string
	Definition CronDefinition
	ParentRef  string
}

// terms reads the card into the terms it would send, or says what is wrong with it, naming the field. Nothing leaves the card
// until it reads clean.
func (f JobFormState) terms() (JobTerms, error) {
	name := strings.TrimSpace(f.Name)
	switch {
	case name == "":
		return JobTerms{}, fieldError(JobFieldName, "must not be blank.")
	case len(name) > 256:
		return JobTerms{}, fieldError(JobFieldName, "at most 256 bytes.")
	}
	trigger, err := f.buildTrigger()
	if err != nil {
		return JobTerms{}, err
	}
	def, parentRef, err := f.kind().build(f)
	if err != nil {
		return JobTerms{}, err
	}
	def.Enabled, def.Trigger = f.Enabled, trigger
	return JobTerms{Name: name, Definition: def, ParentRef: parentRef}, nil
}

// JobPreview is the schedule under the trigger: in words and the next runs, or why neither can be told yet.
type JobPreview struct {
	Words string
	Runs  []string
	Err   error
}

// previewRuns is how many next runs the preview lists.
const previewRuns = 3

// Preview reads the trigger as it stands. A job's interval counts from its anchor: the creation instant, which for a job that
// does not exist yet is as good as now.
func (f JobFormState) Preview(cal PaneCalendar, now time.Time) JobPreview {
	t, err := f.buildTrigger()
	if err != nil {
		return JobPreview{Err: err}
	}
	anchor := now
	if p := f.Opened.Parent; f.Change && p != nil && p.Progress != nil {
		anchor = p.Progress.Anchor
	}
	const layout = "Mon 2006-01-02 15:04 MST"
	preview := JobPreview{Words: t.Words()}
	switch t.Kind {
	case TriggerCron:
		schedule, _ := t.cronSchedule()
		zone, _ := t.zoneAndExpression()
		loc, _ := time.LoadLocation(zone)
		for next := schedule.Next(now); len(preview.Runs) < previewRuns && !next.IsZero(); next = schedule.Next(next) {
			preview.Runs = append(preview.Runs, next.In(loc).Format(layout))
		}
	case TriggerInterval:
		step := time.Duration(t.IntervalSeconds) * time.Second
		next := anchor.Add((now.Sub(anchor)/step + 1) * step)
		for range previewRuns {
			preview.Runs = append(preview.Runs, cal.in(next).Format(layout))
			next = next.Add(step)
		}
	}
	return preview
}

var errJobDaemonGone = errors.New("Daemon disconnected; reconnect before saving this job.")

// submitJob is the card's save: the terms that read clean go out as one write, and anything else stays in the card.
func submitJob(m Model) (Model, []Cmd) {
	f := &m.JobForm
	if !f.Open || f.Saving || f.Conflict != nil {
		return m, nil
	}
	if m.Stale {
		f.Err = errJobDaemonGone
		return m, nil
	}
	terms, err := f.terms()
	if err != nil {
		f.Err = err
		if bad, ok := errors.AsType[jobFieldError](err); ok && f.Shows(bad.field) {
			f.Field = bad.field
		}
		return m, nil
	}
	f.Saving, f.Err = true, nil
	cmd := SaveJobCmd{Workspace: m.Workspace, ProjectID: f.ProjectID, Actor: m.Actor, UID: f.UID, Name: terms.Name, Definition: terms.Definition, ParentRef: terms.ParentRef}
	if f.Change {
		cmd.ExpectedEventUID = f.Opened.DefinitionEventUID
	}
	return m, []Cmd{cmd}
}

// jobFormKey is one key for the Job card on top. Left, right and space move through a choice field's choices, and in a text
// field space is typed.
func jobFormKey(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.JobForm
	if f.Saving {
		return m, nil
	}
	if f.Conflict != nil {
		return answerJobConflict(m, msg)
	}
	binding, bound := LookupBindingForMode(msg.Chord, ModeJobForm)
	field := f.text(f.Field)
	switch {
	case bound && binding.Action == JobCancel:
		f.Open = false
	case bound && binding.Action == JobNext:
		cycleField(&m, 1)
	case bound && binding.Action == JobPrevious:
		cycleField(&m, -1)
	case bound && binding.Action == JobSave:
		return submitJob(m)
	case field == nil && bound && (binding.Action == InputSpace || binding.Action == JobNextChoice):
		f.step(false)
	case field == nil && bound && binding.Action == JobPreviousChoice:
		f.step(true)
	case field != nil:
		var edited bool
		if bound && binding.Action == InputSpace {
			bound = false // a space in a text field is typed
		}
		if *field, edited = editText(*field, &f.Caret, f.Field, binding, bound, msg); edited {
			f.Err = nil
		}
	}
	return m, nil
}

// answerJobConflict is one key while the card shows the newer job: overwrite saves the draft over it, take newer replaces the
// draft with it, discard closes the card, and any other key is swallowed.
func answerJobConflict(m Model, msg KeyMsg) (Model, []Cmd) {
	f := &m.JobForm
	binding, bound := LookupBindingForMode(msg.Chord, ModeJobConflict)
	if !bound {
		return m, nil
	}
	switch binding.Action {
	case JobOverwrite:
		f.Opened, f.Change, f.Conflict = *f.Conflict, true, nil
		return submitJob(m)
	case JobTakeNewer:
		if taken, ok := changeJobForm(m, f.ProjectID, *f.Conflict); ok {
			*f = taken
		} else {
			f.Conflict, f.Err = nil, errors.New("The Job card can't edit the newer job.")
		}
	case JobDiscard:
		f.Open = false
	}
	return m, nil
}

// JobConflictFacts are the lines that show the newer job the card found.
func (m Model) JobConflictFacts() []JobFact {
	job := m.JobForm.Conflict
	if job == nil {
		return nil
	}
	var facts []JobFact
	add := func(label, value string) {
		if value != "" {
			facts = append(facts, JobFact{Label: label, Value: value})
		}
	}
	d := job.Definition
	add("Name", job.Name)
	add("State", jobState(m, *job))
	add("Schedule", d.Trigger.Words())
	if d.Issue != nil {
		add("Child title", d.Issue.Title)
	}
	catchup := d.Catchup
	if d.GraceSeconds > 0 {
		catchup += fmt.Sprintf(", grace %d s", d.GraceSeconds)
	}
	add("Catchup", catchup)
	add("Updated", m.Calendar.Stamp(job.UpdatedAt))
	return facts
}
