# Changelog

All notable changes to tern-kata are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [semantic versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Agents

- The Agent badge appends " · message unread" in the warning tone, and the Agent card row shows "message unread: " and the message's first line, while an agent has a message at its address that it has not cleared.
- An omp agent is told of a person's comment on its Issue when `forward_comments = true` is set in `agent.toml`.
  - The text is "New comment on <ref> from <author>: <first lines> (c:<comment UID>)", sent once per comment, with the Pane closed.
  - `forward_comments` goes at the top level or per Project; comments from agents and from before the agent started are skipped.
- `m` on the Agent card messages the selected agent, whether it is working, idle or waiting and wherever its block is, including a Background agent, which stays in the hidden session.
  - It is off, with the reason, for an exited agent, one that is not omp, one started before agents had an address, and one another actor started.
- `m` on the Comments tab asks the Issue's agent to answer the comment under the cursor.
  - The Message card names the comment, and the message carries the comment.
  - The agent is told to answer with `kata comment --reply`, `--confirm`, `--refute` or `--supersede`, which clears the request and leaves a reply notice on the Attention Page.
- The Agent card shows, for the selected omp agent, its model, tool calls so far, the tool it is running now, its cost so far and the first line of its last message.
  - It reads them from the end of the agent's session file while the card is open, every 2 seconds.
  - A non-omp agent or a session with no file says why in a muted line.
  - A session file longer than the part read (1 MiB) marks the count "at least" and the cost approximate (`~$`).
- `i` on the Agent card interrupts the selected agent's current turn and keeps its block.
- `x` on the Agent card stops the selected agent after asking: it closes the block, removes its entry and prompt file, and gives the Issue the hand-off an exiting agent gets.
  - While the Issue is open, the Pane that stopped the agent also sets `work.attention` to `needs-human` with a reason in `work.attention_msg` and swaps `ready-for-agent` for `ready-for-human` when the Issue has it, since a stopped agent cannot write them itself.
- `r` on the Attention Page and on the Agent card opens a picker of three quick replies to the agent, sent with `1`, `2` or `3` as a typed message would be.
  - It has the same refusals as `m` and asks the same question before replacing an unread message.
  - The replies are "continue", "wrap up: commit what you have and comment your status" and "commit and close if the acceptance criteria hold".
- An omp agent started in a block writes to kata as its own teammate: the wrapper sets `KATA_TEAMMATE` to `<agent.toml name>-<block id>` and drops an inherited one, so the agent's comments and requests carry that name.
- An omp agent gets a Blocker notice when an Issue that blocks its own closes, also with the Pane closed.
  - The notice has the blocker's title and how many other blockers are still open.
  - Only blocking links count, and only blockers in the agent's own Project; a close before the agent started is not sent.
- `S` on an Issue messages every running agent on it and on its open children from one Message card.
  - Each agent gets the message at its own address on its own Issue; agents on closed children and exited agents get none.
  - An agent that has not cleared its last message is skipped, never replaced; once every write has answered the card lists how many got it and each skipped agent with its reason.
- An agent can take messages from more actors than its own: `message_from` in `agent.toml` (top level or per Project) lists them.
  - The Pane passes `--message-from <actor>` to `tern-kata agent`, which sets `TERN_KATA_MESSAGE_FROM`; the agent's first message names every such actor.
  - A message from an actor other than the agent's own needs kata's identity mode (`[auth] require_token_identity`); the Message card and `tern-kata message` say so when it is off. `tern-kata agent` checks the daemon at start and sets `TERN_KATA_VERIFIED_SENDERS=1`.
- A team starts from the parent Issue's Agent card (`c`): a coordinator agent plus Background agents on its ready children, at most 1–8 running at once (default 2).
  - The next ready child starts when a running one exits, from any Pane open on the host; when no child is left to start and none runs, the coordinator gets one message naming the children closed and still open.
  - The parent's Agent card shows the team: each child's agent and state, the queued and skipped children with the reason, the limit and the team's state; `p` pauses or resumes the queue, and a child's card names its parent and coordinator.
  - The coordinator's prompt names its children, `tern-kata team`, `tern-kata message --re`, its authority over its own children only, the shared checkout, the events it hears and how the team ends; a child's prompt names its parent and coordinator and tells it to report and ask by comment on its own Issue.
- A team's child takes its coordinator's messages, and its coordinator's comments on the child's Issue whatever `forward_comments` says, as the person's authority over that Issue.
- A coordinator hears its children's comments, closes, and `work.attention` turning `needs-human` or `stuck`, from when it started, including children added later.
- Each agent a Pane starts publishes an Agent record on its Issue, so Panes on other machines can find it.
  - A Host id, made once in tern-kata's state folder, names the machine in the record; `host` at the top level of `agent.toml` sets the name other machines show (default the machine's host name).
  - The record goes when the agent exits, its block closes, it is stopped or its Background run ends; a Pane at start writes the record of an agent that has none and removes its own host's records that no agent matches.
  - An agent whose address another machine already holds publishes no record and stays local; its Agent card row says which host uses the address.
- The Agent card lists agents running on other machines after this host's own, as "on <host> · started <time>", read from their Agent records; `f` forgets one, after a question, and clears any message left at its address.
  - The Agent badge counts them too; an Issue with only remote agents reads "agent on <host>" or "agents on N hosts".
  - `m` and `r` reach them from their Agent card row and, for a request no local agent raised, from the Attention Page (the newest agent on that Issue with the request's name); `S` and the wrap-up include them, the wrap-up listing each as "on <host>: finished" or "on <host>: not yet". A daemon without identity mode, or an actor the agent does not list, is refused with the reason.

#### Issues and Pages

- `u` on the Attention Page restores the Issue the Pane just deleted, as `u` does on the other Pages, and the Page reads its requests again; it is offered there only while a delete is the newest change in the undo history.
- `n` on the Runs Page switches to the since-you-last-looked mode: the runs that ended since you last marked them seen with `s` on this machine, newest first above a Now line, then the Project's next runs (native jobs with their next time, adapter jobs listed with no time).
  - More than eight new runs fold into one row per job; failed runs are never folded.
  - The mark and the mode are kept per Project in `seen.toml` beside `filters.toml`, so they survive a Pane restart.
- The Runs Page lists a job's skipped occurrences as one row (count, latest condition and reason, when) from this machine's worker status, narrowed by the job filter.
- The Job card shows an adapter job's conditions read-only, or the reader's error when they do not parse.
- `K` opens the Config Page, a Lettered Page (also in the Menu) that lists tern-kata's config files with their paths, whether each exists and loads, and a one-line summary.
  - The rows are `lanes.toml`, `filters.toml`, `views.toml`, `seen.toml`, `worker.toml` (with its supervision), `agent.toml` (with its actor, `host`, name, command and `message_from` actors), Docbank supervision and kata's own `config.toml`, read-only; each is read by the reader its feature uses, so a file that does not load shows that reader's error on its own row.
  - `enter` on a row opens the card or Page that edits the file: the Lanes card on the Board Page, the Filter card, the Menu's Views level, the Runs Page for `seen.toml`, the Worker card on the Jobs Page, and the Daemons Page for kata's `config.toml`.
  - The Docbank supervision row says whether the plugin keeps a local Docbank running, else why not, and `s` turns supervision off or on as `tern-kata docbank supervise off|on` does.
  - `e` on the `agent.toml` row edits it in `$VISUAL` or `$EDITOR`, starting from a commented starter that shows every key when there is no file; the text is checked with the rules an agent start uses and written only if it loads and the file did not change while you edited.

#### Jobs and worker

- The worker pauses an adapter job after repeated failed runs.
  - After `pause_after_failures` runs of one job in a row recorded `failed` (default 3, set under `[limits]` in `worker.toml`, 0 turns pausing off) the worker admits no more of its runs until you resume it; timeouts count, held runs and cancelled runs don't, and a succeeded run starts the count again.
  - The pause lives in the worker's own state, so kata's job is not changed and other machines keep their own counts. `tern-kata worker status` and the Jobs Page name it ("paused: failed runs"); `u` on the Jobs Page, or `tern-kata worker resume <job>` on a host with no Pane, resumes the job.
- A job's `options.conditions` say when a scheduled run may start, and the worker skips a due run that fails one.
  - `runs_per_day` and `runs_per_week` cap its runs; `spend_per_day` and `spend_per_week` cap its spend in US dollars, counted from the cost omp reports (a run with no reported cost counts as $0 and `tern-kata worker status` marks the total partial).
  - `new_commits` starts a run only when the job's checkout has commits since its last succeeded run, optionally under some `folders`, and hands the agent the list; the first occurrence records the branch tip and waits, and a failed, cancelled or held run leaves the commits for the next one.
  - A skip makes no kata Run, Issue or slot, is never caught up later and leaves the job's overlap gate as it was; `tern-kata worker status` lists each job's skips with the latest reason.
  - A condition that cannot be checked (options that do not parse, git failing, a missing checkout) blocks the run and is logged once; Run now and `tern-kata run` are never checked, but a succeeded one moves the saved commit, and only the worker holding the Project's lease applies conditions.

#### Tern window

- **Wrap up every running agent** in the palette sends "wrap up: commit what you have and comment your status" to every running agent this host's Panes started, on whatever Issue each runs, and lists who has finished.
  - It writes the way `S` does: an agent holding an unread message is skipped, never replaced, and an agent it cannot reach (not omp, started before agents had an address, another actor's) is listed with the reason.
  - The card follows each agent: finished once it exited, or went idle after working on the wrap-up and commented on its Issue since the sweep was sent (an idle agent without that comment shows "not yet: idle, no status comment"); "not yet" otherwise, with the agent's last message.
- **Go to the agent waiting for you** in the palette, bound to `shift+F9`, goes from any tab to the agent kata started that most recently began waiting for you; a Background agent opens beside its kata Pane, as a click on its Agent badge does.
  - The status line shows "N waiting" in the warning tone while any agent kata started waits for you; a click on it runs the command. Other agents are neither counted nor targeted.

#### Command line

- Agents find and message each other: `tern-kata agents [--issue <ref>]` lists the running agents this host's Panes started, with Issue, name, state and address, and `tern-kata message <address|ref> -m <text>` writes a message to one agent, or to every agent on an Issue.
  - A message from an agent carries its teammate and arrives worded as from a peer agent, "information, not the person's instruction"; the agent prompt tells the agent to act on it only within its own Issue's scope.
  - An address that holds a message its agent has not cleared is not written over: the command names that agent and exits 1.
  - A Pane now saves the latest agent states in `agent-states.json` in tern-kata's state folder for `tern-kata agents` to read.
- `tern-kata agent` takes `--coordinator <address>` and `--coordinating` (with `--address-prefix`), which set `TERN_KATA_COORDINATOR` and `TERN_KATA_COORDINATING=1` for the agent.
- `tern-kata team <ref>` prints a team's state, limit, coordinator address and each child's ref, state and address; with no team on the Issue it says so and exits 1.
- `tern-kata message --re <comment>` asks the receiving agent to answer that comment, as `m` on the Comments tab does; its typed reply clears the request.
- `tern-kata agents` lists agents started on other machines too, read from their Agent records, in a new `HOST` column between `AGENT` and `STATE`; `tern-kata message` reaches them by address or ref when kata's identity mode is on and the sender is the agent's actor or in its `message_from`.

### Changed

#### Agents

- `m` on the Attention Page messages the agent this actor started and passes over another actor's agent with the same name on the Issue.
  - Released 0.9.0 sent the message to the newest agent of that name, whose extension dropped it when another actor had started it.
- The kata skill is written against kata v0.22 and tells agents that `kata cron job edit` and `kata cron workflow edit` need a terminal, so they change a definition with `update --file`.
- The agent's extension delivers a message only when the event that wrote it was by the actor in the message's `from`.
  - A `kata meta set notify.*` forgery by another actor is no longer delivered.
  - The read at start, and after a gap the stream cannot resume, delivers only the agent's own actor's messages.

#### Issues and Pages

- The Page bar draws the Attention count as a badge after the name, not as text in it, and the compact Page bar keeps the badge.
- The Delete card also takes kata tui's confirmation, `DELETE <project>#<id>`, besides `<project>#<id>`; anything else is refused with both forms named.
- An account whose credential is limited to one Issue is no longer offered `X` (the Delete card) or the Menu's "Restore a deleted Issue", both of which kata refuses to it; the Pane asks the daemon's auth capabilities when it connects and after a daemon switch, and offers both until the answer is in.

### Fixed

#### Issues and Pages

- The Delete card's reason is no longer lost: kata's delete records none, so the Pane adds it to the Issue as a `Deleted: <reason>` comment just before the delete, and deletes nothing when that comment fails.

#### Jobs and worker

- On Windows, a worker whose runs folder is a file instead of a folder refuses work and says why, as on Linux and macOS; it used to read the file as no runs and go on.

#### Tern window

- On Windows, the programs tern-kata runs in the background (`tern` opening a run's session tab, `git config user.name`, docbank setup, the update steps and the file opener) start without a console window, so none flashes up.

## [0.9.0] - 2026-11-06

### Added

#### Attachments

- `tern-kata docbank setup` makes attachments work, either on this machine or against a [Docbank](https://github.com/kenn-io/docbank) someone else runs.
  - Without `--url` it sets Docbank up on this machine: it appends a `[server]` section (`bind_addr` 127.0.0.1, a fixed `api_port` from `--port`, default 8080, a new random `api_key`) to `<DOCBANK_HOME>/config.toml`, stores the key in `<KATA_HOME>/docbank.key` and restarts Docbank.
  - It requires `docbank daemon status` to name the address before touching kata's config, and prints that Docbank reports usage unless `DOCBANK_TELEMETRY_ENABLED=0` is set in its daemon's environment.
  - An existing `[server]` with `api_port` and `api_key` is reused, and so is an existing key file, tightened to mode 0600 when others could read it. A different `--port`, a `[server]` missing one of them, a busy port, a missing `docbank`, a different `[docbank]` section or a `config.toml` symlink whose target is missing is refused before anything is written.
  - A `config.toml` that changes while setup runs is not overwritten: setup stops and asks to be run again.
  - With `--url <url> --key-file <file>` (or `--key-env <NAME>`) it points kata at a Docbank someone else runs; `--key-file -` reads the key from standard input. No flag takes the key and the command never prints it.
  - Either way it checks that Docbank answers with the key, appends a `[docbank]` section to the kata daemon's `config.toml`, and asks before restarting the kata daemon (`--yes` skips the question, `--no-restart` prints the command).
  - It finishes by checking that the kata daemon has attachments on; with `--key-env` it says that daemon reads the variable from its own environment.
  - A rerun on a finished setup says "already set up" and changes nothing; if the earlier restart was skipped, it offers the restart again.
  - `--folder` and the route flags `tern-kata agent` takes choose the vault folder and the kata daemon; plain `http://` to another machine and a remote kata daemon are refused.
- The plugin keeps a local Docbank running while Tern runs, and `tern-kata docbank supervise on|off` turns that on or off.
  - Its host half runs `docbank daemon start` when no Docbank runs, checking every 10 seconds. After a failed start it waits 1 s, then 2 s, doubling to 60 s, and logs one line per start or failed start.
  - It does so when kata's `[docbank]` url is on this machine, Docbank's `[server]` `api_port` is that port and tern-kata's config folder has no `docbank.manual`; `tern-kata docbank supervised` answers whether that holds.
  - `supervise off` creates `docbank.manual` and `on` removes it. Only the default kata daemon's Docbank is kept running; for a profile, local setup says how to keep it running.

### Changed

#### Issues and Pages

- The Page bar draws each Page's key as a keycap, like the footer's key hints: `[1] Issues  [2] Jobs  …  [6] Attention`.
  - A Lettered Page shown in the bar draws its letter (`[C] Credentials`). The current Page keeps its accent name, the compact bar still shows the keycap alone for the other Pages, and a click on an entry still presses its key.

### Fixed

#### Install and update

- `tern-kata update` no longer replaces a plugin that is newer than the binary with the binary's older release.
- On Windows, `tern-kata update` removes the old binaries it set aside also when the install folder's path holds `[`.

#### Issues and Pages

- The help sheet lists the `/` search lines of the Issues, Board, Jobs, Runs and Workflows Pages again.
- A Project title of wide characters (Chinese, Japanese, emoji) no longer pushes the Menu button off the top bar.
- A view saved and then cancelled on the View card says whether the save went through, instead of nothing.
- On Windows, saving views, Lanes, agents, `worker.toml` and other tern-kata files retries for up to a second while another program has the file open, instead of failing.

#### Jobs and worker

- The Workflows Page shows a refused workflow save or archive also when the Project changed before the answer came.
- A job whose `adapter` option has spaces around `tern-kata` runs, and one whose option is blank runs on any adapter, instead of never.
- The worker no longer crashes when a notify run's journal cannot be read while it looks up whom to tell.
- Run details of a damaged journal that lists more steps than its workflow no longer crash the worker.
- On Windows, command steps get no `KATA_` or `TERN_KATA_` variable in any letter case, and a secret can't be mapped to one.
- On Windows, `tern-kata worker edit` opens `worker.toml` in your editor also when the temporary folder's path holds `%`.
- On macOS, the worker checks a recorded agent's start time as it does on Linux, so after a crash or reboot it no longer signals another process that reused the agent's process id.

#### Daemons and Federation

- Switching to a remote daemon while kata's `config.toml` can't be read reports that error, instead of connecting through kata's own lookup with `KATA_AUTH_TOKEN`.
- A wrong value under `[tui]` in kata's `config.toml` no longer hides the daemon list: the Pane ignores it and keeps the default, as kata does.
- Pressing esc while a spoke leaves no longer hides a leave that went through: the list refreshes when it ends.
- Choosing another hub while enrolling a Project no longer keeps the actor the previous hub reported.

#### Command line

- `tern-kata agent` exits 1, not 3, when it refuses the daemon or route.
- `tern-kata recur --start` with spaces around the date picks the date's weekday or day as the default, as without spaces.
- Text printed to the terminal shows invalid UTF-8 bytes as `�`, so a stray byte can't start an escape sequence.

## [0.8.0] - 2026-11-05

### Added

#### Agents

- `m` on the Attention Page sends a message to the omp agent that raised the selected request.
  - It reaches the agent at once, through an omp extension tern-kata loads into every omp agent it starts. A waiting or idle agent starts a turn on it, a working one gets it at its next step without being stopped, and a pending `ask` or tool approval stays until you answer it.
  - Each omp agent has its own Agent address, `<actor>/<name>-<block id>`, set as its `KATA_INBOX_USER`. Only messages from you are delivered, and the agent's first message tells it to act only on those.
  - The Message card asks before replacing a message the agent has not cleared, and says why when the agent cannot receive one (exited, not omp, or started by an older tern-kata).
  - When an agent exits, the Pane clears what is left at its Agent address. Agents that load the extension run with `KATA_OMP_LIVE_INBOX=0`, so kata-omp leaves live delivery to it.
- A `kata close` run by an omp agent the Pane started, or by a worker run whose profile `command` is omp, names that agent's omp session.
  - With `[close.transcript]` enabled in kata's config the close records the session; with `agentsview_url` it also records a link to it in AgentsView.
  - Worker runs now load tern-kata's omp extension for this, also under `--no-extensions`.

#### Attachments

- The Close card for a `done` close can cite one or more of the Issue's Attachments as evidence.
  - The Events tab names a cited Attachment, marks it `[removed]` once it is unlinked and `[changed]` when the Issue now holds another version of it.
- `tern-kata migrate-attachments` moves the old attachments stored on this machine to [Docbank](https://github.com/kenn-io/docbank), deletes the local copies, and lists the ones held on other machines.
  - It leaves a file, an Issue folder or the store that is a symlink, and lists it with its reason, so it never uploads what a link points to.

#### Issues and Pages

- `/` on the Jobs, Runs and Workflows Pages searches their rows by a case-insensitive part of the row's text, as kata tui does.
  - The search shows above the list, `esc` ends it, and an empty result says so.
- Issue list rows show the Date badge of an open Issue with a scheduled date or deadline, after the title, as kata tui does.
  - A row that can't fit its whole title and the badge drops the badge.
- The Dates card and the Create card take kata's date-time forms, `YYYY-MM-DDTHH:MM[:SS]` and `YYYY-MM-DDTHH:MM:SSZ`; the preview shows the time.

#### Jobs and worker

- The Jobs Page inspector lists a job's next three runs with their time zone abbreviations.
  - It says why there are none when the job is paused, its parent is closed, it only runs when started, or its runs can't be worked out.
- `tern-kata worker status` names the installed binary's version when the running worker runs another one, and says how `tern-kata worker stop` moves it there.

#### Daemons and Federation

- `N` on the Federation Page names a spoke.
  - On the enrollment preview it sets the name the hub records with the enrollment; on a spoke, in the list or its detail, it renames it on the hub, and an empty name clears it. The name shows on the spoke's card and detail.
  - The card refuses a name that breaks kata's rule, in kata's words, before anything reaches the hub, and shows the hub's reason when it refuses a name another spoke holds.

### Changed

#### Requirements

- **Breaking:** tern-kata requires kata v0.21.0 or newer.
  - kata v0.21 upgrades its database to schema 43 on first start, which older kata binaries cannot open; kata keeps a backup from before the upgrade.
  - A PostgreSQL store needs `kata storage postgres migrate` with the daemon stopped.

#### Agents

- The kata skill is written for kata v0.21.
  - It starts, changes and stops a recurrence with `kata schedule --repeat`, moves an Issue with its open descendants, and closes with Docbank or attachment evidence.

#### Attachments

- **Breaking:** Attachments are kata's own, with their bytes in [Docbank](https://github.com/kenn-io/docbank).
  - `@` uploads a file, the Attachments tab lists each with its media type and size, and `U` unlinks one (its bytes stay in Docbank).
  - `enter` downloads one to a cache folder and opens it when it is an image, video, audio, PDF or text file; anything else is only saved, and the Pane says where.
  - The daemon needs a `[docbank]` section in kata's `config.toml`; a daemon without one says so plainly in the Pane.

#### Issues and Pages

- The Pages are numbered as in kata tui: `1` Issues, `2` Jobs, `3` Runs, `4` Workflows, `5` Board and `6` Attention.
  - `b` still toggles the Board and `T` still opens Attention.
- The Move card moves an Issue with its open descendants, as kata does.
  - The preview names the Issues that move, the **Only this Issue** switch (`o`, or a click) moves the Issue alone, and the Notice says how many Issues moved.
  - When kata refuses because of a descendant (archived, or held back for another reason), the card names it and keeps **Only this Issue** within reach; a subtree that changed during the move shows in words too.
- The Comments tab shows a discussion as a reply tree.
  - Replies nest under the comment they answer, four levels deep at most, and a reply from another Issue is a line under its target.
  - Confirm and refute show as tags, reading "(before edit)" when the evidence predates an edit of the comment.
- When an Issue's date changed elsewhere since the Dates card opened, the card writes none of your changes, refills that field with the current value and keeps your other edits; it used to ask you to close and reopen it.
  - A date that changes elsewhere during the save itself stops it there, and the card names the fields already saved.

#### Jobs and worker

- **Breaking:** an Agent profile with no `extensions` key now runs omp with `--no-extensions`, where it used to load whatever omp discovers.
  - A worker run no longer loads omp's native extension modules, ambient hook files or installed plugins such as kata-plugin. Explicit `-e`, `--hook` and `--trusted-extension` paths in its `args`, skills, rules and MCP servers still apply.
  - Add `extensions = "ambient"` under the profile to get the old behaviour back. A run already in progress when you upgrade uses the new default from its next attempt or step.
- `worker doctor` warns of kata-plugin's todo takeover only for an Agent profile that sets `extensions = "ambient"`.

#### Daemons and Federation

- Switching to a stopped local daemon on the Daemons Page starts it through kata's client, with the Pane's own environment: `KATA_DSN`, `KATA_DB` and `KATA_SERVER` are no longer removed first.
  - A switch to a remote entry still sends only that entry's own token.
- When the daemon doesn't know the Project a folder is bound to, the message names that Project, or the alias kata matched.

### Removed

#### Attachments

- `tern-kata attach` and `tern-kata detach`: use `kata docbank attachments add`, `ls`, `get` and `rm`.
- The `.kata-attachments` folder store and its pointer keys (`tern-kata.attachment.<name>`, and from 0.3 `attachment.<name>` holding `{host, path}`).
  - `tern-kata migrate-attachments` moves what they hold.

### Fixed

#### Agents

- When an agent's block starts but the Pane cannot record it, the Pane closes the block and says the agent was stopped.
  - It used to leave the agent running with no Agent badge and invite a retry that started a second one. If the block cannot be closed, the error names it.

#### Issues and Pages

- `u` shows the outcome of an undo, and takes the undone change out of the Undo history, when another change landed or the history was cleared while the undo was out.
  - Before, the answer was dropped without a word and the change stayed in the history.

#### Jobs and worker

- A cron expression whose `CRON_TZ=` or `TZ=` zone is quoted, such as `CRON_TZ="Europe/Berlin"`, is read with that zone.
  - The worker no longer sets the job aside as naming another zone than its trigger's, and the Jobs Page shows its runs.
- Supervision no longer starts a second worker for `worker.lock` to refuse, all day long, after a plugin reload or a Tern update.
  - The host half leaves the worker that is still running alone, logs one line naming it, and starts a worker within 10 seconds of its end.

#### Command line

- Fish completion offers all of `tern-kata agent`'s flags, as bash and zsh do, not only `--ref`.
- `tern-kata worker init`, `views add` and `recur` refuse a flag-like word as a flag's value (`--search --all`), as `agent` already did; `--search=--all` still passes it.

## [0.7.2] - 2026-11-02

### Fixed

#### Install and update

- **Update tern-kata** no longer reinstalls a window plugin that is already at the latest release, and no longer toasts that it was updated: it toasts "tern-kata plugin vX is up to date".

#### Issues and Pages

- The Issue detail's Owner, Priority, Labels and date badges no longer touch the badges in the rows above and below them.
- A mono badge (a priority, a date, an Issue ref, also on Board cards) no longer has larger text than the badges beside it.

## [0.7.1] - 2026-11-02

### Fixed

#### Install and update

- **Update tern-kata** updates the plugin on a Windows window machine after updating a session host on another machine. It used to fail in two ways.
  - Windows could take over half a second to start the PowerShell when the script was on its command line, which ran the plugin step past Tern's time limit; the script now goes to PowerShell on its standard input, which starts in a few milliseconds.
  - The step ran inside the plugin's own folder, which Windows won't let the install replace ("tern-kata plugin was not updated"); it now runs in your home folder.

#### Jobs and worker

- On a Windows session host, a supervised Worker no longer stops `tern-kata update` and the install scripts from reinstalling the plugin while it runs: the plugin starts it in your home folder, not in the plugin's folder.

## [0.7.0] - 2026-11-02

### Added

#### Install and update

- `tern-kata update` brings the session host to the latest release.
  - It reads `version.txt`, downloads the OS archive and checks its SHA-256 against `checksums.txt`, then swaps the running binary (on Windows the old file is renamed to `.old`, and one still held by a running Pane or Worker is renamed aside, so nothing has to be closed first).
  - It says when the binary is already up to date, never downgrades, refuses a build that is not a release with the command for its kind of install, refuses to run twice at once, and prints the new version, a reminder to reopen Panes and the skills update command; `--pause` waits for Enter after a success.
- `tern-kata update` also updates this machine's plugin and restarts a supervised Worker when idle.
  - An installed copy whose `plugin.toml` version differs is updated through `tern plugin install --force`, also when the binary is already up to date; a linked copy or a missing `tern` is only noted.
  - A supervised Worker is stopped when idle, and the plugin answers the idle stop with a fresh Worker. A Worker started by hand, or with `worker.manual`, is not stopped and is told to restart.
  - After a forced refusal (a `worker.toml` one, or a pre-flight that could not check this machine) the Worker is left running, with the steps to fix `worker.toml` (`worker upgrade` or `worker edit`), then `worker stop`.
- `tern-kata update` runs the downloaded binary's pre-flight check before swapping and refuses a release that this machine's kata daemon or `worker.toml` can't run, naming the versions or the config error.
  - `--force` prints the refusals as warnings instead.
  - A kata daemon that does not answer within 10 seconds is only a warning.
- The **Update tern-kata** palette command runs `tern-kata update --pause` in a Pane on the focused Pane's machine.
  - When that machine is not the window's and the update succeeded, it reinstalls the window machine's plugin from the same release without a terminal, then toasts "tern-kata plugin updated to vX" (or the script's last error line).
  - The startup failure toasts and the worker log now say tern-kata may be older than the plugin and name Update tern-kata or `tern-kata update`.
- The window plugin checks for a new release once a day (at window start and every 24 hours while the window stays open) and toasts once per version when one is out, naming Update tern-kata.
  - The **Turn tern-kata update checks off/on** palette command or `TERN_KATA_NO_UPDATE_CHECK` turns the check off, and a plugin linked with `tern plugin link` never checks.
- The window plugin is installed from the release, next to the binary, not from the `main` branch on GitHub.
- Window machines get a plugin-only one-liner: `TERN_KATA_PLUGIN_ONLY=1` makes `install.sh` and `install.ps1` install just the plugin.
  - There is no binary, `PATH` or skills step; the install fails when `tern` is not on `PATH` and leaves a linked copy as it is.

#### Agents

- `w` on an Issue opens the **Agent card**, with or without agents.
  - It lists the agents with state, start time and where each is; `enter` goes to one, and `b` sends one to the background.
  - `s`, `t` and `g` start a new interactive omp agent in the Issue's checkout, in a split right of the kata Pane, a tab (right of the kata Pane's tab, in that Pane's Tern session, with the plugin's window half) or the background.
  - `agent.toml` sets the actor, command and agent name for every Project or one, and a closed Issue refuses `w` with a Notice.
  - The agent runs through `tern-kata agent` (the `tern-kata` on the PATH of Tern's shell, so it must be this version) with its Issue as `KATA_REF`, so kata-plugin's todo plan hangs under that Issue, and without your `KATA_INBOX_USER`.
  - It renews its claim when it already owns the Issue, and finishes or drops its todo children before closing.
  - When it stops without closing it comments why, sets the Issue's **Work attention** to `needs-human` with its reason and labels it ready-for-human.
- The **Agent badge** on an Issue's list row, Lane row, Board card and detail shows "agent working", "agent waiting for you", "agent idle" or "agent exited", with " · N" when the Issue has several agents.
  - On the Board card and in the detail a click focuses the agent.
- A **Background agent** opens beside the Pane when you click its Agent badge in any kata Pane or focus it in the hidden "kata agents" session, as Tern's notification for an agent waiting for you does.
  - The Pane that started it moves it back into the hidden "kata agents" session when it works again after waiting for you, wherever it was opened.
- A **Background agent** ends once its Issue is closed and it is idle or has exited.
  - The Pane that started it closes its block with `tern close` and drops its entry and prompt file, unless you have it open in any kata Pane; a close that fails shows Tern's words in a Notice and leaves the entry.
  - An agent in a split or tab is closed this way only once a kata Pane has sent it to the background.
- While an agent you started waits for you, the Issue holds an **Attention request** for your Inbox address with the agent's last message, also when the Pane finds the agent already waiting.
  - It clears when no agent on the Issue waits and when the Pane quits, and a newer request someone else wrote stays.
  - The request names the agent's `agent.toml` name (by default its program, such as `omp`) as its teammate, so the Attention Page shows `actor/<name>`; an Issue holds one request for you, so the agent that waited last is the one named.
  - An agent that exits while its Issue is still open, also when you quit it with `/exit`, leaves the request "agent exited without closing <ref>" until the Issue closes, another agent on it waits or the Pane quits.

#### Issues and Pages

- Saved views: a name for a base fixed Collection or Project with the Filter card's fields and a search text, kept in `views.toml` beside `filters.toml`.
  - The Menu's Views level lists them by name after the fixed Collections and ends with **New view**, which opens the View card.
  - Opening one makes its base current with its filters applied (`@me` is the Pane's actor), and the filter line ends with "view <name>" while the Collection and filters are still the view's.
  - `f2` edits the view under the cursor, `ctrl+d` deletes it after "Delete view <name>? y/n", and a view whose Project is gone or archived says so.
  - See [Pages and the Board](docs/guide/pages-and-board.md#saved-views) and [Views from a shell](docs/guide/views.md).
- A Project can share its Lane list through kata: **Save to: The Project** on the Lanes card writes it to the Project's `tern.lanes` metadata.
  - Every tern-kata on the daemon shows it before its own `lanes.toml`, and a change from another machine reaches an open Board at once.
  - The Board and the Lanes card say whether the list in use is shared, this machine's or the default.
  - `ctrl+x` on the card stops sharing, and a Project changed since the card opened keeps your text and names the list it shares now.
  - An archived Project's Board shows its shared list, which the card can't change until the Project is restored.
- Date fields in the Dates, Create and Recurrence cards open a calendar beside the field on `ctrl+d`.
  - The arrows move it by a day or a week, `enter` or a click picks the day and `esc` closes it unchanged.
  - Typed dates, `today`, `tomorrow`, `+Nd` and `+Nw` work as before, and the card's other date is marked on the calendar.
- A right-click on an Issue row, a Lane row or a Board card opens a menu of Open, Agent…, Close, Priority…, Dates… and Move to Project, plus Reopen on a closed Issue.
  - Each item shows its key and does what that key does on that Issue.
  - On a closed Issue, Close and Agent… are disabled and say why, and `x` says the Issue is already closed instead of opening the Close card.
- Hovering an Issue reference (the Relationships line, Comment links, Links and Events rows, the detail's parent and the Blocked marker) shows a card with its ref, title, status, owner with the claim's expiry and updated time.
  - A Children row's ticket shows the same facts as a tooltip.
  - A reference that isn't loaded shows only what its link carries.
- An open Issue's **Work attention** (`work.attention` set to `needs-human` or `stuck`, by an agent, kata-plugin or anyone else) shows as "needs human" or "stuck".
  - It sits beside the Agent badge on list and Lane rows, as a badge on the Board card, and with its message as a Work attention property in the Issue detail.

#### Jobs and worker

- The worker reference explains how to back up and restore the worker's local state: what to copy, what to leave out, and what a restore on another machine means.
- A worker profile's `extensions` key (`"ambient"`, the default, `"none"` or a list of absolute paths) chooses which omp extensions its runs load.
  - `worker doctor` warns when kata-plugin's omp extension is installed and a profile still loads ambient extensions, which gives its headless runs kata-plugin's todo takeover.
- When a run succeeds but leaves its per-run Issue open, the worker also sends the Issue's owner (else its author) an **Attention request** naming the Issue, beside the comment and the `needs-review` label.

#### Tern window

- A kata Pane's icon in Tern's tab bar is kata's mark, the katakana カ, in place of the terminal icon.
  - It is tinted like the icons beside it, also in a window attached to the Pane's host from another machine.

#### Command line

- `tern-kata views` lists the fixed Collections and saved views, all without Tern and with the View card's checks and messages.
  - `views show <name>` prints the Issues as the Pane groups them (`--json` for one object per group, `@me` from `--as`).
  - `views add <name>` and `views rm <name>` save and delete a view.

### Changed

#### Requirements

- tern-kata needs Tern 0.9.9 or newer, which draws the buttons, tables, selects, calendars, menus and empty states the Pane now uses.

#### Install and update

- `install.ps1` says that new Tern tabs and Panes see the `PATH` it changed at once; only shells already open and other programs already running need restarting.

#### Issues and Pages

- The filter line under the top bar shows each active filter as a chip after a funnel ("status open", "owner me", a Relationship in the Filter card's words such as "relationship has parent"), and the muted note "default" or "view <name>" at its end, in place of the plain `status=open (default)` text.
- The help sheet (`?`) lists the global keys and the keys of the Page it is opened over, each group of keys as a card.
  - The cards are set in one to three equal-width columns as the Pane allows and balanced so the columns end close together.
  - The keys are in an accent mono column that lines up down each column, a long action wraps inside its card, and the agent key `w` sits on one short line.
  - The last line names both keys that close the sheet: "press ? or esc to return".
- The Menu's top level is short: the bound Project, **Views**, **Projects**, the Lettered Pages, Restore a deleted Issue and New Project.
  - The fixed Collections (Inbox, Today, Upcoming, Deadlines, All Open, Logbook) moved into a **Views** level, which Back, `esc` or `backspace` leave for the top with the cursor on Views.
  - The Views row names the current fixed Collection or saved view, typing still finds every Collection from the top, and an unbound directory's Menu sidebar shows the Views level.
- The Issue detail's creation line no longer says "creation: legacy", which every Issue and comment on a Project without federation carries.
  - A legacy Issue or comment shows "source: <actor> / <teammate>" only when it has a teammate or was created by someone other than its author, and no line otherwise; verified and pending lines are unchanged.
- Inbox, Upcoming and Deadlines read kata's all-open snapshot and pick their Issues in the Pane, as kata's web does, instead of the daemon's inbox, upcoming and deadlines views that kata keeps only for older web UIs.
  - They list the same Issues in the same groups; a Deadlines row's start date now reads in the Pane's time zone, as it does in every other Collection.
- The Pane is drawn in Tern's interface face (sans).
  - Issue refs, dates, counts and times stay in mono, the key line at the bottom is a key legend, and the Issue detail's properties line up in either face.
- Popup card and Menu buttons are Tern's own buttons, with their keys drawn as keycaps.
  - A card's buttons sit in a footer bar that stays in view while a tall card scrolls.
  - Its head names the card with its subject (the Issue, Collection, view or workflow) as muted text after it.
  - A destructive confirm is red, and a button whose action can't run now, such as Go to agent with no agent on the Issue, is disabled with the reason as its tooltip.
- Choice fields in Popup cards are Tern's controls: a yes/no field such as Worker Trust is a switch, a short choice such as the Filter card's Status is segmented tabs, and the others are selects.
  - `down` opens a select, typing searches a long list, and `enter` picks.
  - `left`, `right` and `space` still step a choice, a click picks, and Recurrence weekdays are unchanged.
- The Issue detail's Children is a table that sizes its columns and drops the least important ones when narrow; a click on a row still goes to that child.
- Empty Pages and lists (Attention, Runs, Jobs, Workflows, an Issue list with no Issues and the detail with nothing selected) say what is missing and what to do, with a button when one key fills them.
  - A Lane with no cards says so, and words that wait on someone use Tern's pending colour.
- The Issue detail with many comments, the Issue list and the Board draw faster.

#### Jobs and worker

- A workflow's prompt step no longer inherits kata-plugin's steering variables (`KATA_REF`, `KATA_TEAMMATE`, `KATA_INBOX_USER` and the rest) from the worker's environment: of kata's variables it keeps only those that reach and sign in to the daemon.
- A cron trigger whose expression names a `CRON_TZ=` or `TZ=` zone other than the trigger's `timezone` no longer runs in the expression's zone.
  - The worker sets the job aside with "its cron expression names zone X but the trigger's timezone is Y", shown by `worker status`, `worker doctor` and the Job card.
- A job can name its adapter in `options.adapter`.
  - The Job card has an **Adapter** field and puts `tern-kata` on new adapter jobs.
  - The worker neither schedules nor runs a job that names another adapter, listing it in `worker status`, `worker doctor` and the Jobs Page with that reason; `worker upgrade` grants such a job no secrets or command steps.
  - A job without the key runs as before.
- The Worker card lists its facts (Project, Checkout, `worker.toml`, Worker, Supervision) as an aligned grid, each value leading with its state in that state's colour, and names the checkouts `worker.toml` maps only when they differ from the Checkout row.
- The Worker card's restart and supervision-off questions list **when idle** and **now** each beside what it does to the runs in flight, and no longer say a run is held `interrupted` when none is in flight.
- `e` on the Worker card, `ctrl+e` in the Draft editor and the Workflow card, and `tern-kata worker edit` open `$VISUAL`, else `$EDITOR`, else `vi`, as kata's own TUI does.
- A Project's `capacity` defaults to 3, so one hung run no longer holds its only slot until its timeout.
  - `worker init`, `worker add` and the Worker card's Set up and Serve this Project write `capacity = 3`; a `worker.toml` that says `capacity = 1` keeps it, and one with no `capacity` line gets 3 after the upgrade.
  - Every run's contract now tells the agent that other runs may be working in its checkout, and to make a git worktree when it needs a tree of its own.
- A held run's summary in kata starts with the line `reason: <code>` and has the detail on the next, kata's convention for a machine-readable cause, instead of `<code>: <detail>` on one line.
  - The Runs Page shows `reason: <code>` on the card and a Reason line in the detail for any run whose summary leads with one, another executor's too, and for runs held by an earlier tern-kata.
  - The Jobs Page and a Workflow's inspector word such a run as `<code>: <detail>`.
- Supervision is decided by tern-kata itself: the window plugin's host half asks `tern-kata worker supervised` whether to start the worker, so update tern-kata and the window plugin together.
  - A tern-kata too old to answer leaves the worker unstarted, and the plugin logs this once per daemon with the advice to update.
- A run's detail reads its output tail in bounded memory, and the worker no longer lists every Cron job on each Issue event while an occurrence waits nor reads every run journal to admit a job that forbids overlap.

#### Tern window

- New kata Panes on Windows no longer wait for the PowerShell profile: the kata Pane, Update tern-kata and Install tern-kata's Windows choice start PowerShell with `-NoProfile`, while your own shells and other Panes still load it.
- **Open kata**, `F9` and kata links no longer fall back to a new tab when Tern can't split the focused Pane: the Tern that tern-kata now needs lets plugins split and find a remote host's Pane.
- Tern closes a kata Pane without asking only when kata's `[tui] confirm_quit = false`, as it stands when the Pane starts; with the default it still asks.

#### Command line

- `tern-kata views` words dates (`scheduled X · due Y`) and the filter summary (`status=open · owner=dana · search=x`) the way the Pane does.

### Removed

#### Issues and Pages

- A click on the top bar's Project heading no longer opens the Menu; the Menu button and `esc` from the list do.

### Fixed

#### Install and update

- `install.ps1` works in Windows PowerShell 5.1, the `powershell` the README one-liner and the palette's Install and Update commands run: it no longer stops with "You cannot call a method on a null-valued expression" before downloading anything.
- `install.ps1` no longer fails with "Cannot create a file when that file already exists" when the previous install's `tern-kata.exe.old` is still held by a running tern-kata.
  - Reinstalling on Windows works while an older tern-kata still runs, and the next run removes the leftover `tern-kata.exe.old-*` files once those processes have exited.

#### Agents

- The **Attention request** of an omp agent waiting for you says what it waits on: the question of its pending ask call, else its last message, no longer the cut-off start of a thinking block ("Thinking…\nI'm checking who owns the claim on").
  - An omp agent with no text to show gets "agent waiting for you", and an agent that is not omp keeps its last message.
- An agent started from a Pane on another daemon (by `KATA_HOME`, by `KATA_SERVER` or after a switch on the Daemons Page) runs its `kata` commands against the Pane's daemon.
  - Its `KATA_REF` Issue is claimed, commented on and closed there, and kata-plugin registers nothing on Tern's default daemon.
  - The Pane's token, the `token_env` values its `config.toml` names (each reaches only the daemon kata scopes it to) and a Postgres DSN reach the agent through private files, never the command line, and the block's own `KATA_DB`, `KATA_DSN` and `KATA_POSTGRES_SCHEMA` no longer mix with the Pane's home.
  - A switch to a local daemon that the checkout's `.kata.local.toml` pin or `active_daemon` would override, or to a remote entry without a token while `config.toml` has an `[auth]` credential, is refused with the reason in the "Cannot start an agent" Notice.
  - A Pane on the default daemon starts agents as before.
- Agent mapping (`agents.json`) changes from two Panes at once no longer lose one: a save applies on top of another Pane's save, or is refused with the reason shown on the card.
- Sending an agent back records it as a background agent even when a later move step fails, and the Notice still shows Tern's error.
- A Pane reading live agent blocks waits only until its own deadline instead of behind another read's `tern ls`.

#### Issues and Pages

- Lane list (`lanes.toml`) and Default filter (`filters.toml`) saves from two Panes at once no longer lose one: a save applies on top of another Pane's save, or is refused with the reason shown on the card.
- Popup cards draw their error, pending, warning and success lines in the theme's colours instead of the plain text colour, as do the worker notes of a Workflow's inspector.
- With Tern 0.9.8 on a high-density display, the Issue list, the detail and the other column lists no longer run past the bottom of the Pane and scroll the top bar away: each list now fills its column and scrolls inside it at any display scale or Pane height.
- Stopping a Recurrence from its card or with `tern-kata recur --stop` writes nothing when its rule, start date or time zone changed since you read it and shows the schedule as it stands.
  - A stop that got no answer says the Recurrence may have been stopped.
- A Lane list answer sent before a save no longer replaces the saved Lanes, and saving the Lane list to the Project keeps the Undo history.
- Undoing a body edit after someone else changed the body writes nothing and says why.
  - Undoing a label add or remove that someone else already changed says so; a change to another field no longer blocks either.
- `ctrl+u` on an empty Popup card field keeps the card's error.

#### Jobs and worker

- `worker doctor` no longer tells you to create `worker.toml` when the file exists but does not load, and after a warning it says nothing is missing and how many warnings it printed.
- A notify job with a zero lead, the current-owner-or-author recipient and an Issue-date trigger is skipped by the worker only when it notifies the trigger's own Issue; one that names another Issue to notify is now sent by the worker to that Issue's owner or author.
- An `execute` job on an existing Issue no longer starts while the Issue is `someday` or scheduled for later.
  - The scheduled occurrence waits without taking a slot until the Issue is ready, past its catchup grace and across worker restarts (the 7-day catchup window still applies).
  - `worker status` says why, and a manual run is refused with the reason; `deadline_on` never defers it.
- `e` on the Worker card and `ctrl+e` no longer only blink when neither `$VISUAL` nor `$EDITOR` is set, as in a Pane started by **Open kata**: they open `vi`, and with no `vi` either the card says to set one without leaving the Pane.
- Re-enabling a disabled Cron job runs nothing for the time it was off, whatever its `catchup`, with or without a worker restart in between.
- A succeeded run whose per-run Issue can't be read when the worker reports it sends its final observation and waits to comment and label until the Issue can be read, so a closed Issue no longer gets the final message comment and `needs-review`.
- During an idle stop, `worker abandon` of a Held run is accepted; `run` and `resume` are still refused while the worker stops.
- `worker resume` of a Held run whose output can't be set aside, for example because another program has it open on Windows, is refused with the reason and the run stays held as it was, instead of the new attempt overwriting that output.
- A finished turn whose `agent_end` line passes 64 MiB is recognised, live and on recovery (`no_receipt`, not `interrupted`), while a non-terminal one still is not.
  - A single write over the line limit that contains a newline is checked too.
- The Job card lists no next runs, and says why, for an existing interval job whose start point is unknown, instead of runs counted from now.

#### Command line

- A failed write of tern-kata's output (`--version`, help, completion or a subcommand's output, for example to a full disk) exits 1, not 3.

## [0.6.0] - 2026-10-26

### Added

#### Issues and Pages

- A double click on a card of the Jobs, Runs, Workflows or Attention Page opens it as Enter does: the job's inspector, the run's detail, the workflow's steps, or the request's Issue.

### Fixed

#### Attachments

- Attaching a file keeps the stored copy when kata's answer to the pointer write is lost, so the Issue never points at a deleted file.

#### Issues and Pages

- Undoing a Lane move whose second label write was refused can be retried to completion instead of reporting a conflict forever.
- Undoing a Lane move refuses when another actor moved or relabelled the card since, including a card that had no Lane before the move.
- An absolute `TZ` path such as `:/etc/localtime` gives the Pane its local date instead of UTC.
- Restore notices name an Issue of a Project the Pane's catalog does not list by its short id, not `#abcd`.
- Recurrences refuse the zone `Local`, which kata would evaluate in the daemon's zone.
  - A new recurrence, or an existing one stored as `Local`, opens on the Pane's zone (UTC when that is `Local` too), and the card and `tern-kata recur` say when saving replaces a stored `Local`.
- Relative ages read "0s ago" instead of a negative age when the daemon's clock runs ahead.
- Searching by short id resolves in the bound Project when a saved filter names a Project the catalog no longer lists.
- A change you make while the Pane is reading its list, such as moving a card, is read again once that read lands, so the card does not stay where it was when no change event follows (for example while the event stream is down).
- Issue and Children lists keep their owner column aligned for names in CJK or emoji: the columns are fixed-width boxes, so names drawn in another font no longer shift.

#### Jobs and worker

- A worker run whose agent finished its turn is no longer recorded `failed` with "no agent_end" when omp's output carries a message the worker does not read, such as a late LSP diagnostic: the worker now reads the turn's end and final message past it.
- A run that finished while the worker was stopped is held `no_receipt`, not `interrupted`, when its output passed 8 MiB: the saved `output.jsonl` keeps omp's `agent_end` line past the cap.
- A long run whose agent finished its turn is no longer recorded `failed` with "no agent_end" when omp's final `agent_end` line passes 8 MiB; lines up to 64 MiB are read.
- A run journal the worker cannot read for a passing reason, such as too many open files, is no longer quarantined for good; only a journal that does not decode is.
- While a run journal cannot be read, the worker keeps serving the other runs, and a Project whose runs are not all recovered admits nothing until they are, so it never runs past its capacity.
  - `worker status` and `worker doctor` name the unreadable run and say how to clear it.
- A run whose agent's last message cannot be read is recorded `failed` with no stop reason instead of taking an earlier turn's result.
- A run or command step that exits 0 while a process it started in the background still holds its output is recorded by its real exit status, not `failed` with "exited with status -1".
- A run's final report no longer counts as delivered when kata already holds the same status with another summary; the run is left for inspection instead.
- A run's Stderr in the Pane shows the end of a long stderr as plain text, for prompt jobs and concluded workflow steps alike, instead of text from its first 64 KiB.
  - A retried step shows its current attempt's stderr, not the previous one's.
- `worker resume` renews the Project's lease first and refuses when another machine has taken the Project over, instead of starting the agent on a lease that may be gone.
- Saving a new adapter job again after its first answer was lost reports it created, not changed.
- A Workflow conflict whose current version another actor archived offers only Discard Draft, in the card and its key hints; Overwrite would have silently restored it.
- A Workflow conflict shows its choices above the two texts, so they stay visible in a short window, and an archive or restore conflict keeps the selection at its row.
- A foreground `tern-kata worker` stopped by ctrl+c or a signal exits with 128 plus the signal number instead of 0.

#### Daemons and Federation

- A workspace pinned to a daemon in `.kata.local.toml` is labelled with that daemon and marked Current on the Daemons page, and federation enrolment uses its credentials, not the active daemon's.
  - The Pane reads the pin by kata's rules: within the workspace, not when git tracks it, and a `url` pin with no catalog credentials.
- Switching daemons or loading a Project no longer hangs when the daemon answers but stalls reading its instance; it gives up after the connect deadline.

#### Tern window

- The Pane ends cleanly when it is told to stop while it is still connecting or while an editor it opened is running.

#### Command line

- A lone `--` ends tern-kata's own flags: what follows, such as `--issue`, goes to kata's TUI as plain arguments; `--issue` together with another argument for kata's TUI is refused, as it takes one.
- `-h`, `--help` and `--version` given as a flag's value, as in `tern-kata worker init --command -h`, are taken as that value instead of printing help.

## [0.5.0] - 2026-10-25

### Added

#### Jobs and worker

- `tern-kata worker status --json` prints the worker's status as JSON for scripts, in the shape the worker's control socket answers it.
- `tern-kata worker upgrade` gives the jobs you have today what they had before secrets and command steps became per-Project.
  - For each `[[projects]]` table without `secrets` or `commands`, it adds the secrets its jobs use and `commands = true` when one of its workflows runs a command step, leaving every other line as it was.
  - A key you already wrote stays; what the jobs need beyond it is printed.
  - `worker doctor` suggests it when a job is set aside for either reason.

#### Command line

- `tern-kata completion bash|zsh|fish` prints a shell completion script for the subcommands, the worker forms and their flags: `source <(tern-kata completion bash)` (or `zsh`), or `tern-kata completion fish | source`.

### Changed

#### Attachments

- Attaching and detaching refuse an Issue uid from kata that is not a ULID, so a daemon's answer cannot point the Attachment store outside its folder.

#### Issues and Pages

- The Pane responds faster in large Projects.
  - Moving the selection in the list, the Issue detail or the Board reuses the rows already worked out instead of rebuilding them on every key, and a live refresh builds them once.
  - The Jobs Page counts each cron job's "about N Issues a day" warning once per job revision when the jobs are read, instead of on every key, which kept it slow with a job that fires every minute.
  - The Collection and Issue detail reads from a local daemon over its Unix socket now arrive uncompressed, so neither the daemon nor the Pane spends time on gzip for each refresh.

#### Jobs and worker

- A worker serving several Projects keeps each one's secrets to itself: a Project's jobs may use only the `[secrets]` names its `[[projects]]` table lists in `secrets = [...]`, and an empty or missing list allows none.
  - A job naming an unlisted secret is set aside, and is held `secret_unavailable` if run anyway.
  - Run `tern-kata worker upgrade` once to list the secrets your jobs use today.
- Workflow command steps (`sh -c`) run only for a Project with `commands = true` in `worker.toml`; without it, a job whose workflow has a command step is set aside with that reason.
  - `tern-kata worker upgrade` adds the switch where a workflow needs it.
- The worker writes an Issue's title into the agent's prompt as one line, so a title with line breaks cannot pass for another section of the prompt.
- The worker's control socket answers only its own user.
  - The socket is mode 0600, the state folder (followed through a symlink) is narrowed to 0700 when open to others and refused when it is someone else's, and on Linux and macOS a connection from another user is closed.
  - A run or Project uid that is not a ULID is refused before it names a file.

#### Daemons and Federation

- The live event stream fails, and reconnects, on a line or an event over 1 MiB, instead of holding however much a daemon sends.
- Leaving a federated Project sends the hub's token over plain HTTP only when your own catalog entry for the hub sets `allow_insecure`; the binding's flag, which the spoke's daemon reports, no longer counts.

#### Command line

- A command line tern-kata refuses exits 2, as Unix tools do: a subcommand's usage error (a missing ref, an unknown flag) no longer exits 1, and the Pane's own refusals no longer exit 3.
  - A subcommand that fails while running still exits 1.
- Inside Tern, `tern-kata` refuses arguments only `kata tui` takes, naming them, instead of opening the Pane as if they weren't there; outside Tern they still go to `kata tui`.
- Ctrl+c, a terminate or a hangup stops a subcommand cleanly and it exits 128 plus the signal's number (130 for ctrl+c); a second one ends it at once.
  - A terminate or hangup closes the Pane cleanly, and the foreground worker stops on a hangup as on ctrl+c.
- After a lone `--`, `--help` and `--version` are names, not flags, so `tern-kata attach <ref> -- --help` attaches a file called `--help`.
- Subcommands and the foreground worker show control characters in text from kata (titles, attachment names, a daemon's error message) as `�`, so an escape sequence in it cannot reach the terminal.

## [0.4.0] - 2026-10-25

### Added

#### Agents

- The Page bar gains an Attention Page (`6`) listing every request kata holds for your Inbox address.
  - It covers `KATA_INBOX_USER`, else the actor the Pane writes as, in every Project on the daemon, newest first.
  - A row names `project#id`, the title, who asked (`actor` or `actor/teammate`), a tag (request, reply kind or broadcast), the comment a request names (`c:abc123`) and the first line of the message.
  - The selected row shows the whole message, and the header reads "for dangerboi · 3 requests".
  - A request kata would not read is counted as skipped, a failed read keeps the rows, and an address kata would refuse shows why.
  - Enter opens the Issue on the Issues Page, in any Project; `ctrl+r` reads again.
- `c` on the Attention Page clears the selected request, guarded on the value the Page showed.
  - A request rewritten meanwhile is kept, with "The request changed meanwhile; nothing was cleared" and the new request shown, and the row leaves the list on success.
- The Page bar's Attention entry shows how many requests wait and stays current from any Page.
  - It reads "Attention 3", plain at zero.
  - The Pane reads the requests again after it connects and on every Issue or Project event of the daemon, at most one read in flight and one after it.
- Enter on an Attention request that names a comment opens the Issue on its Comments tab with that comment under the cursor, so `R` answers it and kata clears the request.
  - A reply notice opens on the Reply, and a comment that is gone opens on the first comment.
- The kata skill covers kata v0.20.
  - It covers reply requests (`notify --re`), `wait --until reply`, broadcasts (`notify --broadcast`) and repeatable close evidence (`--commit`, `--pr`, `--test`), and no longer carries the v0.19 workarounds.

#### Issues and Pages

- `M` in the list, on the Board, in the Lane view and in the Issue detail opens the Move card, which moves the selected Issue to another Project through kata's move action, as kata web does.
  - The card lists the Collection snapshot's Projects, without the Issue's own and the archived ones, and filters them as you type.
  - `enter` on one previews the move with kata's dry run, and `enter` again moves it, guarded on the revision the card opened with. kata names the Project it moves to, not the new short id, which it assigns only on moving, or shows its refusal, such as "moves are unsupported for federated Projects".
  - When the Issue changed meanwhile the card stays open and offers a fresh preview.
  - A Notice names the new `project#id` as a link that opens the Issue, the selection goes to the neighbouring row once the Collection no longer shows it, and the move clears the undo history.
  - The help sheet and the footer list `M`.
- `X` in the list, on the Board, in the Lane view and in the Issue detail opens the Delete card, which soft-deletes the selected Issue through kata's delete action, as the CLI does.
  - The card names the Issue's `project#id` and title, says a deleted Issue can be restored, and sends the delete with kata's confirmation (`DELETE <project>#<id>`) and an optional reason only once the typed `project#id` matches.
  - kata's refusals show on the card, and purge is not offered.
  - A delete enters the undo history, so `u` right after restores the Issue (refused as "already restored" when kata no longer holds it deleted).
  - The Menu's "Restore a deleted Issue" takes a short id, `project#id` or ULID, reads that Issue with deleted Issues included, shows its title and that it is deleted, and restores it on `enter`.
  - The help sheet lists `X`.
- `u` in the list, on the Board, in the Lane view and in the Issue detail undoes the newest change the Pane made, as in kata tui.
  - It undoes a close, reopen (opening the Close card when kata requires evidence), owner, priority, label, body edit, added link, Lane Move or Dates card change, up to the last 20, newest first, after kata tui's checks ("daemon instance changed", "issue owner changed since the action", "issue dates changed since the action" and the rest).
  - Creating an Issue, a comment or Reply, a timed assignment, a Recurrence change or stop, attaching or detaching a file and an unknown outcome clear the history, and `u` then says "nothing to undo: <reason>".
- `q` outside text fields asks "Quit tern-kata? y/n", as in kata tui.
  - `y` quits, `n` or `esc` stays, and `ctrl+c` quits at once.
  - `q` typed into search or a Popup card stays text, and kata's `[tui] confirm_quit = false` makes `q` quit at once.
- kata tui's other keys work too, and the help sheet lists them.
  - `ctrl+j`/`ctrl+k` change Issue detail tabs, and backspace goes back where `esc` does outside text fields.
  - `ctrl+alt+<`/`ctrl+alt+>` go to the first and last row.
  - `!` then `0`-`4` or `-` sets or clears the priority at once (any other key closes the field and is handled as usual).
  - `T` opens the Attention Page from any Page and the Menu.
- `]` and `[` on a comment in the Issue detail's Comments tab step through its Comment links, as in kata tui.
  - The links are the comment it answers, then the Replies to it on any Issue.
  - The chosen link wraps around and is marked on the comment ("link 2/3: fay refutes in #bbbb"), and `enter` jumps to it, across Issues and Projects, landing on that comment; a comment without links keeps `enter` as it was.
  - The help sheet and the detail footer list both keys.
- The Issue detail shows kata tui's creation line for the Issue (under its properties) and for each comment: "creation: verified · accountable: alice · source: agent / bot".
  - The state reads verified, pending or legacy (an unknown state reads legacy).
  - "accountable" shows only when verified and set, and "source" is the source actor or the author with " / <teammate>" when there is one.
  - Nothing is shown when kata returns no verification state.
- The Close card takes kata's two other evidence types, "Reviewed paths, comma-separated" and "External account", sent as kata's typed evidence.
  - Either satisfies a done close, and reviewed paths may also accompany an audit-no-change rationale.
  - An empty value is not sent and a value of only commas is refused.
  - Activity names them ("reviewed-paths src/a.go, src/b.go", "external alice").
- `t` in the Issue detail gives the Issue a timed assignment, as in kata tui.
  - The field "Assignment duration for #id (1m..24h, e.g. 1h)" takes a Go duration of whole seconds from 1m to 24h and refuses anything else with kata tui's wording, then claims the Issue for your actor for that long.
  - The owner and expiry show after the reload.
  - The help sheet and the detail footer list it.
- `b` in the Issue detail adds a Blocking link and `l` a related link to the Issue you name (a short id, `#id` or `project#id`), as in kata tui.
  - The link is worded by the direction kata records ("This Issue blocks"), and kata's refusal (an Issue to itself, an archived Project, scope) shows under the field.
  - `b` still toggles the Board on the Board and in the Lane view.
  - The help sheet and the detail footer list both keys.
- `o` in the nested Issue list switches siblings between creation order and blocking order, as in kata tui.
  - Blockers come first in blocking order; creation order breaks ties and cycles.
  - It says "graph order: topological" or "graph order: temporal".
  - The choice lasts for the Pane and is not saved; the help sheet and the list footer list it, and `o` on the Runs Page still opens the session.
- Activity names who was asked, told or cleared.
  - A notification request reads "asked bob: <message>", the notice a Reply left "told bob about a confirm" (reply, confirm, refute or supersede), and a cleared request "cleared bob's request".
  - A request sent to several people reads as one line naming three of them and counting the rest.
  - A metadata change that mixes notifications with other keys names each recipient instead of the encoded key.
  - When kata reaches an Issue's scheduled date, Activity says "reached the scheduled date <date>".
- A per-Project default filter: `ctrl+d` in the Filter card saves the card's values (never search text) as the default of the Project the Pane's folder is bound to, and applies them.
  - The default is kept in `filters.toml` beside `lanes.toml`.
  - A Pane starts with its Project's default already applied, the filter line ends with "(default)" while the active filters equal it, and `c` clears them for that Pane only.
  - An empty card removes the default; an unreadable file starts the Pane unfiltered with a Notice and is never overwritten.
- The top bar has a Page bar, "1 Issues  2 Board  3 Jobs  4 Runs  5 Workflows  6 Attention".
  - The digits or a click switch Pages.
  - A Lettered Page (Credentials `C`, Federation `F`, Daemons `D`) is drawn there only while it is shown, `b` still toggles between Issues and the Board, and each Page keeps its selection while another is shown (the Runs Page also its filter and the older runs it read).
  - The Board Page is what Board mode was. It shows the Pane's Project, the Project Collection if one is current, else the workspace's Binding (from Inbox or another fixed Collection in a bound workspace it switches to that Project), and with neither it says so and opens the Menu's Projects list.
- `b` in a Project Collection switches the list to a Board.
  - It has a column for No lane (open Issues with none of the Lane labels, even one labelled only `bug`), one per Lane (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, each with its card count) and Done (Issues closed in the last 14 days, at most 20, newest closed first; the header reads "Done 20 of 192" when more qualify).
  - Each Lane header is a badge in its own theme tone (No lane muted, `needs-triage` warning, `needs-info` error, `ready-for-agent` info, `ready-for-human` user, Done success; custom Lanes take the other tones in turn).
  - A card shows its short id as a badge in its Lane's tone, then its priority as a badge (`P0` error, `P1` warning, `P2` info, other priorities muted; none without a priority, kept on Done cards), its title over at most two lines (muted on a Done card; a recurring Issue's starts with "↻ ", as in the list) and its labels that aren't Lane labels as info badges.
  - An Issue with several Lane labels shows once, in the first Lane, with "also: …", and a Done card says which Lane `r` reopens it into.
  - The selected card shows its full title, its owner as a user badge and every label.
  - `↑`/`↓`/`j`/`k` move within a Lane, `←`/`→` across Lanes (skipping empty ones), a click selects a card, and the Issue keys, `x`, `r`, the filters and the live refresh work as in the list.
  - The Board and its Lane view ignore the status filter, which stays set for the Issues Page: the Board's Filter card has no Status row and keeps its value, the filter line leaves it out and `s` is not bound there.
  - `b` again returns to the list with the same Issue selected, and `b` with no Project to show says the Board needs one and opens the Menu's Projects list.
  - The Lanes share the Pane's width, from 140 columns up, with card text wrapping to fit, and each Lane's cards scroll inside a list that fills the Lane down to the footer, sized from the Pane's rows, so the top bar stays in view.
  - The footer lists `+`, `-`, `a` and `A` for the selected card on the Board and in the Lane view.
  - An open card with a deadline or scheduled date shows one Date badge after its priority badge, for the most pressing date ("overdue 2d" error, "due today" warning, "scheduled 3d ago" warning, "scheduled today" accent, "due tomorrow" warning, "due 10-24" and "scheduled 10-28" muted; another year reads YYYY-MM-DD).
  - The selected card shows a Dates line ("scheduled 2026-10-28 · due 2026-11-02"); both read dates as the Today, Upcoming and Deadlines Collections do and move on at local midnight.
- `>` and `<` on the Board move the selected card one Lane right or left among No lane and the Lanes.
  - The target label is added first, then the other Lane labels are removed, so a failure leaves the Issue in two Lanes, never none; an Issue with two Lane labels ends with one.
  - The card shows in its new Lane at once, a failure leaves a Notice with the daemon's reason, and a Move is refused while the daemon is disconnected.
- An open Board card that an open Issue blocks shows a "blocked by 8gm4" line in the warning tone, on every card whether selected or not.
  - It lists every open blocker (qualified `project#id` outside the Project, cut to three and "+N"), counts blockers the Board doesn't draw, and never shows on Done cards or once the blockers are closed.
- Selecting a Board card highlights the cards related to it.
  - Each gets a badge beside its short id (`parent`, `child`, `blocks this`, `blocked by this`, `related`, as seen from the selected card) and a ring in the Relationship's tone (accent, info, error, warning, neutral).
  - Several Relationships show every tag, and the ring follows blocks this, blocked by this, parent, child, related.
  - The selected card keeps its ring, Done cards stay muted, and the Lane view shows no highlight.
- The selected Board card shows a Relationships line.
  - It lists its parent, its child count ("6 children, 2 open"), the open Issues it blocks, its open blockers and its related Issues, with empty parts left out and ids qualified outside the Project and cut to three and "+N".
  - Each id is clickable and selects that Issue's card, revealing it in its Lane; an Issue the Board doesn't draw opens in the detail beside the Lane view, and Esc goes back to the card.
  - `^` on the Board or in the Lane view selects the parent's card the same way, and leaves "<id> has no parent." without one. The help sheet's Board group lists `^`.
- Enter or a double-click on a Board card opens its Lane view.
  - It shows that Lane's Issues as a full-width list under a Lane header, with the Issue detail beside it at 140 columns and wider.
  - It has the list's keys; `←`/`→` switch Lanes, `<` and `>` Move the card and select the next card in the Lane, and Esc returns to the Board with the card kept.
- A Pane under 140 columns opens the Board Page in the Lane view, on the selected Issue's Lane, so `B` and the Move keys work there too.
- `B` on the Board or in the Lane view opens the Lanes card, a wide card with a Lane list section and a Done section.
  - The Lane list section holds the Project's Lane list as comma-separated names, with its help under it.
  - The Done section has "Days back" and "Most cards", with the defaults 14 and 20 as placeholders; `tab` moves between fields; blank restores the default 14 and 20, and anything but a positive whole number, at most 3650 days and 500 cards, is refused naming the field.
  - `enter` saves it to the Lane list file and the Board's columns change, and the card stays open, locked, until the save answers: it closes on success, or shows the reason with your text kept; `esc` cancels.
  - Names are tidied to labels (`In review` reads `in-review`), and an empty name, a name over 64 characters or a duplicate is refused with a message naming the entry. An empty field restores the default list.
  - A muted "Not Lanes yet" line under the Lane list names the labels on loaded Issues that aren't Lanes.
  - `B` waits, with a Notice, until the saved list has loaded, and opens nothing, with the file's error as the Notice, while the Lane list file is unreadable.
  - Saving never changes a label in kata.
- The Board's Lanes come from the Project's table in `lanes.toml` under tern-kata's folder of the user config directory.
  - It takes optional `done_days` and `done_cards` beside the labels, read each time the Board turns on or the Project changes; a done_days above 3650 or done_cards above 500 counts as unset.
  - A missing file or Project gives the default Lanes, and an unreadable one gives the default plus a Notice and is never overwritten.
- `d` on the selected Issue (list, Board, Lane view, Issue detail) opens the Dates card, with Scheduled and Deadline fields.
  - The fields are prefilled and take `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd` and `+Nw` against the Pane calendar's date with a muted weekday preview; blank clears and anything else is refused naming the field.
  - Saving writes the changed dates only to the Issue's `scheduled_on` and `deadline_on` metadata, each guarded on the value the card opened with, so a date changed elsewhere is reported and not overwritten.
  - A date kata holds with a time is left alone unless edited.
- The Create card (`n`, `N`) has Scheduled and Deadline fields after Priority.
  - They take the same `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd` and `+Nw` input with a muted weekday preview as the Dates card; an invalid date is refused naming the field and nothing is created.
  - The dates are sent in the one create request as the Issue's `scheduled_on` and `deadline_on` metadata, and none when the fields are blank.
- Every single-line text field has a movable caret.
  - The fields are Create, Close, Recurrence, New Project, the single-field forms, the Lanes card, the Dates card, the Filter form, the Menu filter and Search.
  - `←`/`→` move it a character, `Home`/`End` or `ctrl+a`/`ctrl+e` to the ends; typed and pasted text goes in at the caret, Backspace deletes before it, Delete after it, `ctrl+u` clears the field.
  - The Menu keeps `Home`/`End` for its entries (`ctrl+a`/`ctrl+e` move the caret), and the Filter and Recurrence cards keep `←`/`→` for their choice fields.

#### Jobs and worker

- Cron expressions read as kata v0.20 reads them, for the Jobs Page, the Job card, Backfill and the Worker alike.
  - `7` is Sunday, hour ranges may wrap (`22-2`), weekday steps work (`5/2`), a step as large as its span (`*/60`, `*/24`) and `@every 0s` are refused, and a time that does not exist on a spring-forward day runs later that day.
  - When both day fields are restricted a day matching either one runs.
  - Schedules without a phrase read "cron <expression>", and a stored job v0.20 refuses reads as unschedulable.
- The Workflow card and the Workflows Page say what this machine's Worker would refuse.
  - Under the text of a Workflow kata accepts, and in the inspector of each stored Workflow, a note names the refusals (`options.on_fail`, `options.on_failure: "park"`, a profile on a command step, a prompt budget over `max_run_prompts`) and why, or says the Worker would run it.
  - The note never stops a save; when `worker.toml` is missing or cannot be read it says the prompt budget could not be checked and why.
  - While you type, text kata would refuse shows kata's problem in the card instead of the note.
- `worker status` names why a run on Windows keeps its slot when its Job Object cannot be checked.
  - The resume and abandon refusals and the hold say the group "cannot be checked" with the cause, not that the process is still present.
  - The worker logs it once per cause and frees the slot as soon as a later check succeeds.
- `tern-kata worker edit` and `e` on the Worker card edit `worker.toml`.
  - The command opens `$VISUAL`, else `$EDITOR`; the card opens `$EDITOR`.
  - The text must pass the same strict check the worker applies at start before it replaces the file; a refusal reopens the editor with the problem as a comment at the top (the card names it and keeps your text).
  - An unchanged or empty result cancels (line endings alone are no change).
  - A file that changed while you edited is refused and your edit is kept (the command names the file holding it; the card keeps the text and reads the file again).
  - After a write the command says whether a worker runs and that `tern-kata worker stop` makes it reload, while the card offers its restart choice.
- `s` on the Worker card turns the plugin's supervision of the worker off or on.
  - Off creates `worker.manual` beside `worker.toml`, and with a worker running asks whether to stop it when idle or now, naming the runs in flight.
  - On removes the marker and says the plugin starts the worker within 10 seconds.
  - The card and the Jobs Page banner show whether supervision is off.
- `W` on the Jobs Page (list or inspector) opens the Worker card for the Project the Pane's folder is bound to.
  - It shows whether this machine's `worker.toml` exists, loads (or why not) and lists the Project with its checkout, whether a worker runs (process id, executor, runs in flight) and whether the plugin supervises it or `worker.manual` turned that off.
  - With no file it sets the worker up (executor, actor, command, model and thinking), prefilled as `tern-kata worker init` would except the Command field, which starts blank and shows the default command as a placeholder; a blank field is saved as the default, and a typed value, even the default's own text, is saved as typed.
  - With a file that does not list the Project it serves the Project.
  - Both need the trust choice set to yes, show the checkout and the trust note, and write through the same checked write as the commands, naming the field of a refusal and keeping the input.
  - After a save with a worker running it asks whether to restart it when idle or now (naming the runs in flight) and sends the worker its stop request; with none running it says the plugin starts one.
  - The Jobs Page banner now says per state (no file, file does not load, Project not listed, no worker running, worker.manual) what is missing and points at `W`.
- `tern-kata worker stop` stops the running worker.
  - By default it refuses new work (scheduled occurrences, `run` requests and resumes) with "the worker is stopping", lets the runs in flight finish and report, then exits 0.
  - `--now` ends it at once and holds the runs it interrupts `interrupted` and resumable.
  - `worker status` says it is stopping and how many runs it waits for, and the command says whether the plugin starts a fresh worker.
  - A change to `worker.toml` takes effect by stopping the worker so the plugin starts a new one.
- `tern-kata worker init` takes `--executor`, `--actor`, `--command`, `--model` and `--thinking` to set those values in the starter `worker.toml`.
  - One left out keeps today's value, and a value the worker's loader refuses is named and nothing is written.
- `tern-kata worker init` writes a starter `worker.toml` for the Project bound to the current directory, and `tern-kata worker add` adds another Project to it.
  - The starter has that checkout and the `omp` default profile.
  - `tern-kata worker add`, run in another bound checkout, adds that Project and its checkout to the existing file leaving every other line unchanged.
  - Both check the result with the worker's own loader before writing it and refuse an unbound directory.
- The worker runs agents and workflow command steps on Windows too.
  - Each run's processes join their own Job Object before they run, which proves them gone after the run and ends them if the worker dies, as the process group and parent-death signal do on Linux.
  - Command steps need `sh` on the worker's `PATH` (Git for Windows' `bin` folder).
- The worker checks its route to each Project and admits and reports nothing for a Project until a failed check passes again.
  - It checks cron capabilities, Project uid and daemon instance when its event stream reconnects, before each admission and report and every 5 minutes.
  - It pins the daemon instance and authenticated actor with the Project and serves no Project whose pin cannot be saved.
  - It makes at most four kata calls at once (60 seconds each, reads tried up to three times), starts two Projects at a time, and at start delivers the saved reports of and releases its lease on a Project removed from `worker.toml`.
  - A final observation is never sent over a newer observation by someone else (the run is marked as needing inspection in `worker status` and `worker doctor`).
  - A notify run and a held run's inbox request to the Issue's owner or author are worked out once and journaled before they are sent, and an all-command workflow on an existing Issue leaves no comment.
  - `worker status`, `worker doctor` and the Jobs Page list each job the worker sets aside with its reason and warn of a per-run Issue job that creates more than 24 Issues a day.
  - A secret may not alias kata's own credential or be a file owned by someone else.
- The worker's settling and cleanup rules changed.
  - `tern-kata worker abandon` also stops a running run (SIGTERM to its process group, SIGKILL after 10 seconds, then recorded `cancelled`).
  - It records a run kata shows running but this machine has no journal of as `unknown` ("local state lost", with an Issue comment) and acknowledges a quarantined journal (its folder stays and keeps its veto; `worker doctor` names the command and stops flagging it).
  - Like `run` and `resume`, it is refused while another worker owns the Project's lease.
  - `tern-kata run` and `R` read the Project's jobs again first and refuse when they cannot.
  - A notify job on a closed Issue still notifies.
  - Two requests for a job that forbids overlap, or two resumes of one hold, admit one.
  - A workflow run's time limit starts when it is prepared (a resume's when it is accepted), tokens are summed across retries and resumes, a client that sends nothing no longer delays the worker's stop, and settled, fully reported runs older than 14 days are removed daily.
- The Pane shows this machine's worker.
  - The Jobs Page says whether it runs and serves the Project (no worker, not served, admitting nothing and why, or its slots in use).
  - `R` asks it to run the selected adapter job now under the usual lease, overlap and capacity rules, refusing with the reason on a recurring-child job, an archived job, without a worker or for a Project it does not serve.
  - On the Runs Page a run of this machine's worker shows its local state: a `held` badge with the reason on its card, and in its detail why it is held and whether `u` would work, its steps, the tail of the agent's output and its omp session. While a run runs the page follows it every 2 seconds.
  - `o` opens a finished or held run's omp session in a new Tern tab (`tern new tab --cwd <checkout> -- omp --resume <session> --session-dir <folder>`), and `u` resumes and `A` abandons a held run through the worker's control socket, with refusals shown.
  - Runs of other executors show what kata holds. The help sheet lists the keys.
- The worker runs workflow jobs.
  - An `execute` job whose action names a workflow runs its steps one at a time in dependency order, a prompt step as one fresh headless omp process like a prompt job and a command step through `sh -c` in the job's checkout as the worker's user (output kept as its first 1 MiB and last 256 KiB), each retried up to its `retries` after a certain failure.
  - kata records one run for the workflow with every step's result in its summary, a step that fails after its retries fails the run and skips the steps that depend on it, and a timeout is shared by all steps.
  - The journal covers steps, so a crash mid-workflow holds the run at the uncertain step and `tern-kata worker resume` continues from it (a held command step runs its command again); `run-detail` lists every step.
  - Workflows with `options.on_fail` loops, `on_failure: "park"`, a mismatched profile or more prompts than `max_run_prompts` are refused with the reason.
- A run whose output log cannot be written or opened settles as its process ended, and `worker status` and the run detail say "output not recorded" with the reason.
- Reopening a parent (`r`), resuming a recurring-child job (`p`) and restoring one (`x` on an archived job) first open the Backfill card when the job's catch-up is every missed run and runs were missed.
  - It counts the child Issues that kata's daemon would create at once, job by job, and offers to create them, to switch the job to `latest` and go on (a Guarded write of the catch-up before the reopen, resume or restore), or to cancel and change nothing.
  - Nothing is asked when nothing was missed, the job catches up with `latest` or `skip`, or it would stay paused anyway.
- The worker fires `issue-scheduled` and `issue-deadline` jobs on the Issue's own planning date.
  - The date is read from kata as an instant (the Issue's time zone, else the recurrence's, else the daemon default, else UTC), a deadline `lead_seconds` earlier.
  - A job fires once per date value; when the Issue's date changes the worker follows its events and waits for the new instant instead.
  - A date set or moved into the past after the job was first seen (also after it was cleared) is treated like any missed occurrence under the job's `catchup`.
  - An `execute` job on an Issue date runs its prompt as any prompt job does.
  - The worker also delivers adapter `notify` jobs on an existing Issue, on any trigger: it puts the inbox request with the job's message (the Issue's id and title when blank) in the recipient's inbox, resolving `current-owner-or-author` from the Issue when it sends, and records a succeeded or failed run with no agent started.
  - A zero-lead `current-owner-or-author` notification on an Issue date stays with kata's own date sweeper.
- The worker runs agent prompt jobs on their schedule.
  - A `cron` trigger (five fields or descriptors such as `@daily`, in the trigger's time zone, read by kata's own parser), an `interval` or `@every` one (counted from the job's creation) and a `once` instant fire without a request, while `manual` jobs still run only on `tern-kata run`.
  - `overlap: forbid` holds a job's next occurrences until its earlier run has settled, `allow` starts them as slots free, and occurrences beyond the Project's capacity queue and start oldest first (`worker status` shows the next run and the queue).
  - After downtime or a pause, `catchup` `all` runs every missed occurrence in order, `latest` only the newest, and `skip` only the newest within `grace_seconds`; at most 7 days and 100 occurrences are kept per job, and the oldest overflow is dropped and logged.
  - A run over `timeout_seconds` is stopped and failed with "timed out".
  - Editing, disabling or re-enabling a job in kata changes its schedule within about a second, without restarting the worker, and a changed definition drops the occurrences still waiting.
  - A per-run Issue gets its `scheduled_offset_seconds` and `deadline_offset_seconds` as its Scheduled and Deadline dates, counted from the occurrence.
- `tern-kata worker` runs kata's agent prompt jobs on your machine with no Tern window open.
  - From `worker.toml` (executor label, served Projects with capacity, checkouts, Agent profiles, secret sources, limits) it runs each job's prompt through headless `omp` in the job's checkout, creates the run's Issue (or comments on the job's existing one), and records the run in kata as running, then succeeded or failed with the agent's final message and token counts.
  - The agent is told its Issue and to comment and close it with evidence; an Issue left open gets the final message as a comment and the `needs-review` label.
  - A run over its timeout is stopped and failed.
  - Native jobs and Projects not listed in `worker.toml` never run, a second worker on the machine is refused, and a Project whose lease another worker holds admits nothing.
  - A run whose agent may have started when the worker stopped is held as unknown, never sent again.
  - Anyone who can write jobs in a listed Project can run prompts and commands on your machine; see the README.
- A worker that is stopped or killed and started again on the same state folder never sends a run's prompt twice.
  - Each run's journal is saved before every step, and a run whose agent may have started and whose outcome the worker cannot prove becomes a Held run.
  - A Held run is recorded `unknown` in kata with the reason first in its summary and a comment on its Issue (`launch_unproven`, `interrupted`, `no_receipt`, `group_unproven`, `agent_login`, `agent_quota`, and the nothing-started reasons), with its job's overlap gate closed until you settle it.
  - `tern-kata worker resume <run-uid>` continues it as a new headless omp attempt on the saved session with a prompt that warns the previous turn may have partly run, and the run goes back to running.
  - `tern-kata worker abandon <run-uid>` records it `cancelled` ("abandoned by <actor>") and frees its slot.
  - `worker status` lists each held run with its reason and the commands that settle it, and the worker's control socket answers a run's detail (steps, session, output tail) and resume and abandon requests for a later Pane.
- The tern-kata plugin gains a host half (`host.luau`) that keeps the worker running with no Tern window open.
  - When `worker.toml` exists it starts `tern-kata worker` in Tern's session daemon and starts it again whenever it exits, waiting 1 second, then 2, 4 and so on up to a minute, and again whenever the daemon starts.
  - It finds `tern-kata` and `omp` even when Tern was started with a short `PATH`. With no `worker.toml` it does nothing.
  - `tern-kata worker status` says whether Tern supervises the worker or it was started by hand; an empty `worker.manual` beside `worker.toml` turns supervision off (README, "Supervision").
- `tern-kata run <job-uid>` asks the running worker to run an adapter prompt job now.
- `tern-kata worker status` says what the worker serves, runs and refuses (including a Project another machine's worker owns).
- `tern-kata worker doctor` checks `worker.toml`, checkouts, agent commands, secrets, kata and each Project's lease and says how to fix what is missing.
- The Page bar gains a Runs Page (`4`) listing the Pane's Project's kata runs newest first.
  - Each run is a card with a status badge (`running`, `succeeded`, `failed`, `cancelled`, `unknown`), the job's or workflow's name, the Issue, the executor, start, duration (so far while it runs) and the summary's first line, whoever the executor was.
  - Enter shows the detail: the whole summary message, token counts and the references kata holds; `i` opens the run's Issue on the Issues Page.
  - `r` on the Jobs Page opens the Runs Page on the selected job's runs and `f` narrows or widens the list to the selected run's job.
  - Kata lists 100 runs a page and filters only by job, so `m` reads the next older page.
  - The Workflows Page shows each workflow's latest run on its row and its latest runs in its inspector, from the Project's newest 100 runs.
- An Issue a Cron job made is marked in the list, on the Board and in its detail.
  - Its list row and Board card title start with "◷ " (after "↻ " when it also recurs).
  - Its detail has a Cron job property naming the job (its uid when the job is gone) and the run's scheduled time in the Pane calendar; Enter on the property, or a click, opens the Jobs Page with that job selected.
  - A parent's detail has a Cron jobs property listing its jobs with their state, pause reason and schedule, each one openable the same way, and a closed parent says its jobs are paused.
- The Page bar gains a Workflows Page (`5`) listing the Pane's Project's kata workflows with their step counts, archived ones hidden until `a` shows them.
  - Enter opens an inspector with each step's kind, command or prompt, the steps it waits for (`after review`) and its retries, and the live Cron jobs that run it (each row names them too).
  - `n` and `e` edit a workflow as JSON in the Workflow card or in `$EDITOR`, checked locally against kata's rules (unique step keys, `after` naming earlier steps, retries 0 to 100, command or prompt) before it is sent, with the problem named and the text kept.
  - A save is a Guarded write: a workflow changed meanwhile is not overwritten, and the card shows your text beside the new version with Overwrite and Discard Draft.
  - `x` archives and `r` restores; archiving a workflow a live Cron job uses shows kata's refusal.
- `3` shows the Jobs Page, listing the Pane's Project's Cron jobs, native and adapter, as cards.
  - A card has a state badge (`active`, `disabled`, `archived`, `paused: parent closed`, `paused: parent deleted` or `paused: Project archived`), name, kind (`recurring child`, `agent prompt`, `workflow`, `notify`), schedule in words ("every weekday at 09:00 Europe/Berlin", "every 15 minutes", "once at 2026-11-01 09:00 UTC", "manual", "2 days before the Issue's deadline"), agent profile and next run in the Pane calendar's zone.
  - A schedule that can't be read, or a next run that can't be worked out, is left off.
  - Archived jobs are hidden until `a` shows them, `ctrl+r` and daemon events read the jobs again, and the help sheet lists the Jobs keys.
  - At 140 columns and wider an inspector shows the selected job's full definition and, for a recurring child, its parent Issue and delivery progress; Enter on that parent opens it on the Issues Page.
  - Each card also shows the job's latest run (an adapter job: status, when and the summary's first line) or latest child Issue (a recurring child), and the inspector lists the five most recent.
  - A per-run Issue job that creates more than 24 Issues a day is marked "creates about N Issues a day; consider an existing Issue".
  - The Jobs Page keeps its selection, and the Runs Page its selection, older pages read and job filter, while another Page is shown, resetting them only when the Project changes.
- The Job card makes and edits the jobs a worker runs too.
  - Its Kind field gains Agent prompt, Workflow and Notify beside Recurring child (a job being edited keeps to its own side).
  - Agent prompt has a multi-line Prompt (Enter adds a line); Workflow a picker of the Project's live workflows.
  - Both have an Agent profile (`options.profile`, other options kept), a Checkout (`checkout_key`), Secrets (`ENV=source` pairs, `secret_refs`), the Run Issue policy (a new Issue per run with title, body and scheduled and deadline offsets such as `1d` or `-2h`, or an existing Issue by short id), Overlap and a Timeout.
  - Notify names an existing Issue, a recipient and a message.
  - The profile and checkout fields are pickers of this machine's `worker.toml` (read with the worker's own parser each time the card opens) and free text without one, or with one that can't be read, which the card says.
  - Every trigger is offered to a worker's job: cron, interval (no 60-second floor), once (a full RFC 3339 instant), manual, Issue's scheduled date and, for Notify only, Issue's deadline with a lead; the preview reads once and Issue-date triggers in words.
  - Catch up with grace applies to cron, interval and once triggers.
  - A blank prompt, a workflow that isn't a live one of the Project, an Issue-date trigger or existing-Issue policy with no Issue, a once instant without its offset, a malformed secret or checkout key, a named recipient without a message and a span out of kata's range are refused naming the field and nothing is sent; a target Issue that doesn't exist is refused the same way by the write.
  - The help sheet lists Enter (new line in the prompt) and the kata skill shows how to define the jobs.
- The Job card makes and edits recurring-child jobs.
  - `n` on the Jobs Page opens it for a new job, `J` on an Issue (list, Board, Lane view, Issue detail) for a new job with that Issue as its parent, `e` on the selected job, prefilled.
  - It has a name, a trigger (a cron expression with its time zone, or an interval of at least 60 seconds), the parent Issue's short id, a child title and body, an inbox recipient (blank is the parent's current owner or author) and message, a catch-up choice (latest, every missed run, or skip with a grace) and Enabled; fields that don't apply are hidden and skipped by `tab`.
  - Under the trigger a live preview reads the schedule in words and lists the next three runs.
  - `ctrl+o` saves; a bad cron expression, unknown time zone, interval under 60 seconds, skip without a grace or blank name or child title is refused naming the field and nothing is sent.
  - An edit is a Guarded write: a job changed meanwhile is not overwritten, the card shows the newer job and keeps your draft, and `o`, `u` or `d` save the draft over it, replace the draft with it or discard the draft.
- `p` on the Jobs Page pauses a running job or resumes one that is not, and `x` archives the selected job, or restores it when it is archived (listed with `a`).
  - Each is a Guarded write that leaves a job changed meanwhile alone and says so in a Notice.

#### Daemons and Federation

- `D` opens a Daemons Page, a Lettered Page also listed in the Menu, listing the daemons kata's `config.toml` names.
  - It has kata tui's columns (daemon, endpoint or "local", "token" or "no token", "current") and the selected daemon's name and endpoint under the list.
  - Enter switches this Pane to the selected daemon: the Pane starts over on it (Collection, Pages, caches, every card and form, undo history and event stream) and drops unsent drafts, the Notice saying how many. It changes nothing on disk and leaves other Panes alone.
  - A stopped local daemon is started first, as kata tui does: through `kata --daemon <name> daemon recover` for a profile, which refuses a profile whose storage is gone instead of creating an empty database, and through `kata daemon start` with `KATA_DSN`, `KATA_DB` and `KATA_SERVER` removed for a local entry without a `home`.
  - A failed switch shows `daemon "<name>": <reason>` and keeps the current connection, and with no named daemons the Page says how to add one.
  - The top bar names the current daemon by its catalog name and, when the daemon says who you are, the account.
- `F` opens a Federation Page, a Lettered Page also listed in the Menu, showing this daemon's spoke federations.
  - They sit under "Federation for active daemon: <name> <endpoint> instance <uid> auth <auth>", with each Project's hub, actor, push, pending pushes, last sync and badges, and the selected Project's hub URL and Project id, actor and credential under the list.
  - Enter shows the Project's detail (capabilities, push, credential, `allow_insecure`, cursors, claims, quarantines, violations, last pull, push and sync, last error), `r` reads the status again, and esc or backspace returns.
  - It says "loading federation status...", "no spoke federation enrollments" and "failed to load federation: <error>".
- `x` on a Project (in the list or its detail) opens kata tui's leave preview.
  - `d` switches between detaching (the Project stays standalone with all its Issues) and archiving, and `l` makes the leave local-only.
  - Enter runs it: the spoke's leave preflight, then, unless local-only, listing and revoking this spoke's enrollments on the hub (matched by spoke instance and hub Project; a grant for another instance blocks, a grant not limited to a Project is left and named), then the leave.
  - The hub's token is the one of the daemon catalog entry whose URL is the hub's, and the hub step is unauthenticated without one.
  - A failure shows kata tui's wording under the preview, including "hub revoke failed: … retry with local-only" and the `kata federation revoke <id>` hint, and Enter tries again; success shows a result and Enter or esc returns to the refreshed list.
- `b` on the Federation Page browses the Projects of the daemon catalog's hubs, read only.
  - The hubs are every entry that is neither the active daemon nor local, each read with that hub's own token; esc or backspace goes back, and it says "no catalog hub daemons configured" when there are none.
- `n` on the Federation Page enrolls a Project in a hub's federation, as kata tui does.
  - Choose a local Project (a new replica of a hub Project, or an existing Project to adopt), a hub (the active daemon and local entries are refused) and a hub Project, then a preview that names what blocks it. Adopting asks you to type the local Project's exact name first.
  - Enter runs it: on the hub the Pane ensures the Project (same-name adoption only), enables federation and creates the enrollment, reads the hub Project's federation metadata with the enrollment's token, and creates the replica on the spoke.
  - A result follows, and Enter or esc returns to the list.
  - A failure after the hub granted the enrollment keeps a recovery, where `R` shows the `kata federation join …` command with its token, only when asked.
- `C` opens a Credentials Page, a Lettered Page also listed in the Menu, listing the daemon's API tokens.
  - Tokens are newest first, with state, id, name, actor, scope, created, expires, last observed use and revoked in UTC under "N credentials · observed at <time>".
  - It says "credential audit unavailable for this connection" when your account lacks token audit read, and refreshes every 30 seconds; `r` refreshes at once or retries a failed check.
  - esc or backspace returns to the Page you came from. The Page bar draws a Lettered Page only while it is shown.
- `n` on the Credentials Page creates an API token, as `kata tokens create` does, in the Token card.
  - The card takes actor (prefilled with yours), optional name, "expires in" (a positive whole-second duration, such as `720h`; required for an Issue-subtree token), scope (`account`, or `issue` with an Issue given as a short id, `#id` or `project#id`, which the Pane resolves to its Project and root Issue), optional teams (names or uids, at most 256) and the token file.
  - The secret is written to that new file, which only you can read (on Windows only your account), and is never drawn, logged or kept in the Pane.
  - An existing path is refused before anything is sent, and the card then names the file and the token's id.
- `x` on a token on the Credentials Page revokes it.
  - It asks "Revoke token <id> (<name>, <actor>)? This cannot be undone." and `y` revokes it.
  - The list is read again after either create or revoke, and kata's refusals (identity mode off, scoped tokens unsupported, a spoke replica, no permission) show on the card.

### Changed

#### Requirements

- tern-kata requires the kata v0.20.0 daemon, for the Pane and the Worker alike.
  - kata v0.20 says cron peers must upgrade together and does not support v0.19 beside it.
  - Against an older daemon the Pane refuses to connect and says to upgrade, and the Worker and `worker doctor` name v0.20.0.

#### Install and update

- tern-kata builds against kata v0.20.0 and its lean Go client, so the release binary is smaller.
  - The release binary is about 27 MB instead of about 38 MB and its download about 13 MB instead of about 19 MB.

#### Attachments

- Attaching a file moves from `u` to `@` in the list, on the Board, in the Lane view and in the Issue detail, because `u` undoes now.
  - `U` still lists and removes Attachments. An Issue with no Attachments says "No attachments, @ to attach".
- An Attachment pointer is now the `tern-kata.attachment.<name>` meta key, because kata's native attachments claim `attachment.<id>`.
  - tern-kata moves pointers written under `attachment.<name>` by itself, once per Project and kata daemon on each machine, when a Pane for the Project starts or `tern-kata attach` or `detach` runs.
  - It leaves kata's own `attachment.*` keys and the files alone.

#### Issues and Pages

- Field filters (project, status, owner, label, relationship) now keep the Collection's grouping and headings.
  - Today with a status filter still shows Overdue and Today; only a text search shows the flat Results list.
- The Issue detail's heading is the bare `#id` in a Project Collection, since the top bar names the Project.
  - It names the Project, `project#id`, only in Collections that span Projects (Inbox, Today, All Open and the others).
- Following a ticket of another Project from a Project Collection switches the Pane to that Project instead of opening the ticket as a stranger in the current one.
  - The Pane keeps its Page and selects the ticket, the top bar reads "Project: <that Project>", and Esc goes back through the jump history to the earlier Collection, Page, Lane view, selection and focus, one step per hop.
  - This covers an id on a Board card's Relationships line, the detail's Links and children rows, `^`, and a `kata://` or `https://kata.invalid/` link; following inside a Project and from Collections that span Projects works as before.
- The top bar's title ("Project: tern-kata", or the Collection's name) keeps the accent colour whichever column has focus, so following a link into the detail no longer dims it.
  - The columns' borders show focus.

#### Jobs and worker

- `worker status` reasons for a Workflow that breaks kata's own rules (version, step keys, retries, `after`, step shape, options) now use the Workflow card's wording.
  - The worker also applies the step-key and options rules (no credentials or local paths) to the Workflow it reads.

#### Daemons and Federation

- The top bar names a local or remote daemon without `kata` on `PATH`.
  - The kind comes from kata's own discovery, and connecting starts no `kata` process.

#### Command line

- An explicitly empty `--as` (`--as=` or `--as ""`) is refused with "--as must be non-empty" for the Pane launch, `attach`, `detach`, `recur`, `run`, `worker resume` and `worker abandon`, where it used to act as the fallback actor.
  - Without `--as` the actor is still `$KATA_AUTHOR`, `$USER`, git `user.name`, then "anonymous", and an empty variable still counts as unset; `worker init --actor` is unchanged.

### Fixed

#### Attachments

- On Windows, two attaches that start at the same moment in a checkout with no Attachments yet both succeed.
  - One used to fail with "Access is denied" while the other wrote `.kata-attachments/.gitignore`.

#### Issues and Pages

- Beside each other, the Issue list and the Issue detail scroll inside their columns down to just above the footer at any Pane height, instead of stopping a little past half the window.
  - They also follow a Pane made taller or shorter without changing its width.
- Backspace with the caret at the start of the Search field does nothing; it no longer resets the selection and runs the search again.
- Scrolling the Issue detail (`↑`, `↓`, Page Up, Page Down) while it shows an error line or an Issue that is no longer available does nothing instead of ending the Pane with "Tern surface: unknown id main.columns.detail.viewport.content".

## [0.3.0] - 2026-10-20

### Added

#### Agents

- Two agent skills ship with tern-kata: `kata` for working with the kata CLI and `tern-kata` for reading and attaching Attachments.
  - `npx skills add Dangerboi/tern-kata -g` installs them (`bunx` works too).
  - The install scripts offer to run it when they run in a terminal, with `bunx` when Node.js is missing.
- The kata and tern-kata skills tell agents to use `tern-kata recur`.

#### Attachments

- `tern-kata attach <ref> <file>...` attaches files to an Issue from a shell or an agent, without Tern.
  - It prints each stored absolute path.
  - It needs the directory to be bound to the Issue's Project and exits 1 on any failure.
- `tern-kata detach <ref> <name>` removes an Attachment from an Issue from a shell or an agent, without Tern.
  - It needs the directory to be bound to the Issue's Project and exits 1 on any failure.
- An Attachment is a copy of the file in `.kata-attachments` at the root of the Project's primary checkout.
  - Every linked worktree shares that folder, and it stays out of git; an empty `.gitignore` there is rewritten with `*` on the next attach.
  - An `attachment.<name>` meta key on the Issue holds the copy's host and path.
- Detach clears the Attachment's meta key and deletes the copy.
  - It clears the key, including one with an unreadable value, and deletes the copy only when it is on this machine and in that Issue's folder, saying what became of the file.
  - It clears the key only while it still holds what was read: a pointer rewritten meanwhile survives with its file, and the refusal says the Attachment changed.
  - When the key is cleared but the copy can't be deleted, `U` closes the form and `tern-kata detach` prints "Removed <name>; could not delete <path>: <reason>" on stdout, then exits 1 with the error on stderr.
- The Issue detail has an Attachments tab after Links.
  - It lists the Issue's Attachments by name, each on its own row whatever the name.
  - A file held on this machine is a link to the file (one on a Windows network share links as `file://server/share/...`), one held elsewhere reads "<name> on <host>", and an Issue with none says "No attachments, u to attach".
- `u` on the selected Issue, in the list or the Issue detail, asks for a file's path and attaches it.
  - A successful `u` shows "Attached <stored name> to <short id>", so a file stored as `shot-2.png` is named.
  - Attaching is refused when this machine's host name can't be read, and a pointer with an empty host is never treated as on this machine.
- `U` on the selected Issue lists the Issue's Attachments by name and removes the one chosen, and says so instead of opening a picker when there are none.
- `u` and `U` explain why when the directory is not bound to the Issue's Project.
  - The footer and help sheet list them beside the label keys.

#### Issues and Pages

- `tern-kata recur <ref>` prints an Issue's schedule in words with the next Occurrence's date.
  - With `--frequency daily|weekly|monthly|yearly` (and `--every`, `--on`, `--day`, `--month`, `--until` or `--count`, `--start`, `--zone`) or `--rule <rrule>` it makes the Issue recur or changes its schedule, and `--stop` stops it, from a shell or an agent without Tern.
  - The flags go through the Recurrence card's own checks, so invalid input is refused with the card's messages and nothing is sent; a schedule changed meanwhile is printed, not overwritten.
  - Refs, `--as`, the Binding rule and exit codes match `attach`.

## [0.2.0] - 2026-10-19

### Added

#### Install and update

- Install scripts (`install.sh`, `install.ps1`) download the latest release, verify its checksum and install the window plugin when `tern` is on `PATH`.
- The **Install tern-kata** palette command runs the install scripts in a new Pane.

#### Issues and Pages

- `R` on a comment in the Comments tab opens the comment Draft as a reply.
  - `ctrl+r` cycles the kind (reply, confirm, refute, supersede), and a confirm or refute under 40 characters is refused before it is sent.
  - A duplicate reply shows kata's message and offers to send anyway.
- Replies in the Comments tab show their kind, whose comment they answer and, for a comment with replies, how many.
  - `enter` on a reply jumps to the answered comment, even in another Issue.
- `O` on the selected Issue opens the Recurrence card.
  - It makes an Issue recur with a form for frequency, interval, weekdays, monthly and yearly day, Start date, end (a date or a number of occurrences) and time zone.
  - It changes or stops the recurrence of an Issue that already recurs.
- The Recurrence and Filter cards respond to clicks on their choices, and the Recurrence card's Every field names its unit.
- The Issue detail reads a recurrence in words with the next date ("Weekly on Mon, Thu · next 2026-10-26"), or "stopped", and recurring Issues carry a marker in the list.
- An owner with a timed claim reads "owner until 14:30" wherever an owner shows (list, children table, Issue detail).
- `I` opens the Inbox. `ctrl+n` and `ctrl+p` move a row, and `ctrl+v` and `alt+v` page.
- The children table has an owner column, a header row and alternating shading.

#### Command line

- `tern-kata --help` lists the flags.

### Changed

#### Requirements

- tern-kata requires the kata v0.19.0 daemon.
  - Against an older daemon the Pane refuses to connect and says to upgrade.

#### Issues and Pages

- The Issue list draws each id in its Lane's Board colour (Done's for a closed Issue) and each owner in the user colour, as the Board does.
  - Where a list span has no such colour it takes the nearest text colour.
  - A Project's Lane list now loads when the Project opens, not only when the Board turns on.
- The Issue detail uses the Board's colours.
  - The heading's `#id` takes the Issue's Lane tone (Done's for a closed Issue), the owner is a user badge, and labels are info badges.
  - Scheduled and Deadline are badges toned by their own date under the Date badge rules (muted on a closed Issue or before the Pane calendar's date is known).
- Saving a Draft and posting a comment or reply conflict only when the Issue's text changed.
  - An agent claiming the Issue, a claim expiring or an owner change no longer causes a conflict.
  - Closing succeeds while the Issue is open, and if someone closed it first the Close card says so.
- The Logbook, Today, Overdue, Upcoming and Deadlines groups, detail times and the Recurrence card's Start date use your local time zone.
- Child counts arrive with the Collection, so a live refresh downloads nothing extra and a failed count no longer discards a Collection that loaded.
- The footer shows each place's hints in kata tui's order and words and wraps them into a grid instead of hiding the ones that do not fit.
  - The Issue detail footer adds label, owner, parent and priority hints.
- Going back after a jump returns to the tab and row you left.
- Compact activity lists the oldest entry first.
- Every card button names its keys, for example "Keep editing (n / esc)", including the Filter card's Apply and Cancel.
- Comments, replies, new Issues and closes can be sent again safely after a lost answer, without duplicating.
- A Recurrence change that keeps losing a race to another edit says so in plain words.

#### Daemons and Federation

- The Pane finds the daemon the way the kata CLI does, so it works without `kata` on `PATH`.
  - Connecting gives up after 10 seconds instead of hanging.
- A workspace bound to a Project the daemon does not know says to run `kata init`, for local and remote daemons alike.
  - Only an unbound workspace opens the Inbox.

#### Tern window

- A kata link opened in Tern reuses a kata Pane only in the current tab.

### Fixed

#### Install and update

- `install.ps1` parses, keeps `%VAR%` entries in `PATH` and says when it changes `PATH`, so programs started from Explorer, such as Tern, see it.

#### Issues and Pages

- Today and the other date groups move to the new day at local midnight, also when the machine slept through it.

#### Tern window

- **Open kata**, `F9` and kata links open tern-kata in a new tab, in the focused Pane's directory, when Tern can't split the focused Pane, instead of doing nothing.

#### Command line

- Text written in `$EDITOR` is kept in a file, and its path is printed, when Tern cannot be reached after the editor closes.
- `--as` and `--issue` refuse a value that is another flag, so `--as --issue x` no longer starts tern-kata as the actor `--issue`.

[Unreleased]: https://github.com/Dangerboi/tern-kata/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/Dangerboi/tern-kata/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/Dangerboi/tern-kata/compare/v0.7.2...v0.8.0
[0.7.2]: https://github.com/Dangerboi/tern-kata/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/Dangerboi/tern-kata/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/Dangerboi/tern-kata/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/Dangerboi/tern-kata/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/Dangerboi/tern-kata/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/Dangerboi/tern-kata/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Dangerboi/tern-kata/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Dangerboi/tern-kata/compare/v0.1.1...v0.2.0
