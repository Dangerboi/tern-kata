# Workflows

`5` in a Project opens the Workflows Page. It shows the Project's kata **Workflows**, one row each with its name and step count. Archived ones stay hidden until `a` shows them.

The Project is found as the [Board Page](pages-and-board.md#board-page) finds it, and the Page ends when you leave the Project Collection. Esc closes the steps first, then goes to the Menu.

![The Workflows Page with a workflow selected and its steps shown](../images/workflows.png)

Each row names the live Cron jobs that run the workflow and shows the workflow's latest run; the inspector lists its latest runs (see [Runs](runs.md#reading-and-filtering)).

## Keys

| Key | Action |
|---|---|
| Enter | Open the inspector for the selected workflow |
| `n` | New workflow (opens the Workflow card) |
| `e` | Edit the selected workflow |
| `ctrl+e` | In the card, edit the JSON in `$EDITOR` |
| `ctrl+o` | In the card, save |
| `x` | Archive the selected workflow |
| `r` | Restore an archived workflow |
| `a` | Show or hide archived workflows |

## Inspector

**Enter** opens the inspector for the selected workflow: each step in order with its kind (`command` or `prompt`), its command or prompt, the steps it waits for (`after review`) and its retries, the live Cron jobs that run it, and who wrote it. A row also names those jobs. The inspector sits beside the list at 140 columns and wider, in place of it in a narrower Pane.

## Workflow card

`n` and `e` open the **Workflow card** on a new workflow or the selected one, as the JSON kata takes:

```json
{"name": "...", "definition": {"version": 1, "steps": [{"key": "review", "kind": "prompt", "prompt": "..."}, {"key": "test", "kind": "command", "command": "make test", "after": ["review"], "retries": 1}]}}
```

Edit it in the card or press `ctrl+e` for `$EDITOR`; `ctrl+o` saves.

Saving checks the text first, and nothing is sent while it fails. The check requires:

- valid JSON and a name;
- version 1;
- at least one step;
- unique step keys of letters, digits, `-`, `_` and `.`;
- `after` naming earlier steps only (so steps are listed in the order they run);
- retries from 0 to 100;
- exactly one of `command` or `prompt`, matching the step's kind.

The card names the problem and keeps your text.

A save is a Guarded write on the version you started from. If the workflow changed meanwhile, nothing is written: the card shows your text beside the workflow as it stands now, `o` saves yours over it and `d` drops your text.

## Archive and restore

`x` archives the selected workflow and `r` restores an archived one, both guarded the same way. kata refuses to archive a workflow a live Cron job uses, and the Pane shows its message: "workflow W is used by live job J; update or delete that job first".

To run a workflow on a schedule, make a job whose kind is Workflow on the [Jobs Page](jobs.md). How [the worker](worker-reference.md#workflow-runs) runs the steps is in the worker reference.
