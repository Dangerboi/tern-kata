# Agents

Three things help coding agents work with tern-kata: the Agent card, which starts an agent on an Issue and keeps track of it (with the `agent.toml` that configures it, Background agents and Attention requests), a rule that makes ticket references clickable, and two agent skills.

## Agents on an Issue

### The Agent card and starting agents

`w` on the selected Issue (list, Board, Lane view or Issue detail) opens the Agent card, with or without agents. It lists the Issue's agents with state, start time and where each is (split, tab, background or open). `enter` goes to the selected one, `b` sends it to the background, `i` interrupts it, `x` stops it, `esc` cancels.

![The Agent card for "Add Apple Pay to checkout" with no agents yet and its buttons: New in a split (S), New in a tab (T), New in the background (G), plus Go to agent, Send to background and Cancel](../images/agent-card.png)

`s` starts a new interactive `omp` in a split right of the kata Pane, `t` in a new tab right of the kata Pane's tab (in that Pane's Tern session, with the plugin's window half loaded; otherwise the tab opens last), `g` in the background. Each starts in the Issue's checkout: the Binding's workspace, else the Project's `checkout` in `agent.toml`, else the first checkout `worker.toml` lists for it, else a Notice says to add one.

A closed Issue says "This Issue is closed; reopen it first (r)." and opens no card.

### Interrupting and stopping an agent

`i` interrupts the selected agent's current turn, as ctrl+c does in its block: the block and the conversation stay, and the badge reads idle once the turn ends. Use it when an agent is on the wrong path, then message the correction. Interrupt goes through the plugin's window half, since only it can interrupt a Tern agent, so it needs the plugin loaded in the window that shows the Pane. It is off, with the reason in a tooltip and a Notice, for an agent that is not omp or that Tern does not list.

`x` asks "Stop <agent> on <ref>? Its block closes and any unsaved work in it is dropped." and `y` stops it: `tern close` on its block, then its entry and prompt file go. It works on an agent in a split, a tab or the background, and on one another kata Pane on this host started; the Pane reads `agents.json` again just before it closes. The Issue gets what an agent that exits gets: while it is open and the Pane started the agent, it holds the request "agent exited without closing <ref>". A close that fails shows Tern's words in a Notice and leaves the entry. Both actions are off for an agent that has exited.

The agent is told which Issue to work on and how to leave a record: claim it for 24 hours (renewing the claim when it already owns the Issue), comment what it did and how it checked, finish or drop its todo children before closing, close it with evidence only when done and verified, and ask you when it needs a decision. If it stops without closing, it comments why, sets `work.attention` to `needs-human` with the reason in `work.attention_msg`, and labels the Issue ready-for-human in place of ready-for-agent.

### agent.toml

`agent.toml` sets the actor, command and agent name, for every Project or for one. You write it by hand in tern-kata's config folder beside `lanes.toml`. It is read on every start.

```toml
actor = "me-agent"        # optional; without one the agent writes to kata as whoever kata resolves in its shell (KATA_AUTHOR as inherited)
name = "builder"          # optional; the teammate its Attention requests carry, default the command's program name
command = ["omp"]         # optional argv; the Pane appends @<prompt file>, so it must accept omp's @file message
[projects.tern-kata]      # optional, by Project name; overrides the values above
actor = "..."
name = "..."
command = ["omp", "--model", "opus"]
checkout = "/home/me/Code/tern-kata"   # an absolute directory
```

An unknown key, a relative `checkout` or an empty `command` refuses the start and shows the error. The Pane remembers the agents it started in `agents.json` in its state folder and forgets the ones whose block has closed.

### How the agent runs

The agent always runs through `tern-kata agent [--as <actor>] [--address-prefix <actor>/<name>] --ref <qualified ref> --`. For a daemon other than the default one, the Pane also passes the route flags it needs to reach that daemon: `--home`, `--db`, `--schema`, `--dsn-file`, `--env-file`, `--daemon`, `--server`, `--token-file`, `--auth-file`, `--allow-insecure` and `--trust-private-network`, each only when it has a value. No flag holds a secret: a token or DSN goes through a private file the wrapper reads and deletes. The wrapper:

- sets `KATA_REF` to the Issue, so kata-plugin's todo plan hangs under it, and drops an inherited `KATA_REF` and `KATA_INBOX_USER` (your own inbox is not the agent's);
- sets `KATA_AUTHOR` when there is an `actor`, and otherwise leaves `KATA_AUTHOR` as inherited;
- for an omp agent (the Pane passes `--address-prefix`), sets `KATA_INBOX_USER` to the agent's own address, `<your actor>/<agent.toml name>-<block id>` (the block id from `TERN_PANE`), and `KATA_OMP_LIVE_INBOX=0`, and omp loads tern-kata's extension with `-e`, or with `--trusted-extension` when the agent's args hold one (see [Messages to an agent](#messages-to-an-agent)).

`tern-kata` must be on the `PATH` of Tern's shell on that machine, and must be this version: an older one refuses `--ref` or `--address-prefix` and the agent exits at once.

**The Pane's daemon.** A Pane on the default daemon starts agents as before. When the Pane's daemon is not what Tern's shell would pick (the Pane has `KATA_HOME` or `KATA_SERVER`, or you switched on the Daemons Page), the wrapper first drops the block's own `KATA_SERVER`, `KATA_AUTH_TOKEN`, `KATA_AUTH_TOKEN_FILE`, `KATA_ALLOW_INSECURE`, `KATA_TRUST_PRIVATE_NETWORK`, `KATA_DSN`, `KATA_DB` and `KATA_POSTGRES_SCHEMA`. It then sets `KATA_HOME` (made absolute), `KATA_SERVER`, `KATA_ALLOW_INSECURE`, `KATA_TRUST_PRIVATE_NETWORK`, `KATA_AUTH_TOKEN` and `KATA_AUTH_TOKEN_FILE` as the Pane has them. That is how the agent's `kata` commands reach the daemon `KATA_REF` names: its Issue is claimed, commented on and closed there, and kata-plugin registers nothing on Tern's default daemon.

- The Pane's `KATA_DB`, `KATA_POSTGRES_SCHEMA` and `KATA_DSN` are carried only when the route keeps the Pane's home and is not a `KATA_SERVER` or switched-remote route.
- The `token_env` values of `config.toml` are carried under their own names, never as one global token: every `[[daemon]]` and `[auth]` one for a home-only or plain local route, only `[auth]`'s for a `KATA_SERVER` route, none for a switched remote entry (it gets its own token) or a profile with its own home. Each reaches only the daemon kata scopes it to.
- Tokens, a DSN (it may hold a password) and these values travel in private files beside the prompt file, deleted once read, never on the command line.
- A remote daemon wins over the checkout's `.kata.local.toml` pin.
- A switch to a local daemon that the checkout's pin or `active_daemon` would override, or to a remote entry without a token while `[auth]` in `config.toml` holds a credential, is refused with the reason in the "Cannot start an agent" Notice.

### Agent badge

The Agent badge on an Issue's list row, Lane row, Board card and detail shows "agent working", "agent waiting for you", "agent idle" or "agent exited", with " · N" when the Issue has several agents. On the Board card and in the detail, a click focuses the agent.

### Background agents

A Background agent's block goes in a tab of a hidden Tern session called "kata agents" (made when it does not exist), named after the Issue.

- It opens beside the Pane when you click its Agent badge in any kata Pane, or focus it in the hidden session, as Tern's notification for an agent waiting for you does (with the plugin's window half loaded). Nothing else brings it out.
- Once it is open and you answer it (it works again after waiting for you), the Pane that started it moves it back into the hidden session, wherever it was opened.
- One you have not opened ends by itself once its Issue is closed and it is idle or has exited: the Pane that started it closes its block with `tern close` and drops its entry and prompt file, unless you have it open in any kata Pane. A close that fails shows Tern's words in a Notice and leaves the entry.
- An agent that goes idle or exits while open stays open. An agent in a split or tab is closed this way only once a kata Pane has sent it to the background.
- Sending an agent back records it as a Background agent even when a later move step fails, and the Notice still shows Tern's error.

### Attention requests and Work attention

While an agent you started waits for you, the Issue holds an Attention request for your Inbox address with the agent's last message, also when the Pane finds the agent already waiting. It clears when no agent on the Issue waits and when the Pane quits; a newer request someone else wrote stays.

- For an omp agent, the message is the question of its pending ask call, else its last message. An omp agent with no text to show gets "agent waiting for you". An agent that is not omp keeps its last message.
- The request names the agent's `agent.toml` name (by default its program, such as `omp`) as its teammate, so the [Attention Page](pages-and-board.md#attention-page) shows `actor/<name>`. An Issue holds one request for you, so the agent that waited last is the one named.
- An agent that exits while its Issue is still open, also when you quit it with `/exit`, leaves the request "agent exited without closing <ref>" until the Issue closes, another agent on it waits or the Pane quits.

Work attention is the Issue's own `work.attention`. An open Issue whose `work.attention` is `needs-human` or `stuck`, written by an agent, kata-plugin or anyone else, shows `needs human` or `stuck` beside the Agent badge on list and Lane rows and as a badge on the Board card, and the word with its whole message as the Work attention property of the Issue detail. `ok`, no value and an unknown value show nothing.

### Messages to an agent

`m` on the [Attention Page](pages-and-board.md#attention-page) opens the Message card for the agent behind the selected request: the agent on that Issue this host's Panes started whose name the request carries, the newest that waits for you, else the newest. Type the message (at most 1024 bytes) and press `enter`; `esc` cancels.

- **Where it goes**: the message is a kata Attention request from you on the agent's Issue, addressed to the agent's own address, `<actor>/<name>-<block id>` (for example `dana/omp-18494129176579`). No other agent reads that address, on the same Issue or another, and `kata inbox` output names the agent.
- **How it arrives**: tern-kata's omp extension, which every omp agent tern-kata starts loads, follows kata's event stream and hands the message to the agent at once: an agent waiting for you or idle starts a turn on it, a working one gets it at its next step between tool calls without its work being stopped. Text half typed in its prompt stays as it was. A question it waits on stays: a pending `ask` is neither answered nor lost, and a pending tool approval is neither approved nor denied; the message lands after you answer. Your own Attention request ("agent waiting for you") stays until you answer, as before.
- **What the agent sees**: who sent it, the Issue, the message, and the command that clears it, `kata notify <ref> --to <address> --clear`. The request stays until the agent clears it. The extension delivers only requests from the actor that started the agent, and the agent's first message tells it to act only on those; a request someone else writes to the address is never delivered.
- **An unread message**: kata keeps one request per Issue and address. When the agent has not cleared the last one, the card shows it and asks before replacing it (`enter` replaces it, `esc` goes back to your message).
- **When it is off**: `m` opens no card and says why for an agent that has exited, an agent whose command is not omp (only omp loads the extension), an agent started by an older tern-kata, and a request no agent raised.
- **After the agent**: when an agent exits or its block closes, the Pane that started it clears whatever is left at its address.
- **The extension**: it ships in the binary and is written to `omp/tern-kata.ts` in tern-kata's state folder on each start, replaced when its content changes; a running agent keeps the one it loaded. It needs `kata` on the agent's `PATH`, as the agent does. It loads even under `--no-extensions`.
- **With kata-omp installed**: kata-omp lists the message in its inbox section of the agent's system prompt too, until the agent clears it, so the agent sees it twice; that does no harm. The wrapper's `KATA_OMP_LIVE_INBOX=0` tells a kata-omp that delivers messages live itself to leave that to tern-kata's extension.

### Transcript links on closes

kata can record which agent session closed an Issue, and link it to [AgentsView](https://agentsview.io), when `kata close` runs with `[close.transcript]` turned on in kata's `config.toml` (in `KATA_HOME`, `~/.kata` by default):

```toml
[close.transcript]
enabled = true
agentsview_url = "http://127.0.0.1:8080" # optional: also store a link to the session
```

- **What tern-kata does**: its omp extension sets `KATA_TRANSCRIPT_AGENT=omp` and `KATA_TRANSCRIPT_SESSION_ID` to the omp session's id when the session starts, and again when it switches or branches, so every `kata close` the agent runs names the session it ran in. kata stores the agent, the id and, with `agentsview_url`, the link `<agentsview_url>/sessions/omp/<id>` with the close. Without `[close.transcript]` kata ignores both variables. A subagent inside the agent leaves the agent's session named.
- **Which agents**: every omp agent the Pane starts (the same agents [messages](#messages-to-an-agent) reach), and every worker run whose profile `command` is omp. The extension is written as `tern-kata.ts`, into the state folder's `omp` folder for the Pane's agents and into the run folder for a worker run, and loaded with `-e`, or with `--trusted-extension` when the agent's `args` (or a profile's `extensions` list) trust a path, as omp refuses the two together. It loads under `--no-extensions` as well.
- **Worker runs in AgentsView**: a run keeps its omp session in its run folder (`--session-dir`), not in `~/.omp/agent/sessions`, where AgentsView looks for omp sessions, so the stored link may not open until AgentsView reads that folder too. The agent and session id are recorded either way.
- **Checked with**: omp 18.16, which reads the environment an extension sets at session start into the shell its agent's commands run in.

## Clickable ticket links from agents

Tern draws `kata://` links in chat as plain text, so agents should write ticket references as `https://kata.invalid/...` links, which the plugin intercepts. Clicking one focuses the kata Pane in the current tab (or opens one there) and selects that Issue; a kata Pane in another tab is left alone. When the Issue belongs to another Project than the one the Pane shows (a Project Collection), the Pane switches to that Project, on the same Page, and Esc goes back to where it was; in Inbox, All Open and the other Collections that span Projects it stays where it is.

To make your agent do that in every kata-tracked repo, add this to its global rules file (`~/.omp/agent/RULES.md` for OMP):

```markdown
In a repo with a `.kata.toml`, write kata ticket references in chat replies as `[<id>](https://kata.invalid/<project>/<id>)`, with `<project>` the name in `.kata.toml`. The tern-kata window plugin opens them in tern-kata; Tern draws `kata://` links in chat as plain text. Everywhere else (files, commit messages, kata Issue text) write the bare id; those links only work inside Tern.
```

The links only work when your chat is shown in Tern with the window plugin installed. The [README's agent install steps](../../README.md#install-with-an-agent) offer to add this rule for you.

## Agent skills

The repo ships two skills for coding agents:

- `kata`: the kata CLI, for finding, creating, claiming, commenting on, linking and closing Issues, reading and adding [Attachments](attachments.md) with `kata docbank attachments`, and coordinating subagents through them.
- `tern-kata`: making Issues recur with [`tern-kata recur`](recur.md), running an agent job now with `tern-kata run`, checking the [worker](worker.md) (`worker status`, `worker doctor`), and resuming or abandoning its held runs. Setting the worker up (`worker init`, `add`, `edit`) is left to when you ask; agents never stop or reconfigure it unasked.

Install them for the agents on this machine with the [skills](https://github.com/vercel-labs/skills) CLI, which asks which skills and which agents. It needs Node.js; with only Bun installed, use `bunx` in place of `npx`:

```sh
npx skills add Dangerboi/tern-kata -g
```

The install scripts offer the same step when run in a terminal with `npx` available.
