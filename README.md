# tern-kata

Browse and edit [kata](https://katatracker.com/) Issues in a native [Tern](https://docs.stencil.so/tern/) Pane.

kata ([source](https://github.com/kenn-io/kata)) is an issue tracker built for coding agents and the people steering them. Agents create, claim, relate and close Issues from its CLI, and people follow the same work in its terminal UI, `kata tui`. By default Issues live in a local SQLite database rather than in your repo, and no hosted service is needed. tern-kata shows that work in a Tern Pane drawn with Tern's own widgets instead of terminal text.

The same example Project in tern-kata (top) and in `kata tui` (bottom), both running in Tern:

![tern-kata showing the acme-shop Project with "Add Apple Pay to checkout" selected](docs/images/tern-kata.png)

![kata tui showing the same Project and Issue](docs/images/kata-tui.png)

In tern-kata, Issue bodies render as Markdown, priorities and labels show as chips, and rows, tabs and links respond to mouse clicks.

## Features

- **Issue list and detail**: a Project's Issues nested under their parents, with priority, status, child counts and age, beside the Issue detail with its Markdown body, properties and Activity tabs (Comments, Events, Links, Attachments). Changes made anywhere, by you or an agent, show up live.
- **Collections and the Menu**: Inbox, Today, Upcoming, Deadlines, All Open, Logbook and every kata Project, plus a search (`/`) and filters (`f`, `s`) over any of them.
- **Editing from the Pane**: Create (`n`, `N` for a child), Close (`x`), reopen (`r`), comments (`c` in the detail) and replies (`R`), labels (`+`, `-`), owner (`a`, `A`), priority (`!`), parent (`p`), recurrence (`O`) and the Issue body in Tern's editor (`e`).
- **Pages (Issues, Board, Jobs and Workflows), Attachments and the Dates card**, shown below.
- **[kata links](#clickable-ticket-links-from-agents)** from agents' chat replies open the Issue in tern-kata.

**Board Page** ([details](#pages-and-the-board-page)): `2` or `b` in a Project shows its Issues as cards in Lanes, one per triage label, with No lane on the left and recently closed Issues in Done on the right. `<` and `>` move a card to the next Lane by swapping its label in kata. A card shows its priority, its most pressing date and a Blocked marker when an open Issue blocks it; selecting one tags its related cards (`parent`, `blocks this`) and lists its Relationships.

![The acme-shop Board with "Add Apple Pay to checkout" selected, its parent and blocker tagged](docs/images/board.png)

Enter opens the card's Lane beside the Issue detail. Clicking an id on the Relationships line selects that card; an Issue the Board doesn't draw, like this one in another Project, opens in the detail, and Esc goes back.

![The Ready for agent Lane beside the detail of a related Issue in the acme-infra Project](docs/images/related-detail.png)

**Jobs Page** ([details](#jobs-page)): `3` in a Project lists its Cron jobs with their state (and why one is paused), kind, schedule in words, agent profile and next run, with the selected job's definition and parent beside them; Enter on a recurring job's parent opens that Issue.

**Attachments** ([details](#attachments)): `u` attaches a screenshot, log or any other file to an Issue, and the Attachments tab lists them. Agents find them through the Issue's kata metadata, and `tern-kata attach` does the same from a shell.

![The Attachments tab of "Add Apple Pay to checkout" listing a screenshot and a log](docs/images/attachments.png)

**Dates card** ([details](#dates-card)): `d` sets an Issue's scheduled date and deadline, typed as a date, `today`, `tomorrow`, `+3d` or `+1w`, with the day it resolves to shown under the field. Dated cards on the Board show a badge such as "overdue 1d" or "due tomorrow".

![The Dates card over the Board, with "+1w" resolving to 2026-10-29](docs/images/dates-card.png)

## How it works

The plugin has two halves:

- **`tern-kata`**: a Go program that runs in a Pane. It talks to kata's daemon and draws the Issue list and detail with Tern's own widgets, using the Tern Surface Protocol. Run outside Tern, it falls back to `kata tui`.
- **Window plugin** (`window.luau`): adds **Open kata** to the palette and binds `F9`. Both open `tern-kata` in a split on the right, in the focused Pane's directory. It also opens kata links: `kata://<project>/<id>` or `https://kata.invalid/<project>/<id>` focuses the kata Pane in the current tab (or opens one there) and selects that Issue. A kata Pane in another tab is left alone.

In a directory bound to a kata Project, `tern-kata` opens that Project's Issues. A directory is bound when it, or a parent up to the enclosing git root, contains a `.kata.toml`, which `kata init` creates. Anywhere else it opens Inbox beside the Menu sidebar, where you can switch to another view (Today, All Open, Logbook and others), browse your kata Projects or create a new one. In a bound directory the same Menu opens as a popup with `esc`, and `P` opens it straight at the list of Projects. Press `?` for the keybindings.

## Install

Requires kata v0.19.0+ and a Tern that supports the Surface Protocol.

1. On the **session host** (the machine whose shells run in your Panes), install kata, then tern-kata. On Linux or macOS:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.sh | sh
   ```

   On Windows, in PowerShell:

   ```powershell
   irm https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.ps1 | iex
   ```

   They install the latest release to `~/.local/bin` (`%LOCALAPPDATA%\Programs\tern-kata` on Windows, added to your user `PATH`), verify its checksum, and also install the window plugin when `tern` is on `PATH`. Run in a terminal with `npx` available, they then offer to install the [agent skills](#agent-skills). Once the plugin is installed, the **Install tern-kata** palette command runs the same one-liner in a new Pane. `tern-kata --help` lists the flags.

   Alternatively, with Go 1.27+:

   ```sh
   CGO_ENABLED=0 go install github.com/Dangerboi/tern-kata/cmd/tern-kata@latest
   ```

   Go puts the binary in `~/go/bin` (`%USERPROFILE%\go\bin` on Windows); it must be on your shell's `PATH`.

2. On the **machine showing the Tern window**, install the window plugin:

   ```sh
   tern plugin install github.com/Dangerboi/tern-kata
   ```

The window machine needs only the window plugin; kata and tern-kata run on the session host. For example, with a Tern window on Windows 11 and Panes on an Ubuntu host, install the window plugin on Windows and kata and tern-kata on Ubuntu. If the session host and the window are the same machine, do both steps on it.

## Install with an agent

In a Pane on the session host, give your coding agent this prompt:

```text
Install tern-kata by following the "Install with an agent" section of https://raw.githubusercontent.com/Dangerboi/tern-kata/main/README.md
```

Agents: follow these steps only when the user has asked you to install tern-kata.

1. Check that `kata` is on `PATH`. If it isn't, stop and tell the user that tern-kata requires kata, which they install themselves: https://katatracker.com/.
2. Run the install one-liner from [Install](#install) step 1 for this machine's OS.
3. Run `tern-kata --version`. If it isn't found, pass on the `PATH` instructions the install script printed.
4. If the script reported that `tern` is not on `PATH`, tell the user to run `tern plugin install github.com/Dangerboi/tern-kata` on the machine that shows the Tern window. Without it, the Open kata command, `F9` and kata links don't work.
5. Ask the user whether to add the rule from [Clickable ticket links from agents](#clickable-ticket-links-from-agents) to your global rules file, the one you load in every repo. The links only work when your chat is shown in Tern with the window plugin installed. Copy the rule block exactly, and skip it if the file already contains it.
6. Ask the user whether to install the [agent skills](#agent-skills). Run `npx skills add Dangerboi/tern-kata -g` only if they agree; it needs Node.js, and with only Bun installed `bunx` replaces `npx`.

## Clickable ticket links from agents

Tern draws `kata://` links in chat as plain text, so agents should write ticket references as `https://kata.invalid/...` links, which the plugin intercepts. To make your agent do that in every kata-tracked repo, add this to its global rules file (`~/.omp/agent/RULES.md` for OMP):

```markdown
In a repo with a `.kata.toml`, write kata ticket references in chat replies as `[<id>](https://kata.invalid/<project>/<id>)`, with `<project>` the name in `.kata.toml`. The tern-kata window plugin opens them in tern-kata; Tern draws `kata://` links in chat as plain text. Everywhere else (files, commit messages, kata Issue text) write the bare id; those links only work inside Tern.
```

## Agent skills

The repo ships two skills for coding agents:

- `kata`: the kata CLI, for finding, creating, claiming, commenting on, linking and closing Issues, and coordinating subagents through them.
- `tern-kata`: reading an Issue's [Attachments](#attachments), attaching files with `tern-kata attach`, and making Issues recur with [`tern-kata recur`](#recurring-issues-from-a-shell).

Install them for the agents on this machine with the [skills](https://github.com/vercel-labs/skills) CLI, which asks which skills and which agents. It needs Node.js; with only Bun installed, use `bunx` in place of `npx`:

```sh
npx skills add Dangerboi/tern-kata -g
```

## Attachments

Attach a screenshot, log or any other file to an Issue so whoever picks it up sees what you saw. In a Pane, `u` on the selected Issue (in the list or the Issue detail) asks for a path, quoted, starting with `~` or relative to the directory, and attaches the file; a refusal or a failure shows in the form. `U` lists the Issue's Attachments by name: `down` and `up` move and `enter` removes the one chosen, with no further confirmation, and a notice says what happened to the file. `U` on an Issue with none says so. Both keys explain why when this directory is not bound to the Issue's Project. From a shell or for an agent:

```sh
tern-kata attach <ref> <file>...   # prints each stored absolute path, one per line
tern-kata detach <ref> <name>
```

`<ref>` is anything `kata show` takes: a short id in this directory's Project, `<project>#<short id>`, or a ULID. `--as <actor>` names the actor. Both commands run without Tern and never open a Pane; a failure prints kata's message on stderr and exits non-zero. `attach` stops at the first failing file and keeps those already attached. The Issue's Project must be the Project this directory is bound to, and a closed Issue is fine. The file is copied, so deleting the original breaks nothing, and a path in quotes, starting with `~` or relative to the directory all work.

The copy goes to `.kata-attachments/<Issue ULID>/<name>` at the root of the Project's primary checkout (the main worktree, so linked worktrees share it; outside git, the directory holding `.kata.toml`). The folder carries its own `.gitignore` with `*`, so nothing shows in `git status` and your repo's ignore files stay untouched. A name already used on the Issue becomes `name-2.ext`, then `name-3.ext`.

The pointer agents read is one kata meta key per Attachment, visible in `kata show`, `kata show --json` and kata's MCP show tool:

```json
{"attachment.shot.png": {"host": "my-laptop", "path": "/home/me/repo/.kata-attachments/01J.../shot.png"}}
```

`host` is the OS host name of the machine that holds the copy and `path` is the absolute native path there. `detach` clears the key, then deletes the copy only if `host` is this machine; otherwise it says the file was left on that host, or was already gone. If the pointer is cleared but the copy can't be deleted, `detach` still prints `Removed <name>; could not delete <path>: <reason>` and exits non-zero, and `U` shows the same line as its notice. A pointer that changed while it was being removed is left alone and nothing is deleted.

The file must already be on the session host, the machine running tern-kata. With a Windows Tern window and a Linux session host, copy a Windows file over first. With a remote kata daemon, attaching still works for your own agents on this machine, but teammates' agents see a path on your machine that they can open only if they can SSH to it; the `tern-kata` agent skill copies the file over SSH when that works.

## Recurring issues from a shell

`O` opens the Recurrence card in a Pane. `tern-kata recur` does the same from a shell or an agent, without Tern, with the card's rules and messages:

```sh
tern-kata recur <ref>                                         # the schedule in words and the next Occurrence's date
tern-kata recur <ref> --frequency weekly --on mon,thu         # make it recur, or change its schedule
tern-kata recur <ref> --frequency monthly --day "2nd tue" --count 6 --zone Europe/Oslo
tern-kata recur <ref> --rule 'FREQ=WEEKLY;BYDAY=MO'           # a raw RRULE instead of the frequency flags
tern-kata recur <ref> --stop                                  # existing Occurrences stay
```

Flags: `--frequency daily|weekly|monthly|yearly`, `--every N`, `--on mon,thu`, `--day 15|last|"2nd tue"`, `--month 1-12`, `--until DATE` or `--count N`, `--start DATE`, `--zone <IANA>`. The start date defaults to the Issue's scheduled date or today, the zone to this machine's IANA zone or UTC. Refs, `--as` and the Binding rule are those of `attach`; success prints one line and a failure prints the reason on stderr and exits 1. Invalid input is refused with the card's message and nothing is sent; a schedule changed by someone else while the command ran is not overwritten but printed, and the command exits 1.

## Pages and the Board Page

The top bar holds the **Page bar**, "1 Issues  2 Board  3 Jobs  5 Workflows". `1`, `2`, `3` and `5`, or a click on a Page, switch Pages; `b` toggles between Issues and Board. The Issues Page is the list and Issue detail; each Page keeps its selection while another is shown. The Board Page and the Jobs Page act on the Pane's Project: the current Project Collection, else the workspace's Binding (from Inbox or another fixed Collection in a bound workspace it switches to the Binding's Project). With neither it says so and opens the Menu's Projects list.

### Board Page

`b` in a Project Collection shows the Project's Issues as a **Board**; `b` or `1` again returns to the list with the same Issue selected. Columns, left to right: **No lane** (open Issues with none of the Lane labels, always shown), one **Lane** per label of the Lane list, and **Done** (Issues closed in the last 14 days, at most 20 of them, newest closed first; the header says "Done 20 of 192" when more qualify; both numbers are per Project, see below).

- The Lane list is `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human` unless the Project has its own (see below). A Lane is one kata label: an Issue sits in the first of its Lane labels in Lane order, and an Issue with none (an unlabelled one, or one labelled only `bug`) sits in No lane. An Issue with several Lane labels shows once, with "also: …" naming the rest. A closed Issue keeps its Lane label, and its Done card says "r → <Lane>", the Lane `r` reopens it into.
- A card shows its short id, its priority as a badge beside it (`P0` error, `P1` warning, `P2` info, other priorities muted; none without a priority, and Done cards keep it), its title over at most two lines (a recurring Issue's starts with "↻ ", as in the list, and Done cards keep it) and its labels that aren't Lane labels. The selected card shows its full title, owner and every label.
- `↑`/`↓`/`j`/`k` move within a Lane and `←`/`→` to the neighbouring Lane that has cards. A click selects a card. `x` opens the Close card, `r` reopens a closed Issue into its Lane, and `+`, `-`, `a`, `A`, `!`, `p`, `u`, `U`, `e` and `n` act on the selected card as in the list. `esc` opens the Menu.
- `>` moves the selected card one Lane right and `<` one Lane left, among No lane and the Lanes; a Move never enters or leaves Done (`x` closes, `r` reopens). Leaving No lane adds the first Lane's label; moving into No lane removes the Lane labels and keeps every other label. A Move adds the target label first, then removes the other Lane labels, so a failure halfway leaves the Issue in two Lanes, never none, and a Move of an Issue with two Lane labels leaves it with one. The card shows in its new Lane at once; a failure leaves a Notice with the daemon's reason. Moves are refused while the daemon is disconnected.
- `B` (on the Board or in the Lane view) opens the **Lanes card**: a field holding the Lane list as comma-separated names, in order (`needs-triage, in review, ready-for-agent`), then **Done days** and **Done cards** (`tab`/`shift+tab` move between the fields). A blank Done field restores its default (14 days, 20 cards); anything but a positive whole number is refused with a message naming the field. `enter` saves and `esc` cancels. Each name is tidied to a label (`In review` reads `in-review`); an empty name, a name over 64 characters and a duplicate are refused with a message naming the entry. Saving an empty field removes the Project's list, which restores the default list. `B` waits, with a Notice, until the saved list has loaded. A muted line lists the labels on loaded Issues that aren't Lanes yet. Saving writes the Lane list file only and never changes a label in kata, so renaming a Lane just changes which label it shows.
- An open card that an open Issue blocks shows a **Blocked marker** in the warning tone, "blocked by 8gm4": every open blocker, as a short id inside the Project and `project#id` outside it, cut to three and "+N". It shows on every card, selected or not, but never on Done cards, and a card whose blockers are all closed shows none. Blockers count even when the Board doesn't draw them (filtered out, closed beyond Done, in another Project), and a live refresh that closes the last one removes the marker.
- While a card is selected on the Board (not in the Lane view), every other drawn card related to it gets a **Relationship highlight**: a badge beside its short id naming the Relationship as seen from the selected card (`parent`, `child`, `blocks this`, `blocked by this`, `related`) and a ring in its tone (parent accent, child info, blocks this error, blocked by this warning, related neutral). A card with several Relationships shows every tag and takes the ring of the first of blocks this, blocked by this, parent, child, related. The selected card keeps its own ring and Done cards keep their muted ring. The highlight follows the selection and shows nothing without one.
- The selected card also shows a **Relationships line**: `parent zpxs`, `6 children, 2 open` (counted from each child's status), `blocks e000` (open Issues only), `blocked by 8gm4` (open blockers only) and `related 7x74`. A part with nothing in it is left out, and a card with no Relationships has no line. Ids are short inside the Project and `project#id` outside it, cut to three and "+N", and Issues the Board doesn't draw count. Click an id to select that card and scroll it into view in its Lane (in the Lane view, the Pane switches to that Lane); if the Board doesn't draw it (filtered out, closed beyond Done, in another Project) the click opens its detail beside the Lane view, and Esc goes back to the card you came from, then to the Lane view and the Board. `^` selects the parent's card the same way, or leaves "<id> has no parent." when there is none.
- An open card with a deadline or a scheduled date shows one **Date badge** in its id row, after the priority badge and before the Relationship tags, for the most pressing date, read as the Today, Upcoming and Deadlines Collections read it and compared as days in the Pane calendar. In order: a deadline before today "overdue 2d" (error); a deadline of today "due today" (warning); a scheduled date before today "scheduled 3d ago" (warning); a scheduled date of today "scheduled today" (accent); a later deadline "due tomorrow" (warning) or "due 10-24" (muted); a later scheduled date "scheduled tomorrow" or "scheduled 10-28" (muted). A date in another year than today reads YYYY-MM-DD. Done cards and cards without dates have no badge. Before the Pane knows today's date the badge shows the earlier date plainly, muted ("due 2026-10-24"). The wording moves on at local midnight without a key.
- The selected card also shows a **Dates line**, "scheduled 2026-10-28 · due 2026-11-02", with an absent date left out and no line without dates.
- The filters (`/`, `f`, `s`, `c`) narrow the Board as they do the list, and a live refresh moves cards. Done shows only what the Collection holds, so the `open` status filter leaves it empty.
- Enter on a card, or a double-click, opens the **Lane view**: that Lane's Issues as a flat, full-width list under a Lane header line, with the Issue detail beside it at 140 columns and wider. It works like the list (tab moves to the detail, the same keys apply, including `L`), except `←`/`→` switch to the neighbouring Lane that has cards, and `<`/`>` Move the selected card with the selection going to the next card in the Lane. Esc returns to the Board with the same card selected.
- The Board Page needs a Project and ends when you switch to a Collection that is not a Project. A Pane under 140 columns opens it straight in the Lane view, on the selected Issue's Lane, and narrowing the Pane switches the Board to the Lane view; only the focused column shows, so Enter shows the detail, Esc returns to the Lane view, and Esc from the Lane view opens the Menu.

### Workflows Page

`5` shows the Pane's Project's kata **Workflows**, one row each with its name and step count; archived ones stay hidden until `a` shows them. The Project is found as the Board Page finds it, and the Page ends when you leave the Project Collection. Esc closes the steps first, then goes to the Menu.

- **Enter** opens the inspector for the selected workflow: each step in order with its kind (`command` or `prompt`), its command or prompt, the steps it waits for (`after review`) and its retries, and who wrote it. Beside the list at 140 columns and wider, in place of it in a narrower Pane.
- **`n` and `e`** open the **Workflow card** on a new workflow or the selected one, as the JSON kata takes: `{"name": "...", "definition": {"version": 1, "steps": [{"key": "review", "kind": "prompt", "prompt": "..."}, {"key": "test", "kind": "command", "command": "make test", "after": ["review"], "retries": 1}]}}`. Edit it in the card or press `ctrl+e` for `$EDITOR`; `ctrl+o` saves.
- Saving checks the text first, and nothing is sent while it fails: valid JSON, a name, version 1, at least one step, unique step keys of letters, digits, `-`, `_` and `.`, `after` naming earlier steps only (so steps are listed in the order they run), retries from 0 to 100, and exactly one of `command` or `prompt` matching the step's kind. The card names the problem and keeps your text.
- A save is a Guarded write on the version you started from. If the workflow changed meanwhile, nothing is written: the card shows your text beside the workflow as it stands now, `o` saves yours over it and `d` drops your text.
- `x` archives the selected workflow and `r` restores an archived one, both guarded the same way. Kata refuses to archive a workflow a live Cron job uses, and the Pane shows its message: "workflow W is used by live job J; update or delete that job first".

### The Lane list file

A Project's own Lane list lives in `lanes.toml` in `tern-kata` under your user config directory: `~/.config/tern-kata/lanes.toml` on Linux (`$XDG_CONFIG_HOME` if set), `~/Library/Application Support/tern-kata/lanes.toml` on macOS and `%AppData%\tern-kata\lanes.toml` on Windows. It has one table per Project UID, with the Project's name (for readers), its ordered labels and, optionally, Done's limits:

```toml
["01JABC..."]
name = "work"
labels = ["needs-triage", "in-review", "ready-for-agent"]
done_days = 14
done_cards = 20
```

`done_days` and `done_cards` are how many days back Done reaches and how many cards it shows at most; a missing or zero value means the default (14 and 20). A table with only `done_days`/`done_cards` and no `labels` keeps the default Lanes.

tern-kata reads it each time the Board turns on and when the Project changes while it is on, so an edit shows the next time you press `b`; the Lanes card (`B`) writes it for you. Saving rewrites only the Project's `name`, `labels`, `done_days` and `done_cards`; other tables and any other keys you add are kept, but comments are lost. Labels are tidied like kata's (`In review` reads `in-review`). A missing file, or no table for the Project, gives the default Lanes. A file that can't be read or parsed gives the default Lanes and a Notice, and tern-kata won't overwrite it. Removing a label sends its Issues to No lane, or to their next Lane label; kata isn't touched.

### Jobs Page

`3` shows the Pane's Project's **Cron jobs**, the ones kata runs itself and the ones a worker runs, one card each. The card has a state badge (`active`, `disabled`, `archived` or `paused: parent closed`, `paused: parent deleted`, `paused: Project archived`), the name, the kind (`recurring child`, `agent prompt`, `workflow` or `notify`), the schedule in words ("every weekday at 09:00 Europe/Berlin", "every 15 minutes", "once at 2026-11-01 09:00 UTC", "manual", "at the Issue's scheduled date", "2 days before the Issue's deadline"), the agent profile and the next run in the Pane calendar's zone. A schedule that can't be read, or a next run that can't be worked out (a job that isn't running, a manual or Issue-date trigger, an interval of a job a worker runs), is left off the card.

- A kata-run job ("recurring child") makes a child Issue under its parent each time it fires. It pauses by itself while its parent is closed or deleted or its Project is archived; reopening, restoring or re-enabling resumes it. The Pane works the reason out from the parent and the Project, since kata stores none: a disabled job reads `disabled` whatever its parent is doing, and an archived Project reads as paused for every job the Pane already holds, because kata then answers none of them.
- `a` shows or hides archived jobs; they are hidden until then. `ctrl+r` reads the jobs again; they also reload when the daemon reports a change. `↑`/`↓` (or `j`/`k`), `g`, `G` and Page Up/Down move the selection.
- At 140 columns and wider an **inspector** beside the list shows the selected job's full definition (trigger, prompt or workflow, recipient and message, Issue policy, overlap, catchup, timeout, checkout, the names of its secrets) and, for a recurring-child job, its parent Issue with the delivery progress. Narrower, only the focused column shows: Enter or Tab on a card shows the inspector, Esc or Tab goes back.
- With the inspector focused, Enter on a recurring-child job's parent opens that Issue on the Issues Page (a deleted parent leaves a Notice instead).
- The Jobs Page needs a Project like the Board Page and ends when you switch to a Collection that is not a Project.

## Dates card

`d` on the selected Issue (in the list, on the Board, in the Lane view and in the Issue detail) opens the **Dates card**: a Scheduled field and a Deadline field holding the Issue's current dates. `tab` moves between them, `enter` saves and `esc` cancels. A field takes `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd` (N days on) or `+Nw` (N weeks on), counted from the Pane calendar's date, and a muted line under it shows the date it resolves to with its weekday (`2026-10-24 Sat`). A blank field clears the date; anything else is refused with a message naming the field. A date kata holds with a time (`2026-11-02T17:30:00`) is shown as it is and left alone unless you edit that field.

The Create card (`n`, and `N` for a child Issue) has the same two fields after Priority, in tab order, with the same input forms, weekday preview and refusal naming the field (nothing is created). The dates go into the one create request as the new Issue's `scheduled_on` and `deadline_on` metadata; a field left blank sends none. Retrying a failed create reuses its idempotency key, and editing a date makes a new request.

Saving writes the changed dates only, as the Issue's `scheduled_on` and `deadline_on` metadata, each guarded on the value the card opened with: a date changed elsewhere (the kata CLI, an agent, another Pane) since then is reported and not overwritten, and the card stays open with the reason. It is refused while the daemon is disconnected.

## Compared with `kata tui`

tern-kata takes kata tui's movement keys: `j`/`k` and the arrows, `ctrl+n`/`ctrl+p` to move a row, `pgup`/`pgdn` and `alt+v`/`ctrl+v` to page, `g`/`G`, and `I` to open the Inbox (from the list, the Issue detail and the Menu sidebar; tern-kata opens it, where kata tui toggles it). `?` lists every key. The Emacs keys and `I` are plain text in search and in Popup cards.

The Issue detail's Activity tabs are Comments, Events, Links and Attachments, cycled with `tab` and `shift+tab`. Attachments lists the Issue's `attachment.*` meta keys by name: a file held on this machine is a link to it, and one held on another machine reads `<name> on <host>`.

`R` on a comment in the Issue detail's Comments tab replies to it; `ctrl+r` in the card cycles reply, confirm, refute and supersede.

It lacks these kata tui keys:

- `u`: undo the last change.
- `t`: timed assignment.
- `[` and `]`: previous and next comment link.

## Known issues

- **Windows: each new kata Pane takes a moment to open.** Tern starts a Pane's command through PowerShell and can't yet pass `-NoProfile`, so every new kata Pane loads your PowerShell profile before tern-kata starts; a large profile makes the wait longer. Once open, the Pane runs at full speed. A fix has been requested from the Tern developers.
- **Open kata, `F9` and kata links open a tab instead of a split in newer Panes.** Tern 0.9.4 gives window plugins Pane ids without the bits above 2^44, so the plugin can't split a Pane whose id is that large and opens tern-kata in a new tab in the focused Pane's directory instead. For the same reason a kata link never reuses an open kata Pane or selects its Issue, and a failed start shows no toast. It needs a fix in Tern.

## Development

```sh
CGO_ENABLED=0 go build -o tern-kata ./cmd/tern-kata
CGO_ENABLED=0 go test ./...
./smoke/run.sh      # end-to-end against a headless Tern and a throwaway kata daemon
tern plugin link .  # use this checkout as the window plugin
tern plugin reload  # exits 1 if a plugin fails to load; window failures show only as a toast
```

Releases: pushing a `vX.Y.Z` tag runs `.github/workflows/release.yml` (vet, test, then publish), with the tag's `CHANGELOG.md` section as the release notes. Check it locally before tagging:

```sh
scripts/release-notes.sh vX.Y.Z   # prints the notes; fails unless CHANGELOG.md has a [X.Y.Z] section
scripts/release.sh vX.Y.Z         # writes dist/ (gitignored); fails unless plugin.toml's version is X.Y.Z and the notes exist; publishes nothing
```

It builds `tern-kata_<os>_<arch>.tar.gz` (`.zip` for `windows`) for `linux`, `darwin` and `windows` on `amd64` and `arm64`, plus `checksums.txt` in `sha256sum` format.

A release build sets the version with `-ldflags "-X main.version=vX.Y.Z"`. Without it, `tern-kata --version` reports the Go module version (`go install …@vX.Y.Z`), or `dev` for an untagged build.

`window.luau` is `--!strict`. To type-check it against the installed Tern's API (needs [luau-lsp](https://github.com/JohnnyMorganz/luau-lsp)):

```sh
tern plugin types .  # writes tern.d.luau (gitignored; regenerate per Tern version)
luau-lsp analyze --platform=standard --definitions=@tern=tern.d.luau --ignore=tern.d.luau window.luau
```
