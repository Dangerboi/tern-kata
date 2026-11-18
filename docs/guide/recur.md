# Recurring Issues

`O` opens the Recurrence card in a Pane. `tern-kata recur` does the same from a shell or an agent, without Tern, with the card's rules and messages:

The card's head reads **Make recurring** or **Change recurrence**, with the Issue's ref as a muted line, and its primary button is **Save** in both modes. Each field has its label above it: Frequency, Every (the unit `weeks`, `days`, `months` or `years` follows the number), Weekdays, Day of month (the hint `1-31, last, 2nd tue, last fri` shows while it has focus), Month, Ends, Occurrences or End date, Start date and Time zone; dates are typed as `YYYY-MM-DD`. When changing a rule, a muted summary under the head reads for example `Weekly on Mon, Thu · next 2026-11-05`. A rule set outside Tern shows the warning "Set outside Tern. Saving replaces it." with the rule in a muted line. Stopping asks "Stop this Issue recurring? Existing occurrences stay." with **Stop recurring** and **Keep editing**.

```sh
tern-kata recur <ref>                                         # the schedule in words and the next Occurrence's date
tern-kata recur <ref> --frequency weekly --on mon,thu         # make it recur, or change its schedule
tern-kata recur <ref> --frequency monthly --day "2nd tue" --count 6 --zone Europe/Oslo
tern-kata recur <ref> --rule 'FREQ=WEEKLY;BYDAY=MO'           # a raw RRULE instead of the frequency flags
tern-kata recur <ref> --stop                                  # existing Occurrences stay
```

## Flags

| Flag | Meaning |
|---|---|
| `--frequency daily\|weekly\|monthly\|yearly` | How often it recurs |
| `--every N` | Every N periods |
| `--on mon,thu` | Weekdays; an Issue that already recurs keeps its weekdays when you don't give them |
| `--day 15\|last\|"2nd tue"` | Day of the month; an Issue that already recurs keeps its day when you don't give one |
| `--month 1-12` | Month; an Issue that already recurs keeps its month when you don't give one |
| `--until DATE` or `--count N` | When it ends |
| `--start DATE` | Start date; defaults to the Issue's scheduled date (its day, when the date has a time) or today; an Issue that already recurs keeps its stored start |
| `--zone <IANA>` | Time zone; defaults to this machine's IANA zone or UTC; an Issue that already recurs keeps its stored zone |
| `--rule 'RRULE'` | A raw RRULE instead of the frequency flags |
| `--stop` | Stop the recurrence; existing Occurrences stay |

A value that starts with `--` must be written `--flag=value`; written apart, it reads as the next flag and the command says the flag requires a value.

## Behaviour

- `<ref>` is anything `kata show` takes: a short id in this directory's Project, `<project>#<short id>` or a ULID. `--as <actor>` names the actor; without it the actor is `$KATA_AUTHOR`, `$USER`, then `git config user.name`. An explicitly empty value (`--as=` or `--as ""`) is refused with "--as must be non-empty" and nothing is written, so a script that expands an unset variable into `--as` never acts as someone else. The Issue's Project must be the one this directory is bound to.
- Success prints one line and a failure prints the reason on stderr and exits 1; a command line it refuses (a missing ref, an unknown flag) exits 2.
- Invalid input is refused with the card's message and nothing is sent.
- A schedule changed by someone else while the command ran is not overwritten but printed, and the command exits 1.
