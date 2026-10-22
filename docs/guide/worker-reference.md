# Worker reference

Details behind [the worker](worker.md): every `worker.toml` key, how the worker checks its route to kata, where it keeps state, what a run does, and how to recover. Read the trust note in the worker guide before listing a Project.

## `worker.toml`

The file is decoded strictly (an unknown key is an error) and read once at start. It holds no kata address or credential.

| Key | Meaning |
|---|---|
| `executor` | Required. A unique, stable name for this machine's worker. |
| `actor` | Optional. Author of runs and Issues; default `$KATA_AUTHOR`, `$USER`, then git `user.name`. |
| `[limits]` `max_active_runs` | Optional. Runs at once across all Projects; unset means each Project's capacity only. |
| `[limits]` `default_timeout` | Optional. Timeout when a job sets none; a job's `timeout_seconds` overrides it, up to `max_timeout`. |
| `[limits]` `max_run_prompts` | Optional. The most prompts one run may send, 1 to 100; default 12. It is frozen into a run when the run is prepared. A workflow that could send more (prompt steps plus their retries) is refused. The cap is global: a profile has none of its own. |
| `[limits]` `max_timeout` | Optional. The longest timeout a job may ask for; a job over it is set aside. |
| `[[projects]]` `name` | A kata Project; listing it trusts it. |
| `[[projects]]` `capacity` | Runs at once for the Project; default 1. |
| `[[projects]]` `actor` | Optional. Overrides the worker's `actor` for this Project; the order is Project `actor`, then top-level `actor`, then the default. |
| `[[projects]]` `checkouts` | Optional allowlist of checkout keys; default all. |
| `[[projects]]` `profiles` | Optional allowlist of profile names; default all. |
| `[checkouts]` | A job's `checkout_key` mapped to an absolute directory. |
| `[profiles.<name>]` | A job's `options.profile` (default `default`); `default` is required. Keys: `command`, `model`, `thinking`, `args`. |
| `[secrets]` | A job's `secret_refs` name mapped to where the value lives; never a literal. |

<details>
<summary>A fully commented single-Project example</summary>

```toml
executor = "laptop"            # required: a unique, stable name for this machine's worker
actor    = "dangerboi"         # optional: author of runs and Issues; default $KATA_AUTHOR, $USER, git user.name

[limits]                       # all optional
max_active_runs = 4            # across all Projects; unset = each Project's capacity only
default_timeout = "2h"         # a job's timeout_seconds overrides it, up to max_timeout
max_timeout     = "8h"
max_run_prompts = 12           # most prompts one run may send; 1 to 100, default 12

[[projects]]
name      = "tern-kata"        # a kata Project; it is trusted
capacity  = 1                  # runs at once; default 1
actor     = "dangerboi"        # optional: overrides the worker's actor for this Project
checkouts = ["tern-kata"]      # optional allowlist of checkout keys; default all
profiles  = ["default"]        # optional allowlist of profile names; default all

[checkouts]                    # a job's checkout_key -> absolute directory
tern-kata = "/home/me/Code/tern-kata"

[profiles.default]             # a job's options.profile (default "default"); this one is required
command  = "omp"
model    = "anthropic/claude-sonnet-5-5"
thinking = "medium"
args     = []                  # extra arguments, e.g. ["--profile", "worker"] to isolate omp's own state

[secrets]                      # a job's secret_refs name -> where the value lives; never a literal
github-review = { file = "~/.config/tern-kata/github-review" }   # a private (0600) regular file
# token = { env = "GITHUB_TOKEN" }
```

</details>

### Daemon, pins and secrets

- The worker finds the daemon the way the Pane does: `KATA_SERVER`, `.kata.local.toml` in its working directory, then the default local daemon.
- It refuses a Project whose uid, daemon instance, authenticated actor, actor or executor differs from what it saw at first start (`state/projects/<uid>/pin.json`). A Project whose pin cannot be saved is not served.
- A secret may not name kata's own credential (`KATA_AUTH_TOKEN`, `KATA_AUTH_TOKEN_FILE`, or a file inside kata's home), and a secret file must be a regular file owned by you with mode 0600.

## Route checks

The worker checks its route to each Project when its event stream (re)connects, before each admission and each report (a passing check is reused for 5 seconds) and every 5 minutes.

- A daemon that stops advertising `cron_v1`, answers for another Project or is another instance admits nothing and delivers nothing for that Project until a check passes. The check is retried after 30 seconds, doubling to 5 minutes.
- The worker makes at most four kata calls at once, each with a 60-second deadline, and resolves two Projects at a time at start.
- A Project you remove from `worker.toml` has its saved reports delivered and its lease released at the next start.
- `worker status` and `worker doctor` list each job the worker sets aside, with its reason (an unmapped profile, checkout or secret, a timeout over `max_timeout`, a workflow it does not run, a trigger it cannot schedule), and warn of a per-run Issue job that creates more than 24 Issues a day. Neither stops any other job.

## State and leases

State lives in `$XDG_STATE_HOME/tern-kata` (else `~/.local/state/tern-kata`):

- `worker.lock`: a second worker on the machine is refused;
- the control socket;
- each run's journal (`runs/<run uid>/run.json` with its prompt, session and output);
- the pins.

`tern-kata worker stop` keeps leases: a stopped worker does not release them, and its next start renews them.

Across machines, the worker holds a lease for each Project in the Project's own kata metadata (`tern_kata.lease`, renewed every 3 minutes, expiring after 10). A Project whose lease another worker holds admits nothing here and `worker status` says which worker owns it. `tern-kata worker --takeover` moves it on purpose.

## What a run does

The agent is told its Issue and how to leave a record: comment what it did, close the Issue with evidence when the work is verified. When it exits without closing, the worker comments its final message and labels the Issue `needs-review`.

- Exit status 0 after a finished turn is `succeeded`, anything else `failed`.
- A run over its timeout is stopped and failed as "timed out".
- A run whose prompt may have reached the agent when the worker stopped is held (`unknown` in kata, with the reason first in its summary), never repeated.
- A job the worker cannot run (a native job, a workflow it does not support) is refused with the reason.

## Recovery and held runs

The worker saves each run's journal before every step that has a side effect (creating its Issue, telling kata it runs, starting omp, recording the process, recording the exit, each comment and the final status). A worker that is stopped or killed and started again on the same state folder continues from the journal: it finishes what it can prove and never sends a prompt that may have reached the agent a second time.

A run whose agent may have started, and whose outcome the worker cannot prove, becomes a **held run**: kata records it `unknown` with the reason first in its summary, the Issue gets a comment, the run keeps its job's overlap gate closed, and nothing restarts it automatically. `worker status` lists each held run with its reason and how to settle it:

| Reason | What happened |
|---|---|
| `launch_unproven` | the worker stopped while starting the agent, so it may have started (no process to check) |
| `interrupted` | the worker stopped or died while the agent ran, and the agent is gone |
| `no_receipt` | the agent finished its turn and is gone, but the worker never saved its exit status |
| `group_unproven` | the agent's, or an exited command's, process group is still present (or cannot be checked); the worker waits it out and never stops it |
| `agent_login`, `agent_quota` | omp ended the turn with a provider answer of 401/403 (log in again) or 429 (wait for the usage limit to reset) |
| `secret_unavailable`, `invalid_prompt`, `execution_error` | nothing was started: a secret could not be read, the prompt is over 256 KiB, or the worker failed inside its own code (read its log) |
| `admission_rejected`, `create_rejected` | held before kata saw the run (the lease moved, or kata refused the run's Issue): nothing was written to kata |

A run that is not uncertain is never held: a nonzero exit, a timeout and a spawn error are recorded `failed` with the cause. On a timeout the process group is stopped and proven gone: SIGTERM, then SIGKILL after 10 seconds, until nothing of it is left. A command step that the worker stops, by a timeout or an abandon, is stopped the same way, however its children treat SIGTERM; only a command that exited by itself and left a process behind is held.

### Resume

`tern-kata worker resume <run-uid>` continues a held run. It starts a new headless `omp -p --mode json --session-dir <run folder> --resume <session id>` on the run's saved session, with a prompt that says the previous turn may have partly run and asks the agent to check the checkout and the Issue for work already done before redoing it.

- The held attempt's own prompt is never sent again.
- The run goes back to `running` in kata with its original start time, gets a fresh timeout and ends recorded `succeeded` or `failed` by the new attempt. The held attempt's output stays in the run folder as `output.attempt1.jsonl`.
- A resume is refused when the run is not held with `unknown`, when no omp session was saved (`launch_unproven`, and the nothing-started reasons: only abandon remains), when its recorded process group is still present, when the Project's lease is not held here, when the Issue is closed or when the Project is at capacity.

### Abandon

`tern-kata worker abandon <run-uid>` gives up a held run: kata records it `cancelled` with "abandoned by <actor>" and the held reason, the Issue gets a comment, and the run's slot and overlap gate are freed. A hold from before kata saw the run is closed locally. Resume and abandon both take `--as <actor>`. A `launch_unproven` run keeps its slot until it is abandoned, because the worker cannot prove no agent exists.

`worker abandon` also handles other cases:

- A run this machine has no journal of at all but kata shows running for this executor (the state folder was lost): it is never rerun, `worker doctor` lists it with the command, and abandoning records it `unknown` ("local state lost") and comments on its Issue.
- A **running** run: abandoning stops it. The process group gets SIGTERM, SIGKILL after 10 seconds, and the run is recorded `cancelled` ("abandoned by <actor> (was running)") once the group is gone.
- A **notify** run that has not settled is never abandoned: it is refused with the reason that its notification is being sent and the run ends in a moment, recorded as it was sent.

Like `run` and `resume`, every form of abandon is refused while another worker owns the Project's lease.

### Quarantine

A journal the worker cannot read is moved whole to `quarantine/runs/<run uid>`, which keeps that run from ever starting again; `worker doctor` lists them. Never delete one to unblock a job. `tern-kata worker abandon <run-uid>` acknowledges one: if kata still shows the run running for this executor it is recorded `unknown` with the summary "local state lost" (and its Issue gets a comment), the folder stays where it is and keeps its veto, and the doctor lists it as a note instead of a problem.

### Limits and cleanup

A workflow run's time limit starts when its run is prepared, and a resume gets one fresh limit from the moment it is accepted, not from when a step starts. A step is never started once the limit has passed: the run is recorded failed "timed out" and its unstarted steps are skipped. Tokens are summed over every attempt of a run: retried steps and resumed attempts all count. A daily pass removes the folders of runs that were settled and fully reported more than 14 days ago; held runs, runs with a write still owed and runs in flight stay.

## Workflow runs

A job whose action is a workflow runs its steps one at a time, in the order the workflow lists them (kata requires `after` to name earlier steps).

- **Prompt step.** One fresh omp process with the same profile, prompt contract and journal as a prompt job. The prompt names the workflow, its `about`, and the step's place in it. Steps hand work to each other through the checkout and the Issue, since each step starts a new session.
- **Command step.** `sh -c <command>` in the checkout, in its own process group, as the worker's user, with the job's secrets and `TERN_KATA_RUN`, `TERN_KATA_RUN_DIR` and `KATA_PROJECT` in its environment but not the worker's kata credentials. Its output goes to `output.log`, keeping the first 1 MiB and the last 256 KiB with a marker between. Exit status 0 with nothing left in its process group succeeds; a leftover process, a missing exit status or a worker stop holds the run.
- **Retries.** A step that fails with a certain nonzero exit is retried up to its `retries` as a new attempt (a retried prompt step is told it is attempt N after a failure and to check the checkout first). A timeout is never retried, because the job's timeout is the whole run's, shared by every step and retry.
- **Failure.** A step that fails after its retries skips the steps that run after it, steps that do not depend on it still run, and the run is `failed`.
- **Records.** kata records one run for the workflow, its summary listing each step's result. The local journal and the [Runs Page](runs.md) detail keep each step's state, attempts, process and output (`steps/<n>-<attempt>/` in the run's folder).
- **Held runs.** A held run stops at its uncertain step: `worker resume` continues an omp step in its session and runs a held command step's command again, and the steps before it are never repeated.
- **Refused workflows.** The worker refuses workflows that use `options.on_fail` loops or `options.on_failure: "park"`, name a profile that differs from the job's, give a command step a profile, or could send more prompts than `max_run_prompts` (prompt steps plus their retries).

## On Windows

A run's processes are a Job Object instead of a process group.

- The agent or command starts suspended, joins its own job, and only then runs, so everything it starts belongs to the run. The worker proves the run's processes gone by the job's process count.
- The job ends its processes when the worker's handle to it closes, so a worker that dies takes its running agent with it (on Linux the parent-death signal does the same), and the next start finds that run `interrupted`.
- Windows has no SIGTERM for these processes: a timeout, an `A` or a worker stop ends the job at once rather than after a 10-second grace.
- Command steps still run through `sh -c`, so `sh` must be on the worker's `PATH`. Install [Git for Windows](https://gitforwindows.org/); it has one in `C:\Program Files\Git\bin`.
