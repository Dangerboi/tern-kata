# Glossary

**Pane**: The Tern area the program draws into. _Avoid_: block (in Tern a block is a pane type; in kata `blocks` is a link).

**Page**: One of the Pane's top-level views, chosen from the **Page bar**: Issues (the list and the Issue detail), Board, Jobs and Workflows for now. Each Page keeps its own state, such as its selection, while another is shown. _Avoid_: tab (Tern's tabs and the Issue detail's tabs).

**Page bar**: The row in the top bar that names the Pages with their keys (`1` Issues, `2` Board, `3` Jobs, `5` Workflows); a click on a Page presses its key. _Avoid_: tab bar.

**Issue**: One unit of tracked work in kata, as kata defines it.

**Project**: kata's namespace for issues, links, labels and events.

**Binding**: The Project a workspace names in its `.kata.toml`, or failing that its git alias. A workspace with neither is unbound and opens the Inbox.

**Collection**: A named set of issues the user browses: Inbox, Today, Upcoming, Deadlines, All Open, Logbook, or one project.

**Menu**: The popup inside the Pane that switches the current Collection: a type-to-filter list of the fixed Collections, the bound Project pinned first, a **Projects** entry that opens the list of kata Projects, and New Project, with the current Collection marked. It stands in for the Menu sidebar, which only an unbound directory shows, and only while a fixed Collection is current. _Avoid_: palette (Tern's own command palette).

**Blocking link**: kata's `blocks` / `blocked_by` relationship between two issues. _Avoid_: block, blocker.

**Draft**: An unsaved edit to an issue's text or fields, kept until the user saves or discards it, even if the issue changes underneath.

**Popup card**: A card drawn over the columns that takes the keyboard until closed: Create, Close, Recurrence, the Draft editor, the single-field form, New Project, the Lanes card, the Dates card, the Menu and the Filter form. When several are open, one order (`TopCard`) decides which takes the keys, is drawn and holds the text caret; the help sheet covers them all. A card may hide fields that do not apply to its state (the Recurrence card does by frequency), and tab skips them. Each card declares which global chords it lets through (only `ctrl+c`, from the Menu, Close, Recurrence and New Project). _Avoid_: layer, modal, overlay (Tern's element name).

**Issue detail**: The column showing one Issue: its properties, Children and Activity tabs, with its own row cursor and jump history.

**Pane driver**: The step that turns one Tern input or daemon result into the next state, the view to draw, the Tern ops to send and the daemon commands to run.

**Daemon gateway**: The module that runs one command against the kata daemon and returns exactly one result message. It also runs the Pane's timed waits: the stream's reconnect delay and the end-of-day wake, which it checks against the wall clock at least once a minute.

**Guarded write**: A write the Daemon gateway applies only if the issue has not changed in a way that matters since the user saw it: saving a Draft of the issue's text, posting a comment or reply, closing, and changing or stopping a Recurrence. A Draft or comment conflicts only when the issue's text differs from the text the Draft started against; a revision that moved for another reason (owner, status, claim, expiry) is written against as it now stands. Closing succeeds while the issue is open and reports an issue closed meanwhile without writing; kata enforces this atomically for a close sent with its idempotency key. A Recurrence change is guarded on the Recurrence's rule, start date and time zone: it conflicts only when one of them differs from what the Recurrence card opened with, and then writes nothing and shows the current schedule; a revision that moved because an Occurrence was created is written against as it now stands. Stopping succeeds while the Recurrence is live and reports one already stopped without writing.

**Pane calendar**: The day and time zone the Pane reads every date in. Today's date and zone arrive with each Collection snapshot, and the day ends at local midnight, when the Pane reads the Collection snapshot again.

**Collection snapshot**: What the daemon says the shown Collection holds for the current Collection and filters. One path loads it for the first load, a Collection switch, a filter change, an id search and a live refresh; a result is applied only if it answers the current query.

**Reply**: A comment that answers another comment, with a kind: reply, confirm, refute or supersede.

**Recurrence**: kata's schedule that creates the next Occurrence of an Issue when the current one closes as done. Stopping it creates no further Occurrences and leaves the existing ones as they are.

**Occurrence**: An Issue a Recurrence created, or the Issue that was made the first one.

**Attachment**: A file copied into a Project's Attachment store and pointed to by one `attachment.<name>` meta key on one Issue. The key holds `{"host", "path"}`: the OS host name of the machine with the copy and its absolute native path. Attachments are context for agents; tern-kata lists them and never previews them. _Avoid_: upload, image.

**Attachment pointer**: The `{"host", "path"}` value an `attachment.<name>` meta key holds, naming the machine with the copy and the copy's absolute native path. A pointer is on this machine when its host equals this machine's host name without regard to case; an empty host is never this machine.

**Attachment store**: The `.kata-attachments` folder at the root of a Project's primary checkout, holding `<Issue ULID>/<name>` copies and a `.gitignore` of `*`. Outside git the root is the directory holding the workspace's `.kata.toml`.

**Primary checkout**: The main worktree of a git repository, the parent of git's common directory; every linked worktree shares it.

**Board Page**: The **Page** that shows one Project's Issues as a Board instead of the list (Board mode before Pages existed). It is Pane state that is not saved, the Pane starts on the Issues Page, and it ends when the Collection is not a Project. It acts on the Pane's Project: the current Project Collection, else the Binding. A Pane narrower than the side-by-side width shows the Lane view in place of the Board. In code and on the help sheet "focus" already means the focused column, so no Board Page term uses it.

**Board**: All Lanes side by side, in place of the list column, with the Issue detail column hidden: No lane, one Lane per Lane label, then Done. Each Issue is a card; the selected card shows its full title, owner and labels.

**Relationship**: A parent, child, Blocking link or related link between two Issues, as seen from one of them; the word the Filter form already uses. Read from the Collection snapshot's links, so it counts Issues the Board doesn't draw. _Avoid_: relation, dependency.

**Blocked marker**: The "blocked by 8gm4" line, in the warning tone, on an open Board card that at least one open Issue blocks. It names the open blockers, qualified (`project#id`) outside the Project, cut to three and "+N". Done cards never show it.

**Date badge**: The one badge on an open Board card, in the id row after the priority badge, for the most pressing of its deadline and scheduled date, worded in the Pane calendar's terms: "overdue 2d" (error), "due today" (warning), "scheduled 3d ago" (warning), "scheduled today" (accent), "due tomorrow" (warning), "due 10-24" (muted), "scheduled tomorrow" and "scheduled 10-28" (muted); another year reads YYYY-MM-DD. Done cards and undated cards have none.

**Dates line**: The line on the selected Board card giving both dates in full, "scheduled 2026-10-28 · due 2026-11-02", absent parts left out and no line without dates.

**Relationship highlight**: What the Board draws on the cards related to the selected card: a badge beside the short id in the card head naming the Relationship as seen from the selected card ("parent", "child", "blocks this", "blocked by this", "related") and a ring in the Relationship's tone. A card with several Relationships shows every tag; its ring takes the first of blocks this, blocked by this, parent, child, related. Only the Board draws it, not the Lane view.

**Relationships line**: The line on the selected Board card that names its Relationships: "parent zpxs, 6 children, 2 open, blocks e000, blocked by 8gm4, related 7x74", empty parts left out. Each id is a click that selects that Issue's card, or opens its detail when the Board doesn't draw it; `^` does the same for the parent.

**Lane view**: One Lane as a flat, full-width list under a Lane header line, with the Issue detail column beside it (only the focused column in a Pane narrower than the side-by-side width). Enter or a double-click on a card opens it, `←`/`→` switch Lanes, and Esc returns to the Board. The name avoids "focus", which already means the focused column.

**Lane**: One label of the Lane list, shown as a column and titled from its label ("ready-for-agent" reads "Ready for agent"). Every Lane stands for one kata label, so the Board is a view of kata data, not a copy.

**Move**: Taking an open card one Lane left or right (`<`, `>`) among No lane and the Lanes, never into or out of Done. One daemon command adds the target Lane's label (if missing), then removes the Issue's other Lane labels.

**Lane label**: Any label of the Lane list. An Issue whose only labels are not Lane labels has no Lane label.

**Dates card**: The Popup card `d` opens on the selected Issue in the list, the Board, the Lane view and the Issue detail. Two text fields, Scheduled and Deadline, prefilled with the Issue's dates; each takes `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd` or `+Nw` against the Pane calendar's date, previews the day it resolves to with its weekday, and clears the date when blank. Saving sends one command for the changed fields only; the Daemon gateway patches the `scheduled_on` and `deadline_on` metadata keys one at a time, each guarded on the value the card opened with, and stops at the first failure. A key changed meanwhile is reported, not overwritten. It lets no global chord through. _Avoid_: date form, schedule card.

**Lanes card**: The Popup card `B` opens on the Board or in the Lane view. One text field holds the Lane list as comma-separated names; enter saves it to the Lane list file, esc cancels, and an empty field removes the Project's list and so restores the default. It lets no global chord through. Saving never changes a label in kata. _Avoid_: Lane editor, Lanes form.

**Lane list**: A Project's ordered Lane labels. Saved in the **Lane list file**, `lanes.toml` in tern-kata's user config folder, and loaded when the Board Page turns on or the Project changes; until one is loaded, or when none is saved, it is the default: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`.

**No lane**: The column of open Issues with no Lane label. It is always shown. An Issue with several Lane labels shows once, in the first of its Lanes in Lane order, with an "also" note naming the others.

**Done**: The column of Issues closed in the last N days of the Pane calendar (default 14), at most M of them (default 20), newest closed first; N and M are per Project, set on the Lanes card or in `lanes.toml`. Its header says "20 of 192" when more qualify. It shows only what the Collection snapshot holds, so a status filter of `open` leaves it empty. Each card names the Lane `r` would reopen it into, the Lane its labels point at.

**Worker**: tern-kata's kata adapter, `tern-kata worker`: a foreground process, one per machine, that runs the adapter prompt jobs of the Projects listed in its `worker.toml` through omp and records every run in kata. It reads its Projects' events, holds each Project's lease, and holds a run it cannot prove finished instead of sending its prompt again. It never runs native jobs and has no window or Pane. A Project listed in `worker.toml` is trusted: anyone who can write its jobs can run prompts and commands on the machine. _Avoid_: daemon (kata's, Tern's), scheduler, runner.

**Held run**: A **Run** the worker will not settle on its own because its outcome is uncertain: the agent may have started and the worker cannot prove how it ended (`launch_unproven`, `interrupted`, `no_receipt`, `group_unproven`), a provider failure needs the user (`agent_login`, `agent_quota`), or nothing was started for a reason the user must look at (`secret_unavailable`, `invalid_prompt`, `execution_error`). Kata records it `unknown` with the reason first in the summary; the worker never starts its prompt again and nothing restarts it automatically. The user resumes it (`tern-kata worker resume`, a new headless omp attempt on the saved session; the run goes back to running) or abandons it (`tern-kata worker abandon`, recorded `cancelled`). A run held before kata saw it (`admission_rejected`, `create_rejected`) writes nothing to kata and can only be abandoned. _Avoid_: stuck run, failed run (a failure the worker can prove is `failed`), paused.

**Agent profile**: A named omp command line in `worker.toml` (`[profiles.<name>]`: command, model, thinking level, extra arguments) that a job selects with `options.profile`, `default` when it names none. _Avoid_: agent (the program), config.

**Cron job**: kata's recurring or scheduled definition in one Project: a name, a trigger (cron expression, interval, once, manual, or an Issue's scheduled date or deadline with a lead), an action (notify, or run an agent prompt or a workflow), an Issue policy and the overlap and catchup rules. Kata stores it and reads it back; it also delivers a **native job**, while an **adapter job** is run by a worker on someone's machine. Archiving (kata calls it deleting) a Cron job stops it until restored. The Jobs Page lists them. _Avoid_: cron (alone), schedule (that is the trigger's words), task.

**Native job**: A Cron job that notifies with a fresh Issue per run (`notify` action, `per-run` Issue policy), whatever its enabled state. Kata's own daemon delivers it: each due occurrence becomes a child Issue of the job's **parent** Issue, and the parent's `cron.progress.<job uid>` metadata records how far delivery has got (`anchor`, the job's creation, and `through`). The Pane reads that metadata and the parent's status. The Jobs Page calls it a "recurring child". _Avoid_: recurring job (a **recurrence** is the Issue's own RRULE).

**Cron-made Issue**: An Issue a native job created, recognised by its `cron.occurrence` metadata (`job_uid`, `definition_event_uid`, `scheduled_at`), which only kata's daemon writes. The list row and Board card carry a "◷" marker, distinct from the Recurrence's "↻", and the Issue detail's Cron job property names the job (its uid when the job is gone) and the run's scheduled time; Enter on it opens the Jobs Page on that job. The job's parent Issue shows a Cron jobs property listing its jobs. _Avoid_: occurrence (that is a Recurrence's Issue), generated Issue.

**Adapter job**: Any Cron job that is not native: it runs an agent prompt or a workflow, or notifies about an existing Issue or an Issue date. Kata never runs it; a worker does, so its interval counts from a start only the worker knows and the Jobs Page leaves its next run out. _Avoid_: external job.

**Pause reason**: Why a Cron job is not running, shown in its state: "archived", "paused: Project archived", "disabled", "paused: parent deleted" or "paused: parent closed"; a job with none is "active". Kata stores no pause, so the Pane derives it, in that order: the job's `deleted_at` beats everything, then an archived Project (read from the Project's own state, because kata answers none of its jobs), then `enabled: false`, then the parent's deleted or closed status. Reopening the parent, restoring the Project or the job, or enabling the job resumes a native job.

**Run**: Kata's record of one execution of a Cron job or **Workflow**, written by the executor that ran it (an adapter, a worker, another machine's): its status (`running`, `succeeded`, `failed`, `cancelled` or `unknown`, the last for one nobody could settle), the job or workflow it names, the Issue it ran for, the executor's label, start and end, and a summary (message and token counts). Kata keeps it as a revisioned observation and lists runs newest first by when each was first observed. The Runs Page lists them. _Avoid_: execution, job run, history entry.

**Runs Page**: The **Page** (`4`) that lists the Pane's Project's **Runs**, with a detail (Enter) and a filter to one Cron job. Its state is Pane state that is not saved. It acts on the Pane's Project like the Board Page.

**Workflow**: One of a Project's kata workflows: reusable command and prompt steps, each with a key, optional `after` steps it waits for (earlier steps only) and optional retries (0 to 100). Kata stores it; a Cron job that runs one names it. Archiving one that a live Cron job uses is refused by kata. _Avoid_: pipeline, recipe.

**Workflows Page**: The **Page** (`5`) that lists the Pane's Project's Workflows, archived ones hidden until asked for, with an inspector (Enter) that reads the selected one as steps with their dependencies and retries. Its state is Pane state that is not saved. It acts on the Pane's Project like the Board Page, and ends when the Collection is not a Project.

**Workflow card**: The Popup card `n` and `e` open on the Workflows Page. It holds the Workflow as the JSON text kata takes (`{"name": ..., "definition": {...}}`) in the Draft editor's text field, edited in the card or in `$EDITOR`. Saving checks the text locally against kata's rules first and keeps the text on any failure; the write is a Guarded write on the Workflow's `definition_event_uid`, and a conflict keeps the text beside the Workflow as it stands now.
