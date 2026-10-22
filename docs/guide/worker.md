# The worker

kata stores jobs that run an agent prompt on a schedule, and the evidence of each run, but runs none of them. `tern-kata worker` is the adapter that does, on your machine, with no Tern tab or window open.

What it does:

- runs prompt jobs, and the steps of workflow jobs, through [omp](https://github.com/can1357/oh-my-pi) headless in the job's checkout with the job's agent profile (command steps through your shell);
- creates each run's Issue, or comments on the job's existing one;
- records every run in kata as running, then succeeded or failed with a summary and token counts.

What it never does:

- run native jobs (a recurring child kata's daemon delivers);
- send a prompt twice: a run whose outcome is uncertain after a crash is held.

It needs kata v0.19.0 or newer and `omp` on `PATH`. On Windows, workflow command steps also need `sh`, which [Git for Windows](https://gitforwindows.org/) provides.

## Trust

> **Trust.** Anyone who can write jobs in a Project you list in `worker.toml` can run prompts and commands on this machine, as your user, with your files and credentials, and the agent runs unattended (`--approval-mode yolo`) in the checkout the job names. There are no command allowlists and no sandbox. List only Projects whose writers you trust, and keep secrets out of the checkouts a job can name.

## Set it up

`worker.toml` lives in tern-kata's config folder: `~/.config/tern-kata/worker.toml` on Linux (honouring `$XDG_CONFIG_HOME`, beside `lanes.toml`) and `%AppData%\tern-kata\worker.toml` on Windows. Two commands write it for you. Run both in a directory bound to a kata Project, the same Binding the Pane uses (a `.kata.toml` in the directory or a parent up to the git root; `kata init` creates one). The directory is the Project's checkout. Neither command calls the kata daemon.

### `tern-kata worker init`

Writes a new `worker.toml`. It refuses when the file already exists (use `worker add`) or when the directory is not bound (run `kata init`). The file holds, in this order, with short comments:

1. a header comment with the trust note;
2. `executor = "<hostname>"`, the machine's hostname lowercased with every character outside `a-z 0-9 _ -` replaced by `-`;
3. a commented-out `# actor = "..."` showing the default the worker would use;
4. a commented `[limits]` example;
5. a `[[projects]]` block for this Project with `capacity = 1` and `checkouts = ["<key>"]`, so the Project's jobs may use only its own checkout;
6. `[checkouts]` with `<key> = "<absolute checkout>"`;
7. `[profiles.default]` with `command = "omp"` and commented `model`, `thinking` and `args` examples;
8. a commented `[secrets]` example.

When it finishes it prints the path written and what was added, one line of the trust note, that the plugin's host half starts the worker within 10 seconds (else run `tern-kata worker`), and that `tern-kata worker doctor` checks the setup.

### `tern-kata worker add`

Adds the current Project to an existing `worker.toml`. It refuses when there is no file (run `tern-kata worker init`), when the directory is not bound, or when the Project is already listed (the message names it). It:

- appends a `[[projects]]` block for the Project, with the same three keys as `init` writes;
- puts the checkout line inside the existing `[checkouts]` table, creating the table at the end when absent;
- leaves every other line and comment byte for byte unchanged.

A running worker reads `worker.toml` only at start. After `worker add`, end it (`tern-kata worker status` prints its process id; `kill <id>`, or `Stop-Process -Id <id>` on Windows) and the plugin starts a fresh one.

### The checkout key

Both commands name the checkout after the Project: the Project name lowercased, every character outside `a-z 0-9 _ -` replaced by `-`. If `[checkouts]` already has that key for another directory, `-2`, `-3` and so on is appended; the same key and directory is reused.

### Safe writes

Both commands write through a temporary file in the same folder, check it with the worker's own strict loader, then rename it into place with mode 0600. A file that would not load is never written, and the error says why.

## A two-Project `worker.toml`

After `worker init` in the `acme-shop` checkout and `worker add` in the `acme-infra` checkout, the file reads (comments abridged):

```toml
# Anyone who can write jobs in a listed Project can run prompts and commands
# on this machine as you.
executor = "laptop"
# actor = "dangerboi"

[[projects]]
name      = "acme-shop"
capacity  = 1
checkouts = ["acme-shop"]

[checkouts]
acme-shop = "/home/me/Code/acme-shop"
acme-infra = "/home/me/Code/acme-infra"

[profiles.default]
command = "omp"

[[projects]]
name      = "acme-infra"
capacity  = 1
checkouts = ["acme-infra"]
```

Each `[[projects]]` block lists its own checkout only, so a job in `acme-shop` cannot name the `acme-infra` directory. `worker add` appends its `[[projects]]` block at the end of the file; TOML lets the array of tables sit after other tables, and the `[checkouts]` line is added to the existing table. Every key is described in the [worker reference](worker-reference.md#workertoml).

## Commands

```sh
tern-kata worker doctor     # checks the file, checkouts, commands, secrets, kata and each Project's lease; says how to fix them
tern-kata worker            # runs in the foreground; ctrl+c stops it
tern-kata worker status     # from another shell: what it serves, runs and refuses
tern-kata run <job-uid>     # run an adapter prompt job now
tern-kata worker resume <run-uid>    # continue a held run in its omp session
tern-kata worker abandon <run-uid>   # give up a held run (kata records it cancelled), stop a running one, or settle one whose local state is lost
```

`tern-kata worker --takeover` moves a Project's lease on purpose (see [state and leases](worker-reference.md#state-and-leases)). `tern-kata --help` lists every command.

## Supervision

The plugin's host half starts `tern-kata worker` when `worker.toml` exists and keeps it running. It runs in Tern's session daemon, so it needs no window.

- **Restarts.** It starts the worker again whenever it exits, after 1 second, then 2, 4 and so on up to a minute between tries. A worker that stayed up for a minute starts the count over.
- **When it starts.** When Tern's daemon starts, which a window does on demand and Tern's "Start at login" setting does at login, the host half starts the worker with it. Nothing starts the daemon at boot or after a quit by itself: with Tern not running, no worker runs.
- **Finding the program.** The host half finds `tern-kata` on the daemon's `PATH` or in the folder the install scripts use (`~/.local/bin`, `%LOCALAPPDATA%\Programs\tern-kata`, `$TERN_KATA_INSTALL_DIR`). It gives the worker the daemon's `PATH` plus those folders, Tern's bundled omp, `~/go/bin` and Homebrew's, so `omp` resolves even when Tern was started from a launcher with a short `PATH`.
- **Logs.** Its log lines (`tern-kata: worker started`, `the worker exited with status ...`) are in Tern's daemon log. `tern-kata worker status` says whether Tern supervises the worker or you started it by hand.

A worker the daemon left running when it stopped keeps running (Tern does not stop its children), so after a daemon restart the host half's new attempts are refused by `worker.lock` until it ends, and then the next attempt starts a worker.

This is the one supervision path tern-kata ships. A service of your own (a systemd user unit, a launchd agent) also works: create an empty `worker.manual` beside `worker.toml`, and the host half never starts a worker.

## Turning supervision off

Create an empty `worker.manual` next to `worker.toml` (or delete `worker.toml`): the host half starts no worker, and looks again every 10 seconds. It cannot stop a worker that already runs: end that one yourself, with the process id `worker status` prints (`kill <id>`, or `Stop-Process -Id <id>` on Windows). Create the marker first, or the host half starts a new one. Removing the marker turns supervision back on.

`tern plugin remove tern-kata` or disabling the plugin in Tern's settings removes the host half altogether, together with the window half.

See also [Jobs](jobs.md) for making jobs, [Runs](runs.md) for watching them, and the [worker reference](worker-reference.md) for held runs, recovery and the complete `worker.toml` key list.
