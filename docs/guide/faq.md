# FAQ and known issues

## Known issues

**Windows: each new kata Pane takes a moment to open.** Tern starts a Pane's command through PowerShell and can't yet pass `-NoProfile`, so every new kata Pane loads your PowerShell profile before tern-kata starts; a large profile makes the wait longer. Once open, the Pane runs at full speed. A fix has been requested from the Tern developers.

**Open kata, `F9` and kata links open a tab instead of a split in newer Panes.** Tern 0.9.4 gives window plugins Pane ids without the bits above 2^44, so the plugin can't split a Pane whose id is that large and opens tern-kata in a new tab in the focused Pane's directory instead. For the same reason a kata link never reuses an open kata Pane or selects its Issue, and a failed start shows no toast. It needs a fix in Tern.

## Questions

### What do I need installed?

kata v0.20.0 or newer and a Tern that supports the Surface Protocol. For the worker, also omp, and on Windows Git for Windows. See [Install](../../README.md#install).

### Which machine gets which part?

kata and tern-kata go on the session host, the machine whose shells run in your Panes. The window machine, the one showing the Tern window, needs only the window plugin. If both are one machine, install everything there.

### What does tern-kata open in a directory with no `.kata.toml`?

Inbox beside the Menu sidebar. A directory is bound to a Project when it, or a parent up to the enclosing git root, contains a `.kata.toml`, which `kata init` creates. See [How it works](pages-and-board.md#how-it-works).

### Why does the Board, Jobs, Runs or Workflows Page say it needs a Project?

Those Pages act on the Pane's Project: the current Project Collection, else the workspace's Binding. With neither, the Page says so and opens the Menu's Projects list. See [The Page bar](pages-and-board.md#the-page-bar).

### Why did moving a card change a label in kata?

A Lane is one kata label, so `<` and `>` swap labels. Moving into No lane removes the Lane labels and keeps every other label, and a Move never enters or leaves Done. See [Moving around and moving cards](pages-and-board.md#moving-around-and-moving-cards).

### Does editing the Lanes in the Lanes card change my Issues?

No. Saving writes the Lane list file only and never changes a label in kata. See [The Lanes card](pages-and-board.md#the-lanes-card).

### Where does the worker's configuration live, and how do I create it?

In `worker.toml` in tern-kata's config folder. Press `W` on the Jobs Page and choose **Set up** (or **Serve this Project** for a further Project), or run `tern-kata worker init` in a directory bound to a Project and `tern-kata worker add` in each other Project's directory. See [Set it up](worker.md#set-it-up).

### I added a Project (or edited `worker.toml`) and nothing changed. Why?

A running worker reads `worker.toml` only at start. Answer the Worker card's restart choice, or run `tern-kata worker stop`; the plugin then starts a fresh worker. See [Restarting and stopping](worker.md#restarting-and-stopping).

### Is it safe to list a Project in `worker.toml`?

Only if you trust everyone who can write jobs in it. They can run prompts and commands on your machine as your user, unattended, in the checkout the job names. There is no sandbox. See [Trust](worker.md#trust).

### No worker runs. Why?

The host half starts the worker when `worker.toml` exists, inside Tern's session daemon. With Tern not running, no worker runs. A `worker.manual` file beside `worker.toml` turns supervision off; `s` on the Worker card (`W` on the Jobs Page) shows and changes that. `tern-kata worker doctor` checks the setup, and the Jobs Page says whether this machine's worker serves the Project. See [Supervision](worker.md#supervision).

### A run is stuck as `unknown` or `held`. What do I do?

The worker could not prove what the agent did, and never sends the prompt again by itself. `tern-kata worker status` shows the reason and how to settle it: resume it in its omp session, or abandon it. See [Recovery and held runs](worker-reference.md#recovery-and-held-runs).

### Why does `R` or `u` on the Jobs or Runs Page refuse?

The refusal's reason comes from the worker: no worker runs on this machine, it does not serve the Project, another machine holds the Project's lease, the Project has no free slot, or the run is not held. See [Run now](jobs.md#run-now) and [Runs of this machine](runs.md#runs-of-this-machine).

### Why do kata links show as plain text in chat?

Tern draws `kata://` links as plain text, so agents write `https://kata.invalid/...` links, which the window plugin intercepts. Add the rule from [Agents](agents.md#clickable-ticket-links-from-agents) to your agent's global rules.

### Can an agent see an Attachment I added on another machine?

Only through the pointer in the Issue's kata metadata, which names the host and path. A teammate's agent can open it only if it can SSH to that host. See [Hosts and remote daemons](attachments.md#hosts-and-remote-daemons).
