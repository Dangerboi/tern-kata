# The worker

kata stores jobs that run an agent prompt on a schedule, and the evidence of each run, but runs none of them. `tern-kata worker` is the adapter that does, on your machine, with no Tern tab or window open.

What it does:

- runs prompt jobs, and the steps of workflow jobs, through [omp](https://github.com/can1357/oh-my-pi) headless in the job's checkout with the job's agent profile (command steps through your shell);
- creates each run's Issue, or comments on the job's existing one;
- records every run in kata as running, then succeeded or failed with a summary and token counts.

What it never does:

- run native jobs (a recurring child kata's daemon delivers);
- send a prompt twice: a run whose outcome is uncertain after a crash is held.

It needs kata v0.21.0 or newer (kata v0.21 upgrades its database to schema 43 on first start, and older kata binaries cannot open it afterwards; kata keeps a backup from before the upgrade, and PostgreSQL stores need `kata storage postgres migrate` with the daemon stopped) and `omp` on `PATH`. On Windows, workflow command steps also need `sh`, which [Git for Windows](https://gitforwindows.org/) provides.

## Trust

> **Trust.** Anyone who can write jobs in a Project you list in `worker.toml` can run prompts on this machine, as your user, with your files and credentials, and the agent runs unattended (`--approval-mode yolo`) in the checkout the job names. Workflow command steps run only for a Project with `commands = true`, and a job can use only the secrets its Project lists in `secrets`; both are off until you list them (for a `worker.toml` from before this, `tern-kata worker upgrade` lists what the existing jobs use). There is no sandbox. List only Projects whose writers you trust, and keep secrets out of the checkouts a job can name.

## Set it up

Setting up means one file, `worker.toml`, in tern-kata's config folder (`~/.config/tern-kata/worker.toml` on Linux, honouring `$XDG_CONFIG_HOME`, beside `lanes.toml`; `%AppData%\tern-kata\worker.toml` on Windows). The Worker card in the Pane and the `tern-kata worker` commands write it the same way, with the same checks, so use whichever is at hand:

| | From the Pane | From a shell |
|---|---|---|
| First Project | **Set up** | `tern-kata worker init` |
| Another Project | **Serve this Project** | `tern-kata worker add` |
| Change anything else | **Edit** (`e`) | `tern-kata worker edit` |
| Pick up a change | the restart choice | `tern-kata worker stop` |
| Supervision on or off | `s` | `worker.manual` |

Both need a folder bound to a kata Project, the Binding the Pane uses: a `.kata.toml` in the folder or a parent up to the git root (`kata init` creates one). The **checkout** the worker maps for the Project is the folder holding that `.kata.toml`, also when you start from a subdirectory. Neither calls the kata daemon. What they write, and the checks, are in the [reference](worker-reference.md#writing-workertoml).

### From the Pane: the Worker card

`W` on the Jobs Page (from the list or the inspector) opens the Worker card for the Project the Pane's folder is bound to.

![The Worker card for the acme-shop Project, with the worker.toml that serves it](../images/worker-card.png)

It reads this machine once, asks the worker for its status and shows, one fact per row:

- **worker.toml**: whether it exists, whether the worker's loader accepts it (or why not), and whether it lists the Project; the checkouts it maps are named when they differ from the card's Checkout row;
- **Worker**: whether a worker runs, with its executor, process id and runs in flight;
- **Supervision**: whether the plugin supervises it, or `worker.manual` turned that off.

What the card offers depends on the file:

- **No `worker.toml`: Set up.** Executor, actor, command, model and thinking, prefilled as `worker init` would.
- **A file that loads but does not list the Project: Serve this Project.** It does what `worker add` does.
- **Any file: Edit** (`e`). The file opens in `$VISUAL`, else `$EDITOR`, else `vi`, as a `.toml` file; the text you save is checked and written like the commands do, unless `worker.toml` changed while you edited. Then nothing is written, the card says so, reads the file again and keeps your text, which lacks that change: `e` and a save replace the file with it deliberately. A refusal names the problem on the card and keeps your text, so `e` again reopens it rather than the file; an unchanged or empty result changes nothing. Edit can repair a file that does not load, which Set up and Serve never overwrite.

Set up and Serve show the checkout and the trust note, and save only when the trust choice is Yes (it starts at No; space, left and right or a click change it). A value the loader refuses is named, with focus on that field, and your input stays. A folder bound to no Project gets a message and no fields. Esc closes the card.

After a save or an edit with a worker running, the card asks how to restart it, since it reads `worker.toml` only at start. **When idle** (`i` or enter, the default) and **now** (`n`) are the two [stop modes](#restarting-and-stopping), each listed beside what it does to the runs in flight, which the card names. Esc leaves the worker running. Its notice then says whether the plugin starts a fresh worker. With no worker running, the card says the plugin starts one within 10 seconds, or, under `worker.manual`, that you start it.

`s` on the card turns [supervision](#supervision) off or on (when the card draws no fields; otherwise `s` is typed). The Jobs Page banner says per state what is missing (no file, a file that does not load, a Project not listed, no worker running, `worker.manual`) and points at `W`.

### From a shell

Run these in a directory bound to the Project.

```sh
tern-kata worker init                    # writes worker.toml for this Project
tern-kata worker init --executor laptop --actor dangerboi --command omp --model anthropic/claude-sonnet-5-5 --thinking medium
tern-kata worker add                     # in each further Project's directory
tern-kata worker edit                    # change the file by hand
tern-kata worker upgrade                 # list the secrets and command steps existing jobs use
tern-kata worker doctor                  # check the result
```

- **`init`** writes a new file for this Project: its checkout, a `default` profile, capacity 3. The five optional flags set the values the Set up card prefills; one you leave out keeps its default, and a value the loader refuses is refused naming it, with nothing written. It refuses when the file exists (use `add`) or the directory is not bound. It ends by printing the path, a line of the trust note, and that the plugin's host half starts the worker within 10 seconds (else run `tern-kata worker`).
- **`add`** adds this Project to an existing file, with its checkout, and leaves every other line and comment byte for byte as it was. It refuses when there is no file (use `init`), the directory is not bound, or the Project is already listed.
- **`edit`** opens the file in `$VISUAL`, else `$EDITOR`, else `vi`, on a temporary copy. A result that does not load reopens the editor on your text with the problem as `#!` comment lines at the top, until it loads; an unchanged or empty result cancels. If `worker.toml` changed while you edited, nothing is written and the error names the temporary file that keeps your edit. It refuses with no file and points at `init`.

After any of them, a running worker reads the new file only after `tern-kata worker stop`; `edit` and `add` say so, and whether a worker runs.

## A two-Project `worker.toml`

After `worker init` in the `acme-shop` checkout and `worker add` in the `acme-infra` checkout, the file reads (comments abridged):

```toml
# Anyone who can write jobs in a listed Project can run prompts on this machine as you,
# and commands too in a Project you give commands = true.
executor = "laptop"
# actor = "dangerboi"

[[projects]]
name      = "acme-shop"
capacity  = 3
checkouts = ["acme-shop"]

[checkouts]
acme-shop = "/home/me/Code/acme-shop"
acme-infra = "/home/me/Code/acme-infra"

[profiles.default]
command = "omp"

[[projects]]
name      = "acme-infra"
capacity  = 3
checkouts = ["acme-infra"]
```

Each `[[projects]]` block lists its own checkout only, so a job in `acme-shop` cannot name the `acme-infra` directory. A profile without an `extensions` key runs omp with `--no-extensions`; see the [`extensions` row of the worker reference](worker-reference.md#workertoml). Every key is described there too.

## Restarting and stopping

`tern-kata worker stop` asks the running worker to stop. It is how a change to `worker.toml` takes effect: the plugin then starts a fresh worker that reads the file. The Pane sends the same request from the restart choice.

- **By default** (when idle) the worker admits nothing new, lets its runs in flight finish and report to kata, and exits with status 0. `tern-kata worker status` says it is stopping and how many runs it waits for.
- **`--now`** ends it at once. The runs it interrupts are held `interrupted` and resumable, as after a terminate signal. A `--now` during a drain upgrades the drain.

The command prints the mode, the runs in flight and whether a fresh worker follows: it does when the plugin supervises this worker; a worker you started by hand, or one under `worker.manual`, stays stopped. The [reference](worker-reference.md#stopping-the-worker) has the refusals and what is kept.

## Supervision

The plugin's host half starts `tern-kata worker` when `worker.toml` exists and keeps it running, inside Tern's session daemon, so no window is needed. You do not start the worker yourself; `tern-kata worker` in a terminal is for a worker you want to watch.

The host half doesn't read the config folder itself: while no worker of its own runs, it asks `tern-kata worker supervised` every 10 seconds, which answers with `worker.toml`'s text when this machine's worker is supervised, or names the worker that already runs. The binary and the window plugin are installed separately, so update them together with `tern-kata update`, which also brings this machine's plugin copy to the binary's version and, when the worker is supervised, stops it when idle so the plugin starts the new binary (a worker started by hand is only named: restart it yourself): a tern-kata too old to know `worker supervised` leaves the worker unstarted, and the host half logs this once per daemon with the advice to update tern-kata.

- **Restarts.** It starts the worker again whenever it exits, after 1 second, then 2, 4 and so on up to a minute between tries. A worker that stayed up for a minute starts the count over.
- **When it starts.** When Tern's daemon starts, which a window does on demand and Tern's "Start at login" setting does at login. Nothing starts the daemon at boot or after a quit by itself: with Tern not running, no worker starts. A worker already running keeps running when the daemon stops (see below).
- **Finding the program.** The host half finds `tern-kata` on the daemon's `PATH` or in the folder the install scripts use (`~/.local/bin`, `%LOCALAPPDATA%\Programs\tern-kata`, `$TERN_KATA_INSTALL_DIR`). It gives the worker the daemon's `PATH` plus those folders, Tern's bundled omp, `~/go/bin` and Homebrew's, so `omp` resolves even when Tern was started from a launcher with a short `PATH`.
- **Logs.** Its log lines (`tern-kata: worker started`, `the worker exited with status ...`) are in Tern's daemon log. `tern-kata worker status` says whether Tern supervises the worker or you started it by hand, and names the installed binary's version when the worker runs another one; for a supervised worker it adds that `tern-kata worker stop` moves it to the installed version.

A worker the host half did not start is left running. This happens after a plugin reload (`tern plugin reload`, an install or update of the plugin, a change to a file in a checkout linked with `tern plugin link`; from Tern 0.9.16 changes under `.git/`, so `git status`, no longer reload it) or a Tern update loads the host half again while the worker runs on, after a daemon restart or `tern daemon stop` (Tern does not stop its children), and with a worker you started by hand. The host half logs one line naming that worker (`tern-kata: worker 1234 (tern-kata v0.7.2, started by Tern) is already running, so the plugin starts none until it exits`) and starts none of its own while it runs. To stop the worker as well, run `tern-kata worker stop` after `tern daemon stop`: run before it, the plugin would start a fresh worker.

When that worker ends, the host half starts a new one. On Tern 0.9.16 and later Tern tells it at once after a plugin reload or a Tern update, so the new worker starts within a second or two and the daemon log gets a line with the old worker's exit (`tern-kata: worker 1234 exited with status -1 (...); starting a new one`, or `timed out at the run limit`); the host half can name the worker only after it has logged the "already running" line. On older Tern, and after a daemon restart or for a worker you started by hand, it starts one within 10 seconds of its end and logs no exit.

### Turning supervision off

Create an empty `worker.manual` next to `worker.toml`, or press `s` on the Worker card, which does it for you. The host half then starts no worker and looks again every 10 seconds. Deleting `worker.toml` has the same effect. Use it to run the worker from a service of your own (a systemd user unit, a launchd agent).

The marker cannot stop a worker that already runs: create it first, or the host half starts a new one, then run `tern-kata worker stop`. The card writes the marker before the stop request and asks how the running worker stops (when idle or now). Removing the marker, or `s` again, turns supervision on, and the plugin starts the worker within 10 seconds. The card and the Jobs Page banner show the state.

`tern plugin remove tern-kata`, or disabling the plugin in Tern's settings, removes the host half altogether, together with the window half.

## Only when: conditions and skips

A job whose `options.conditions` caps its runs (`runs_per_day`, `runs_per_week`) or their spend (`spend_per_day`, `spend_per_week`; counts only the cost omp reports) starts a scheduled run only while the cap has room, and with `new_commits` only when its checkout has commits since the last run of the job that succeeded (the worker runs git in the job's checkout and gives the agent the commit list). When a due occurrence is over the limit the worker skips it: no kata Run, no Issue, no slot, and no catch-up later. `tern-kata worker status` lists each job's recent skips with the latest reason, and a job whose conditions cannot be checked as blocked. Only the worker that holds the Project's lease applies the conditions; a worker on another machine that does not know them runs the job anyway. See [Conditions and skips](worker-reference.md#conditions-and-skips).

## Worker widget

On Tern 0.9.17 and later the window offers a **kata worker** widget for the widget bar. It shows "N running" (or "idle") and, in the warning tone, "N held" while a run waits for you; the ring style fills with the share of the worker's slots in use. The tooltip names the worker and each held run with its reason. A click on a held count runs **Show kata's held runs**, which opens kata's Attention Page, or, while a card, a question, the help sheet or a search is open in the Pane, shows a toast asking you to close it instead of switching Pages (on the Pane's daemon error screen it only focuses the Pane); otherwise it opens kata. Place it with the palette command **Add kata widgets to the widget bar** or from Edit widgets....

The widget shows the worker of the machine the window runs on. A remote window therefore does not show the worker of the session host. The widget shows nothing while no worker runs on that machine. It also shows nothing for a worker the plugin did not start (one you started by hand, or one left from an earlier daemon) while `worker.manual` or a missing `worker.toml` turns supervision off; a worker the plugin started stays shown until it exits. It hides a summary that no Tern daemon on that machine has refreshed for a minute, so a worker stopped while no daemon runs there does not linger.

## Commands

```sh
tern-kata worker doctor      # checks the file, checkouts, commands, secrets, kata and each Project's lease; says how to fix them
tern-kata worker status      # what it serves, runs and refuses; held runs with their reasons; each job's recent skips
tern-kata worker             # runs in the foreground; ctrl+c stops it (a supervised worker needs no start)
tern-kata worker stop [--now]
tern-kata run <job-uid>      # run an adapter prompt job now
tern-kata worker resume <run-uid>    # continue a held run in its omp session
tern-kata worker resume <job>        # lift the pause after repeated failed runs (job uid or name; `worker status` names the paused jobs)
tern-kata worker abandon <run-uid>   # give up a held run (kata records it cancelled), stop a running one, or settle one whose local state is lost
```

`tern-kata worker --takeover` moves a Project's lease on purpose (see [state and leases](worker-reference.md#state-and-leases)). `tern-kata --help` lists every command.

See also [Jobs](jobs.md) for making jobs, [Runs](runs.md) for watching them, and the [worker reference](worker-reference.md) for held runs, recovery and the complete `worker.toml` key list.
