---
name: tern-kata
description: Files attached to kata Issues, recurring Issues, and the tern-kata worker, through tern-kata. Use when an Issue's metadata has `attachment.*` keys, to attach a screenshot, log or other file to an Issue, to make an Issue recur, change or stop its schedule, or print it (`tern-kata recur`), to run a scheduled agent job now (`tern-kata run`), or to check what the worker serves and runs (`tern-kata worker status`, `worker doctor`), or to resume or abandon a held worker run (`tern-kata worker resume`, `worker abandon`), or to stop the worker (`tern-kata worker stop`).
---

An Attachment is a file copied into the Project's Attachment store, `<primary checkout>/.kata-attachments/<Issue ULID>/<name>`, and pointed to by one `attachment.<name>` meta key on the Issue. The store is gitignored and never committed.

## Read

Reading needs kata and your own file tools, plus SSH for a file on another host.

- `kata show <ref> --json` holds each Attachment at `.issue.metadata["attachment.<name>"]` as a `{"host", "path"}` object; `kata show --agent` and kata's MCP show tool carry the same key. `<name>` is the stored file name.
- When `host` matches `hostname` (any case), read `path` directly with your file or image tools. File searches skip the gitignored store: take the path from the pointer.
- Another host means the file is on that machine, and `host` is its OS host name. A `Host <host>` entry in `~/.ssh/config` can supply the `HostName` and `User` it needs. When `ssh -o BatchMode=yes -o ConnectTimeout=5 <host> exit` succeeds (`exit`, since a Windows host runs PowerShell or cmd, where `true` fails), copy the file to a temporary file outside the repo with `scp -o BatchMode=yes '<host>:<path>' <temp file>` and read the copy. Write a Windows `path` with `/` for each `\` (`C:/Users/...`); scp refuses the backslashes. Only read on that host; the file there stays as it is. When SSH fails (no key, no user, or a host key not yet trusted), tell the user the file is on `host` and that a `Host <host>` entry with its `User`, plus one connection to trust its key, would let you read it; then work without it.

## Attach and detach

These need the `tern-kata` binary installed and on `PATH`: check with `command -v tern-kata` (PowerShell: `Get-Command tern-kata`). When it is missing, tell the user the Attachment was left out and point them to https://github.com/Dangerboi/tern-kata#install.

```bash
tern-kata attach <ref> <file>... [--as <actor>]   # copies each file, prints its stored absolute path, one per line
tern-kata detach <ref> <name> [--as <actor>]      # clears the pointer, deletes the copy when it is on this machine, says what became of the file
```

- Attach the evidence behind a bug report, a finding or a close, then cite the printed path in the comment or close message.
- Run from a checkout of the Issue's own Project; linked worktrees share the primary checkout's store. Anywhere else is refused, and the message names the Project to work from. Closed Issues take attachments.
- `<ref>` is a short id in this checkout's Project, `<project>#<short id>`, or a ULID. `--as` defaults to `$KATA_AUTHOR`, `$USER`, then `git config user.name`.
- A name the Issue already uses is stored as `<stem>-2<ext>`, then `-3`: cite the printed path, not the original name. `attach` stops at the first failing file and keeps those already attached; any failure exits non-zero with the reason on stderr.
- The file must be on this machine. With a Windows Tern window and a Linux session host, a file on the Windows side has to be copied over first.
- Change attachments only through these two commands, which keep the copy and its pointer together (`kata meta unset` would strand the copy).

## Recur

`tern-kata recur` makes an Issue recur, changes or stops its schedule and prints it, with the Recurrence card's rules and messages. It needs the `tern-kata` binary on `PATH` (check with `command -v tern-kata`; PowerShell: `Get-Command tern-kata`); when it is missing, tell the user and point them to https://github.com/Dangerboi/tern-kata#install. kata's CLI has no recurrence command, so do not tell the user to do it by hand.

```bash
tern-kata recur <ref>                                   # prints the schedule in words and the next Occurrence's date, or says the Issue does not recur
tern-kata recur <ref> --frequency weekly --on mon,thu   # makes it recur, or changes its schedule when it already recurs
tern-kata recur <ref> --rule 'FREQ=WEEKLY;BYDAY=MO'     # a raw RRULE instead of the frequency flags
tern-kata recur <ref> --stop                            # stops the Recurrence; existing Occurrences stay
```

- Schedule flags: `--frequency daily|weekly|monthly|yearly`, `--every N`, `--on mon,thu` (weekly), `--day 15|last|"2nd tue"` (monthly, yearly; `last` monthly only), `--month 1-12` (yearly), `--until YYYY-MM-DD` or `--count N`, `--start YYYY-MM-DD`, `--zone <IANA zone>`. Start defaults to the Issue's scheduled date, else today (a recurring Issue keeps its start); the zone to this machine's IANA zone, else UTC; weekly with no `--on` falls on the start date's weekday.
- Closing an Occurrence `--done` creates the next one; `--wontfix` and the other closes do not, so the series stalls.
- Same refs, `--as`, Binding rule and exit codes as `attach`: run from a checkout of the Issue's own Project; one result line on success, the reason on stderr and exit 1 on failure. Invalid input is refused and nothing is sent.
- Changing a schedule someone else changed meanwhile writes nothing: it prints the current schedule and exits 1. Changing or stopping an Issue that never recurred or whose Recurrence was stopped is refused; kata won't make a stopped Issue recur again.
- Leave the `O` card and `kata ui` to people.

## Worker runs

`tern-kata worker` (a foreground process the user starts) runs kata's agent prompt jobs through omp on their machine and records each run in kata. The commands below talk to it; they need the worker running, say `no tern-kata worker is running on this machine` when it is not, and exit 1 with the reason on stderr when refused.

```bash
tern-kata run <job-uid>                       # run an adapter prompt job now; prints the run's uid
tern-kata worker status                       # what the worker serves and runs, with each held run's reason and next step
tern-kata worker edit                         # edit worker.toml in $VISUAL/$EDITOR; a result that would not load reopens with the problem, unchanged or empty cancels
tern-kata worker stop [--now]                 # stop the worker: let its runs finish (default) or end at once; says whether the plugin starts a fresh one
tern-kata worker resume <run-uid> [--as me]   # continue a held run in its omp session
tern-kata worker abandon <run-uid> [--as me]  # give up a held run: kata records it cancelled
```

To set the worker up for the Project this directory is bound to, `tern-kata worker init` writes a new `worker.toml` (it refuses when one exists; optional `--executor`, `--actor`, `--command`, `--model` and `--thinking` set those values, a value the loader refuses writes nothing) and `tern-kata worker add` adds this Project and its checkout to an existing one (it refuses when there is none, or the Project is already listed); `tern-kata worker edit` changes the file by hand in `$VISUAL`/`$EDITOR` (the worker's strict check must pass, else the editor reopens with the problem; a worker running reads the result after `worker stop`), and `e` on the Worker card does the same. Anyone who can write jobs in a listed Project can run prompts and commands on the machine as the user, so run them only when the user asked. The Pane does the same from the Worker card (`W` on the Jobs Page): it shows the setup, sets the worker up or serves the Project once its trust choice is yes, and offers to restart a running worker (when idle, or now) with the stop request. A running worker reads `worker.toml` only at start: after `worker add` or a save from the card, `tern-kata worker stop` ends it and the plugin starts a fresh one. Guide: https://github.com/Dangerboi/tern-kata/blob/main/docs/guide/worker.md

- A **held run** is `unknown` in kata with the reason first in its summary (`interrupted`, `launch_unproven`, `no_receipt`, `group_unproven`, `agent_login`, `agent_quota`, `secret_unavailable`, `invalid_prompt`, `execution_error`, or, before kata saw the run, `admission_rejected` and `create_rejected`). The worker never starts its prompt again and nothing restarts it; read the reason and the run's Issue comments, then resume or abandon.
- Resume starts a new headless omp attempt on the saved session; the agent is told its previous turn may have partly run. It is refused unless the run is held `unknown` with a saved omp session, its agent process group is gone, the Project's lease is held, the Issue is open and the Project has a free slot; the refusal names which. Abandon is refused while a recorded process group is still present and for runs that are not held. Neither kills a process.
- Decide for the user only when they asked you to: resuming spends model usage and lets the agent act again, abandoning ends the run for good. The refusal text is the answer when neither is possible; do not edit `~/.local/state/tern-kata` journals or delete a `quarantine/` folder to get around it.
