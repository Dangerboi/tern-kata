package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// recur runs `tern-kata recur <the fixture's Issue> args...` as main does: planned from the command line, run through
// runSubcommand.
func (f subcommandFixture) recur(t *testing.T, args ...string) (string, error) {
	t.Helper()
	plan, err := PlanLaunch(append([]string{"recur", f.uid, "--as", "tester"}, args...), func(string) (string, bool) { return "", false }, "test")
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = runSubcommand(context.Background(), plan, data.NewGateway(f.locate).WithHost(testHost), &out)
	return out.String(), err
}

func TestAnIssueIsMadeToRecurPrintedChangedAndStopped(t *testing.T) {
	f := newSubcommandFixture(t)
	if out, err := f.recur(t); err != nil || !strings.Contains(out, "does not recur") {
		t.Fatalf("stdout %q, err %v; want it to say the Issue does not recur", out, err)
	}
	out, err := f.recur(t, "--frequency", "weekly", "--on", "mon,thu", "--start", "2026-11-02", "--zone", "Europe/Oslo")
	if err != nil || !strings.Contains(out, "recurs: Weekly on Mon, Thu") || !strings.Contains(out, "start 2026-11-02, Europe/Oslo") {
		t.Fatalf("stdout %q, err %v; want the schedule in words with its start and zone", out, err)
	}
	printed, err := f.recur(t)
	if err != nil || printed != out {
		t.Fatalf("print gave %q, err %v; want what making it recur printed, %q", printed, err, out)
	}
	if out, err = f.recur(t, "--frequency=monthly", "--day", "2nd tue", "--count", "3"); err != nil || !strings.Contains(out, "2nd Tuesday") || !strings.Contains(out, "3 times") {
		t.Fatalf("stdout %q, err %v; want the changed schedule", out, err)
	}
	if out, err = f.recur(t, "--rule", "FREQ=DAILY;INTERVAL=3"); err != nil || !strings.Contains(out, "Every 3 days") {
		t.Fatalf("stdout %q, err %v; want the raw rule's schedule", out, err)
	}
	if out, err = f.recur(t, "--stop"); err != nil || !strings.Contains(out, "no longer recurs") {
		t.Fatalf("stdout %q, err %v", out, err)
	}
	if out, err = f.recur(t); err != nil || !strings.Contains(out, "was stopped") {
		t.Fatalf("stdout %q, err %v; want it to say the recurrence was stopped", out, err)
	}
}

func TestInvalidScheduleFlagsAreRefusedWithTheCardsMessagesAndNothingIsSent(t *testing.T) {
	f := newSubcommandFixture(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--frequency", "daily", "--every", "0"}, "Every must be a whole number, 1 or more."},
		{[]string{"--frequency", "weekly", "--on", ""}, "Pick at least one weekday."},
		{[]string{"--frequency", "monthly", "--day", "32"}, "Day must be between 1 and 31."},
		{[]string{"--frequency", "daily", "--count", "0"}, "Occurrences must be a whole number, 1 or more."},
		{[]string{"--frequency", "daily", "--start", "2026-11-02", "--until", "2026-11-01"}, "End date must not be before the start date."},
		{[]string{"--frequency", "daily", "--zone", "Mars/Base"}, `Unknown time zone "Mars/Base".`},
		{[]string{"--frequency", "daily", "--until", "2027-01-01", "--count", "2"}, "Give --until or --count, not both."},
		{[]string{"--every", "2"}, "Give --frequency"},
		{[]string{"--frequency", "daily", "--on", "mon"}, "--on applies to weekly schedules only."},
	} {
		out, err := f.recur(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) || out != "" {
			t.Errorf("recur %v: stdout %q, err %v; want the refusal %q", tc.args, out, err, tc.want)
		}
	}
	if out, _ := f.recur(t); !strings.Contains(out, "does not recur") {
		t.Fatalf("a refused schedule was sent: %q", out)
	}
}

func TestStoppingAnIssueThatDoesNotRecurOrWasStoppedIsRefused(t *testing.T) {
	f := newSubcommandFixture(t)
	if out, err := f.recur(t, "--stop"); err == nil || !strings.Contains(err.Error(), "does not recur; nothing was written") || out != "" {
		t.Fatalf("stdout %q, err %v", out, err)
	}
	if _, err := f.recur(t, "--frequency", "daily"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.recur(t, "--stop"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--stop"}, {"--frequency", "weekly"}} {
		if out, err := f.recur(t, args...); err == nil || !strings.Contains(err.Error(), "was stopped; nothing was written") || out != "" {
			t.Fatalf("recur %v: stdout %q, err %v", args, out, err)
		}
	}
}

func TestAScheduleChangedMeanwhileIsNotOverwrittenAndIsPrinted(t *testing.T) {
	f := newSubcommandFixture(t)
	if _, err := f.recur(t, "--frequency", "daily", "--start", "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanLaunch([]string{"recur", f.uid, "--frequency", "weekly", "--on", "fri"}, func(string) (string, bool) { return "tester", true }, "test")
	if err != nil {
		t.Fatal(err)
	}
	gateway := data.NewGateway(f.locate).WithHost(testHost)
	defer gateway.Close()
	results := make(chan model.Msg, 1)
	send := func(cmd model.Cmd) model.Msg {
		if change, is := cmd.(model.ChangeRecurrenceCmd); is {
			// someone else changes the schedule between the read and the write
			other := change
			other.Opened = change.Opened
			other.RecurrenceTerms = model.RecurrenceTerms{Rule: "FREQ=DAILY;INTERVAL=5", Start: "2026-11-02", TimeZone: "UTC"}
			gateway.Run(context.Background(), other, results)
			if moved := (<-results).(model.RecurrenceResultMsg); moved.Err != nil {
				t.Fatal(moved.Err)
			}
		}
		gateway.Run(context.Background(), cmd, results)
		return <-results
	}
	var out bytes.Buffer
	err = runRecur(plan, f.repo, send, &out)
	if err == nil || !strings.Contains(err.Error(), "changed meanwhile; nothing was written") || !strings.Contains(out.String(), "Every 5 days") {
		t.Fatalf("stdout %q, err %v; want the other schedule printed and the change refused", out.String(), err)
	}
	if now, _ := f.recur(t); !strings.Contains(now, "Every 5 days") {
		t.Fatalf("the schedule is %q; the change overwrote the other one", now)
	}
}
