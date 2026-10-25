# Recurring Issues

`O` opens the Recurrence card in a Pane. `tern-kata recur` does the same from a shell or an agent, without Tern, with the card's rules and messages:

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
| `--on mon,thu` | Weekdays |
| `--day 15\|last\|"2nd tue"` | Day of the month |
| `--month 1-12` | Month |
| `--until DATE` or `--count N` | When it ends |
| `--start DATE` | Start date; defaults to the Issue's scheduled date or today |
| `--zone <IANA>` | Time zone; defaults to this machine's IANA zone or UTC |
| `--rule 'RRULE'` | A raw RRULE instead of the frequency flags |
| `--stop` | Stop the recurrence; existing Occurrences stay |

## Behaviour

- Refs, `--as` and the Binding rule are those of `attach`, including the refusal of an empty `--as` (see [Attachments](attachments.md#from-a-shell-or-for-an-agent)).
- Success prints one line and a failure prints the reason on stderr and exits 1; a command line it refuses (a missing ref, an unknown flag) exits 2.
- Invalid input is refused with the card's message and nothing is sent.
- A schedule changed by someone else while the command ran is not overwritten but printed, and the command exits 1.
