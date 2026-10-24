# Jobs

`3` in a Project opens the Jobs Page. It lists the Project's **Cron jobs**, the ones kata runs itself and the ones [a worker](worker.md) runs. The **Job card** makes and edits them.

## Jobs Page

![The Jobs Page with a job selected and its inspector open](../images/jobs.png)

One card per job. A card shows:

- a state badge: `active`, `disabled`, `archived`, or `paused: parent closed`, `paused: parent deleted`, `paused: Project archived`;
- the name;
- the kind: `recurring child`, `agent prompt`, `workflow` or `notify`;
- the schedule in words: "every weekday at 09:00 Europe/Berlin", "every 15 minutes" (only for a step that divides the hour; `*/40` shows as `cron */40 * * * *`), "once at 2026-11-01 09:00 UTC", "manual", "at the Issue's scheduled date", "2 days before the Issue's deadline";
- the agent profile;
- the next run in the Pane calendar's zone.

A schedule that can't be read, or a next run that can't be worked out (a job that isn't running, a manual or Issue-date trigger, an interval of a job a worker runs), is left off the card.

### Keys

| Key | Action |
|---|---|
| `↑`/`↓` or `j`/`k`, `g`, `G`, Page Up/Down | Move the selection |
| Enter or Tab | Show the inspector (narrow Panes) |
| Esc or Tab | Back to the list |
| `n` | New job (opens the [Job card](#job-card)) |
| `e` | Edit the selected job |
| `J` | On an Issue (list, Board, Lane view, Issue detail): new recurring-child job with that Issue as parent |
| `p` | Pause (writes the job disabled) or resume |
| `x` | Archive, or restore an archived job; the footer reads "archive" or "restore" |
| `a` | Show or hide archived jobs (hidden until then) |
| `ctrl+r` | Read the jobs again; they also reload when the daemon reports a change |
| `r` | Open the Runs Page filtered to the selected job (see [Runs](runs.md)) |
| `R` | Ask the worker to run the selected adapter job now |
| `W` | Open the [Worker card](worker.md#from-the-pane-the-worker-card): this machine's worker; set it up, serve the Project, edit, restart or stop it |

### What a card tells you

- A kata-run job ("recurring child") makes a child Issue under its parent each time it fires. It pauses by itself while its parent is closed or deleted or its Project is archived; reopening, restoring or re-enabling resumes it. The Pane works the reason out from the parent and the Project, since kata stores none: a disabled job reads `disabled` whatever its parent is doing, and an archived Project reads as paused for every job the Pane already holds, because kata then answers none of them.
- A card also names the job's latest run (an adapter job: status, when, the first line of its summary) or latest child Issue (a recurring child), and the inspector lists the five most recent.
- An enabled per-run Issue job of a worker that creates more than 24 Issues a day (`worker status` and `worker doctor` list it too) carries the line "creates about N Issues a day; consider an existing Issue"; the job keeps running. An interval of a worker's job counts from the job's creation, as the worker does.
- The Jobs Page keeps its selection while another Page is shown and reads the jobs again when you return.
- A job this machine's worker serves the Project for but sets aside (a profile, checkout or secret `worker.toml` does not map, a timeout over `max_timeout`, a workflow it does not run) shows "Set aside on this machine: <reason>" in the warning tone on its card and a **Set aside** fact in the inspector; the reason is the worker's, from `worker status`.

### Inspector

At 140 columns and wider an **inspector** beside the list shows the selected job's full definition (trigger, prompt or workflow, recipient and message, Issue policy, overlap, catchup, timeout, checkout, the names of its secrets) and, for a recurring-child job, its parent Issue with the delivery progress. Narrower, only the focused column shows: Enter or Tab on a card shows the inspector, Esc or Tab goes back.

With the inspector focused, Enter on a recurring-child job's parent opens that Issue on the Issues Page (a deleted parent leaves a Notice instead).

### Issues made by a job

An Issue a recurring-child job made is marked "◷" in the list and on its Board card, and its detail has a **Cron job** property: the job's name (its uid when the job is gone) and the run's scheduled time. Enter on it (it is the first row the detail's cursor stands on) or a click opens the Jobs Page with that job selected.

The parent's detail has a **Cron jobs** property listing its jobs with state, pause reason and schedule, each openable the same way; a closed parent says its Cron jobs are paused. The detail reads the Project's jobs for this, so it names them a moment after the Issue shows.

### Is the worker running?

A line under the heading says whether **this machine's worker** runs and serves the Pane's Project:

- no worker set up, a `worker.toml` that does not load, or one that does not list the Project (the line says which and points at `W`);
- no worker running yet (the plugin starts one, or `worker.manual` leaves it to you);
- a worker that does not serve the Project;
- a Project it serves but admits nothing for (the reason is the worker's, for instance another machine's lease);
- the worker's executor label with its slots in use.

The Pane asks the worker over its control socket when the page opens, on `ctrl+r` and after a run is requested.

### Run now

`R` asks the worker to run the selected **adapter** job now, under the same lease, overlap and capacity rules as a scheduled run. It is refused with the reason on a recurring-child job (kata runs those, not the worker), on an archived job, when no worker runs on this machine, when the worker does not serve the Project, and with the worker's own words when it declines (a full Project, a closed overlap gate). On success the Notice names the new run, which shows on the [Runs Page](runs.md).

The Jobs Page needs a Project like the Board Page and ends when you switch to a Collection that is not a Project.

### Pause, archive and guarded writes

On the Jobs Page, `p` pauses a job that is running (it writes the job disabled) and resumes one that is not (enabled again); `x` archives the selected job and, on an archived one (press `a` to list them), restores it. All three are Guarded writes: a job changed meanwhile is left alone, the list shows the newer job and a Notice says nothing was written. A job's list and inspector update as soon as the write is answered.

## Job card

![The Job card editing an agent prompt job, with the schedule preview](../images/job-card.png)

`n` on the Jobs Page opens the **Job card** for a new job; `J` on an Issue (list, Board, Lane view, Issue detail) opens it for a new recurring-child job with that Issue as its parent; `e` opens it on the selected job, prefilled.

The **Kind** field offers:

| Kind | Run by |
|---|---|
| **Recurring child** | kata's own daemon |
| **Agent prompt** | an adapter ([the worker](worker.md)) |
| **Workflow** | an adapter |
| **Notify** | an adapter |

kata never runs the last three. The card makes and edits all of them and kata accepts them; the worker runs all three kinds and refuses a workflow it does not support with the reason. A job being edited offers only the kinds of its own side (a recurring child stays one, and a worker's job stays a worker's).

The fields are reached with `tab` and `shift+tab`; a field that doesn't apply is hidden and skipped.

### Name and Trigger

**Name**, and the **Trigger**, whose choices depend on the kind.

- A recurring child takes a **cron** expression (five fields, or a descriptor like `@daily`; `@every` and a `TZ=` prefix are refused) with its **time zone** (an IANA name, blank is UTC, the Pane's zone to start with), or an **interval** in seconds or as a duration like `15m` or `2h`, at least 60 seconds.
- A worker's job also takes **once** (a full RFC 3339 instant with its offset, `2026-11-01T09:00:00+01:00` or `…Z`; a date or a time alone is refused), **manual** (it runs only on `tern-kata run` or a request), **Issue's scheduled date** and, for Notify only, **Issue's deadline** with a **lead** (seconds or `2h`, `1d`; blank fires at the deadline), because kata lets a deadline fire a notification and nothing else.
- An Issue-date trigger names its **Target Issue** by short id.
- A worker's interval has no 60-second floor.

Under the trigger a live preview reads the schedule in words and lists the next runs: three for a cron or a recurring child's interval; the instant of a once trigger; none for manual and Issue dates, and none for a worker's interval, which counts from a start only the worker knows. An interval counts from the job's creation, so a new job's runs are "about" those times. A trigger that can't be read says why instead.

### Recurring child fields

- **Parent Issue**: a short id, which a job made with `J` has filled in. The card resolves it to the Issue's full uid. A job keeps its parent, so the field is hidden when editing.
- **Child title** and **Child body**: the body is one line; a body with line breaks is shown on one line and saved that way.
- **Inbox recipient**: blank is the parent's current owner or author; otherwise an exact address like `alice` or `alice/reviewer`.
- **Inbox message**.

### Agent prompt and Workflow fields

- An **Agent prompt** has a multi-line **Prompt** (Enter adds a line, a pasted block keeps its lines).
- A **Workflow** job has a **Workflow** picker of the Project's live workflows (archived ones are not offered; with none, the card says to make one on the [Workflows Page](workflows.md)).
- Both have an **Agent profile** (stored in `options.profile`; the options the card doesn't edit are kept), a **Checkout** (stored as `checkout_key`), **Secrets** (`ENV=source` pairs, stored as `secret_refs`: the environment variable the run sees and the name of the source the worker's config defines; never a value), the **Run Issue** policy, **Overlap** and a **Timeout**.
- **Run Issue** policy: **a new Issue per run**, with a title, a one-line body and optional **scheduled** and **deadline offsets** from the run such as `1d` or `-2h`, or **an existing Issue**, named by short id, which every run belongs to.
- **Overlap**: wait for the running one (the default) or run alongside it.
- **Timeout**: seconds or `30m`, `2h`; blank is the worker's default.
- **Profile and Checkout are pickers** when this machine has a `worker.toml` (tern-kata's config folder, read each time the card opens with the worker's own parser): the profiles and checkouts it names, plus "Worker default" and "None". Without one, or when it can't be read (the card says why), both take free text. A profile or checkout the job holds that the worker config doesn't name stays listed, marked "not available".

### Notify fields

A **Notify** job notifies an existing Issue: the **Target Issue**, or the trigger's Issue for an Issue-date trigger. For an Issue-date trigger an optional **Other Issue to notify about** names a second Issue to notify instead of the trigger's. It also has an **Inbox recipient** (blank is the Issue's current owner or author; a named recipient needs a **message**) and an **Inbox message**.

### Catch up and Enabled

**Catch up** is one of:

- the latest missed run;
- every missed run;
- skip runs older than the **grace** in seconds, which skip requires for a recurring child and takes as optional for a worker's job.

It applies to cron, interval and once triggers. **Enabled** turns the job on or off. Turning a paused recurring child back on from the card, with catch-up on every missed run and runs missed, opens the [Backfill card](#backfill-card) first, as `p` does; it sends the card's draft with the answer, and Esc returns to the card.

### Saving

`ctrl+o` saves (or the button).

<details>
<summary>What the card refuses, and what a save does</summary>

Invalid input is refused naming the field and nothing is sent:

- a bad cron expression;
- an unknown time zone;
- an interval under 60 seconds for a recurring child, or one beyond kata's range;
- skip without a grace on a recurring child;
- a blank name, child title or prompt;
- a workflow that isn't a live one of the Project;
- an Issue-date trigger or existing-Issue policy with no Issue;
- a once instant without its offset;
- a secret that isn't `ENV=source`;
- a checkout key with a space;
- a span out of kata's range;
- a named recipient without a message.

An Issue that doesn't exist is refused by the write naming the field, and kata's own refusal of anything else is shown as it says it.

A new job is sent at a uid the card made when it opened, so sending the same card again is not a second job.

An edit is a Guarded write on the job's `definition_event_uid`: if the job changed meanwhile nothing is written, the card shows the newer job (name, state, schedule, child title, catch-up, when it changed) and keeps your draft, and saving again needs an explicit choice:

| Key | Result |
|---|---|
| `o` | save the draft over the newer job |
| `u` | replace the draft with the newer job |
| `d` | discard the draft |

</details>

## Backfill card

Before a recurring-child job whose catch-up is every missed run makes the runs it missed, the **Backfill card** asks. Reopening the parent with `r` on the Issues Page, resuming the job with `p` and restoring it with `x` on an archived one each end a pause, after which kata's daemon creates one child Issue for every occurrence since the parent's progress stopped advancing, all at once.

When that count is above zero the card names it, job by job (a parent can have several jobs), and offers:

| Key | Choice |
|---|---|
| `c` | **create them** |
| `l` | **switch to latest and resume** ("restore" or "reopen" for those keys): first replaces the job's catch-up with `latest` as a Guarded write, then goes on |
| `esc` | **cancel**: changes nothing |

A job that catches up with `latest` or `skip`, one with nothing missed and one that would stay paused for another reason (the parent is still closed, the job disabled) ask nothing. `r` reads the parent's jobs afresh first, since its progress stopped when it closed. A job changed meanwhile is not written; the list shows it and a Notice says so. Restoring a deleted parent from the Menu asks no Backfill; only reopening a closed parent does.
