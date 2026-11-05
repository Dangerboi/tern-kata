# Compared with `kata tui`

![kata tui showing the same Project and Issue](../images/kata-tui.png)

tern-kata takes kata tui's movement keys:

- `j`/`k` and the arrows;
- `ctrl+n`/`ctrl+p` to move a row;
- `pgup`/`pgdn` and `alt+v`/`ctrl+v` to page;
- `g`/`G`;
- `I` to open the Inbox (from every Page, the Issue detail and the Menu sidebar; tern-kata opens it, where kata tui toggles it);
- `o` in the nested Issue list to switch siblings between creation order and blocking order (blockers first; creation order breaks ties and cycles), which it answers with "graph order: topological" or "graph order: temporal". The choice lasts for the Pane and is not saved; `o` on the Runs Page still opens the session.

`?` lists the global keys and the keys of the Page it is opened over. The Emacs keys and `I` are plain text in search and in Popup cards.

## Activity tabs

The Issue detail's Activity tabs are Comments, Events, Links and Attachments, cycled with `tab` and `shift+tab`. Attachments lists the Issue's native attachments with their media type and size, and `enter` downloads and opens the one under the cursor (see [Attachments](attachments.md)).

The Comments tab shows a discussion as a reply tree, as kata's TUI does: a reply is indented under the comment it answers, down to four levels, after which deeper replies stay at the fourth. Replies to one comment are ordered by creation time, then UID, and a cycle of replies shows each comment once. A reply from another Issue has no row of its own: it is a muted "↳ <kind> <handle> in #<id> by <author> …" line under the comment it answers. A comment that confirms or refutes another carries a `confirm` or `refute` tag on the answered comment, reading "(before edit)" when all that evidence predates an edit of it, and a reply whose target changed after it reads "Target edited after this reply".

`R` on a comment in the Issue detail's Comments tab replies to it; `ctrl+r` in the card cycles reply, confirm, refute and supersede.

`]` and `[` on a comment in the Comments tab step through its Comment links, wrapping around: the comment it answers (if any), then the Replies to it on any Issue, as kata returns them with the Issue detail. The comment under the cursor shows "link 2/3: fay refutes in #bbbb" for the chosen link (the first until you step), and `enter` jumps to it, across Issues and Projects, landing on that comment. A comment without Comment links keeps `enter` as it was.

## Credentials

`C` opens kata tui's Credentials view as a Lettered Page, and the Menu lists it. Its wording, fields and 30-second refresh are kata tui's; see [Pages and the Board](pages-and-board.md#lettered-pages). `n` creates an API token and `x` revokes the selected one after asking, as `kata tokens create` and `kata tokens revoke` do (kata tui itself only lists tokens); the secret goes to a new owner-only file you name and is never shown. The Token card and its rules are in [Pages and the Board](pages-and-board.md#lettered-pages).

## Federation

`F` opens kata tui's federation view as a Lettered Page, and the Menu lists it: the spoke federations with their status, a detail on Enter, `x` to leave one with the hub revoke, `b` to browse the hubs' Projects (read only) and `n` to enroll a Project in a hub's federation. Its wording, header, states and the order of the enrollment's steps are kata tui's; the hubs are the entries of kata's daemon catalog, each used with its own token. See [Pages and the Board](pages-and-board.md#lettered-pages).

## Daemons

`D` opens kata tui's daemons view as a Lettered Page, and the Menu lists it: the daemons kata's `config.toml` names, with kata tui's columns and `Enter` to switch this Pane to one. The switch lives in this Pane only and writes nothing to disk, as in kata tui; the top bar names the daemon by its catalog name and, when the daemon says who you are, the account ("Daemon: work · remote · kata v0.21.0 · as alice"). See [Pages and the Board](pages-and-board.md#lettered-pages).

## Jobs, Runs and Workflows

kata tui switches tabs with `1` to `4`: kata (the Issues), Jobs, Runs and Workflows. tern-kata numbers its Pages the same way for those four: `1` Issues, `2` Jobs, `3` Runs, `4` Workflows. The Board and Attention Pages come after them, as `5` and `6`.

kata tui's three tabs read the Project's Cron jobs, Runs and workflows, and open a record to follow its links (a run to its job and workflow, a workflow to the jobs that use it). Its only writes are `n` and `e` on a job. tern-kata's Pages show the same records and go further:

| | kata tui | tern-kata |
|---|---|---|
| Job card | New and edit for four kinds: recurring child issue, agent prompt, workflow and notify | The same four kinds, with the profile and checkout as pickers read from `worker.toml`, and a Backfill card before a catch-up of every missed run |
| Pausing and archiving | None; editing an archived job stops with "use the CLI to restore it before editing" | `p` pauses or resumes, `x` archives or restores, `a` shows archived jobs |
| Run now | None | `R` asks the worker to run the selected adapter job |
| Worker | None | `W` opens the Worker card for this machine's worker |
| Runs | The newest 100, read-only | `m` reads older runs, `f` narrows to a job, `i` opens the run's Issue, and `o`, `u` and `A` open the session of, resume or abandon a run of this machine's worker, held runs included |
| Workflows | Read-only | The Workflow card makes and edits workflows as JSON, `x` archives, `r` restores and `a` shows archived ones |

kata tui's `r` refreshes the tab; tern-kata reads again with `ctrl+r`, and `r` on the Jobs Page opens the Runs Page filtered to the selected job.

## Links

In the Issue detail, `b` adds a Blocking link and `l` a related link to the Issue you name: a short id, `#id` or `project#id`. The field is kata tui's add-link request. kata records a Blocking link from the Issue you add it on, so the field reads "This Issue blocks (Issue id)"; kata tui words the same key "add blocker". When kata refuses the link (an Issue to itself, an archived Project, a Project outside your scope) its reason shows under the field. On the Board and in the Lane view `b` still toggles the Board.

## Timed assignment

`t` in the Issue detail gives the Issue to you for a set time. The field reads "Assignment duration for #id (1m..24h, e.g. 1h)" and takes a Go duration of whole seconds from 1m to 24h (`90m`, `1h`, `24h`); anything else is refused with kata tui's wording, "duration must be a whole number of seconds from 1m through 24h". kata records the claim for your actor, and the Issue detail shows the owner and "until" the claim ends.

## Moving an Issue

kata tui has no key for moving an Issue to another Project; kata web and the CLI do. `M` in tern-kata opens the Move card, which sends kata's move; like the CLI and web it moves the Issue with its descendants unless the card's "Only this Issue" switch is on (see [Moving an Issue](pages-and-board.md#moving-an-issue)).

## Deleting and restoring an Issue

kata tui has no key for deleting an Issue; the CLI does. `X` in tern-kata opens the Delete card, which sends kata's delete with the confirmation `kata delete` asks for: the typed qualified id, as `DELETE <project>#<id>` in the confirmation header. The Menu's **Restore a deleted Issue** brings one back. See [Deleting and restoring an Issue](pages-and-board.md#deleting-and-restoring-an-issue). Purge stays in the CLI.

## Closing with an attachment

The Close card for a `done` close can cite one or more of the Issue's Attachments as evidence, and the Events tab names a cited one; kata tui has no such choice. See [Attachments](attachments.md#citing-an-attachment-when-closing).

## Undo

`u` in the list, on the Board, in the Lane view and in the Issue detail takes back the newest change this Pane made, as in kata tui. The Pane remembers the last 20 completed changes, across Issues, and undoes the newest first. It undoes a close (reopens), a reopen (closes as done, opening the Close card first when kata requires evidence), an owner or priority change, a label added or removed, a body edit, a link added (the link is removed, found by type and its two ends), a Lane Move (its label changes reversed), a Dates card change (both dates put back, each guarded on the value the change wrote) and a delete (the Issue is restored; "already restored" when kata no longer holds it as deleted).

Before it writes, `u` checks, with kata tui's words, that it is still the same daemon and account ("daemon instance changed", "daemon principal changed"), the same Issue ("issue identity changed") and that nobody changed the Issue since: the revision the change produced for owner, priority and link ("issue owner changed since the action" and the like), the body text the change wrote for a body edit ("issue body changed since the action"; kata v0.20 takes no revision precondition on an edit, so a body edit landing in the instant between this check and the write is overwritten), the label state for a label added or removed (a label someone else changed reads "already restored"), the dates the change wrote for a Dates card change ("issue dates changed since the action"). A refusal drops that change and says so; the one after it can still be undone. An Issue already as the undo would leave it reads "already restored". A second `u` while one is out says "undo already in progress".

Every other change to an Issue clears the history, and the next `u` says "nothing to undo: <reason>": creating an Issue, a comment or Reply, a timed assignment, a Recurrence change or stop, attaching or unlinking a file, and every Cron job or Workflow change. A change whose outcome is unknown (a timeout) clears it too. `u` never undoes an older change across one of these.

## Quitting

`q` outside text fields asks "Quit tern-kata? y/n": `y` quits, `n` or `esc` stays, and every other key is ignored while the question shows. `ctrl+c` quits at once, asked or not. `q` typed into search or a Popup card stays text. kata's own `[tui] confirm_quit = false` in `<KATA_HOME>/config.toml` (`~/.kata/config.toml` unless `KATA_HOME` is set) makes `q` quit at once in tern-kata too; a config file kata would refuse leaves the question on. Closing a tern-kata Pane (its tab, or Tern's close) follows the same setting as it stands when the Pane starts: Tern closes it without asking only when `confirm_quit = false`, and asks otherwise.

## Key aliases

kata tui's alternative keys work where kata has them:

| Key | Same as |
|---|---|
| `ctrl+j`, `ctrl+k` | `tab`, `shift+tab`: the next and previous tab of the Issue detail |
| `backspace` | `esc` where it goes back outside text fields: the Issue detail, the Lane view, the Menu's Views and Projects levels, the Jobs and Runs inspectors and the Lettered Pages |
| `ctrl+alt+<`, `ctrl+alt+>` | `g`, `G`: the first and last row of every list, the Issue detail and the Menu |
| `!` then `0`-`4` or `-` | set or clear the priority at once; any other key closes the field and is handled as usual |
| `T` | open the Attention Page, from any Page and the Menu |
