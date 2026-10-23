# Agents

Two things help coding agents work with tern-kata: a rule that makes ticket references clickable, and two agent skills.

## Clickable ticket links from agents

Tern draws `kata://` links in chat as plain text, so agents should write ticket references as `https://kata.invalid/...` links, which the plugin intercepts. Clicking one focuses the kata Pane in the current tab (or opens one there) and selects that Issue; a kata Pane in another tab is left alone. When the Issue belongs to another Project than the one the Pane shows (a Project Collection), the Pane switches to that Project, on the same Page, and Esc goes back to where it was; in Inbox, All Open and the other Collections that span Projects it stays where it is.

To make your agent do that in every kata-tracked repo, add this to its global rules file (`~/.omp/agent/RULES.md` for OMP):

```markdown
In a repo with a `.kata.toml`, write kata ticket references in chat replies as `[<id>](https://kata.invalid/<project>/<id>)`, with `<project>` the name in `.kata.toml`. The tern-kata window plugin opens them in tern-kata; Tern draws `kata://` links in chat as plain text. Everywhere else (files, commit messages, kata Issue text) write the bare id; those links only work inside Tern.
```

The links only work when your chat is shown in Tern with the window plugin installed. The [README's agent install steps](../../README.md#install-with-an-agent) offer to add this rule for you.

## Agent skills

The repo ships two skills for coding agents:

- `kata`: the kata CLI, for finding, creating, claiming, commenting on, linking and closing Issues, and coordinating subagents through them.
- `tern-kata`: reading an Issue's [Attachments](attachments.md), attaching files with `tern-kata attach`, making Issues recur with [`tern-kata recur`](recur.md), setting up the worker with `tern-kata worker init` and `worker add`, running agent jobs now and checking the [worker](worker.md).

Install them for the agents on this machine with the [skills](https://github.com/vercel-labs/skills) CLI, which asks which skills and which agents. It needs Node.js; with only Bun installed, use `bunx` in place of `npx`:

```sh
npx skills add Dangerboi/tern-kata -g
```

The install scripts offer the same step when run in a terminal with `npx` available.
