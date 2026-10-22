# Runs

`4` in a Project opens the Runs Page. It lists the Project's **Runs**, newest first as kata lists them (by when each was first observed).

A Run is kata's record of one execution of a Cron job or workflow, written by whoever ran it: an adapter, a worker here or another machine's. The Pane shows what kata holds, whoever the executor was.

![The Runs Page with an unknown run selected and its detail open](../images/runs.png)

Each card has:

- a status badge: `running`, `succeeded`, `failed`, `cancelled` or `unknown`;
- the job's or workflow's name;
- the Issue the run is for (`#short title`);
- the executor (its label, else the actor);
- when it started;
- how long it took (so far, while it runs);
- the first line of its summary.

## Keys

| Key | Action |
|---|---|
| Enter or Tab | Open the detail; Enter reads the run from kata again |
| Esc or Tab | Back to the list |
| `i` | Open the run's Issue on the Issues Page, from the list or the detail (a run with no Issue leaves a Notice) |
| `f` | Narrow the list to the selected run's job, or show every run again |
| `m` | Read the next older 100 runs |
| `ctrl+r` | Read the newest page again |
| `o` | Open the omp session of a finished or held run of this machine in a new Tern tab |
| `u` | Resume the selected held run through the worker |
| `A` | Abandon a held run, or cancel a run still running on this machine |

`r` on the [Jobs Page](jobs.md) opens the Runs Page filtered to the selected job.

## Detail

Enter (or Tab) opens the **detail**: status, job and workflow, Issue, executor, actor and teammate, start, end, duration, the summary's whole message, input and output token counts, and the references kata holds (occurrence, job and workflow uids and the version of each the run used, Issue uid, run uid and revision). At 140 columns and wider the detail sits beside the list, narrower only the focused column shows.

## Reading and filtering

- On the Runs Page `f` narrows the list to the selected run's job, or shows every run again. The filter stays, like the selection and the older runs already read, while another Page is shown and until the Project changes. A run of a workflow alone has no job to narrow to.
- kata lists at most 100 runs a page, filters them only by job (a run that names a workflow alone matches no job, and there is no filter by status or workflow) and names none of them by job or workflow. The Pane reads the newest 100, `m` reads the next older 100 and adds them, and it looks up each job's, workflow's and Issue's name itself. `ctrl+r` and daemon events read the newest page again.
- The [Workflows Page](workflows.md) shows each workflow's latest run on its row and its latest runs in its inspector. It reads the Project's newest 100 runs and then older pages (up to 1000 runs) until every workflow has its latest run and the selected one the five its inspector lists: a workflow whose runs are all older than that shows none.

## Runs of this machine

A run whose executor label is this machine's worker's is also read from the worker's local state, over its control socket. A run of another executor shows only what kata holds.

- A **held** run (the worker could not prove what its agent did) shows the badge `held` and its reason on its card. The detail says why it is held, what the worker knew, whether `u` would be accepted and, when not, why. See [held runs](worker-reference.md#recovery-and-held-runs).
- The detail also lists the run's **steps** (kind, attempt, state, exit status), the **tail of the agent's output** (its last assistant messages, at most 4 KiB), the end of its stderr while it runs or is held, and the omp session id.
- While a run is running the page asks the worker every 2 seconds and reads the newest runs again, so the output grows and the status turns `succeeded` or `failed` without a key.

### Open the session, resume, abandon

- `o` opens the omp session of a finished or held run of this machine in a new Tern tab: `tern new tab --cwd <the run's checkout> -- omp --resume <session id> --session-dir <the run's session folder>`. The tab opens in Tern's session daemon and is not focused; the Notice says it opened. It is refused for a run still running, a run with no session, and a run of another machine. Needs `tern` and `omp` on the Pane's `PATH`.
- `u` resumes the selected **held** run through the worker and `A` abandons it, or cancels a run still running on this machine (the worker stops its agent's process group and records the run cancelled), like `tern-kata worker resume` and `abandon`. The worker's refusal (its Project's lease is held elsewhere, no free slot, the agent's process group is still present) is shown as the Notice. Both are refused for a run that is not held and for a run of another executor.

The Runs Page needs a Project like the Board Page and ends when you switch to a Collection that is not a Project.
