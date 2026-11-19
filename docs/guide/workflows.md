# Workflows

`4` in a Project opens the Workflows Page. It shows the Project's kata **Workflows**, one card each with its name and step count. The heading counts them (`Workflows 2`) and, while archived ones are hidden, says how many (`2 archived hidden`); `a` shows them. With none, the Page reads "No workflows" / "A workflow is a list of steps a job runs." with a **New workflow** button; when every workflow is archived it reads "No workflows" / "All of them are archived." with a **Show archived** button.

The Project is found as the [Board Page](pages-and-board.md#board-page) finds it, and the Page ends when you leave the Project Collection. From 140 columns the selected workflow's steps sit beside the list from the first frame, as the Issues Page shows its list and detail. In a narrower Pane the list is alone until Enter opens the steps in its place, and Esc closes them first. Otherwise Esc ends a search, then goes to the Menu.

![The Workflows Page with a workflow selected and its steps shown](../images/workflows.png)

Each card names the jobs that run the workflow (`Used by Hourly report +1`) and shows its latest run (`succeeded · 2026-11-17 10:00`); the inspector lists its latest runs (see [Runs](runs.md#reading-and-filtering)).

## Keys

| Key | Action |
|---|---|
| Enter | In a Pane narrower than 140 columns, open the inspector for the selected workflow |
| `n` | New workflow (opens the Workflow card) |
| `e` | Edit the selected workflow |
| `ctrl+e` | In the card, open the whole workflow as JSON in `$VISUAL`, else `$EDITOR`, else `vi` |
| `ctrl+o` | In the card, save |
| `ctrl+n`, `ctrl+x` | In the card, add a step after the shown one, remove the shown step |
| `ctrl+up`, `ctrl+down` | In the card, move the shown step up or down |
| `x` | Archive the selected workflow |
| `r` | Restore an archived workflow |
| `a` | Show or hide archived workflows |
| `/` | Search the workflows: type to keep the workflows whose name, step count, using jobs or latest run holds the text (case-insensitive); Enter keeps the search and returns to the list, Esc ends it and brings every workflow back. An empty result says so under the search |

## Inspector

The inspector shows the selected workflow: each step in order with its number and key, its kind in words (`command` or `prompt`), its command or prompt, the steps it waits for (`after review`) and its retries (`retries 2`), the jobs that run it, its latest runs, and `Created by <author>`. A card also names those jobs. Above the steps a line says whether this machine's worker can run the workflow, or lists what it would refuse.

## Workflow card

`n` and `e` open the **Workflow card** on a new workflow or the selected one. It is a form:

- **Name**, **About** and **Input** are the workflow's own texts.
- **Step** chooses the step whose fields are shown below it (`1 review`, `2 test`); the other fields belong to that step: **Step name**, **Kind** (command or prompt), the **Command** or **Prompt** in a multi-line editor, **After** (the earlier steps it waits for, separated by commas) and **Retries** (0 to 100).
- **Add step** puts a new prompt step after the shown one, **Remove step** takes the shown step out (the last one stays), and **Move up** and **Move down** reorder it. Tab moves between fields; left and right change the chosen step or kind.

`ctrl+o` saves. The card's other buttons are **Open in editor** and **Cancel**; leaving a changed form asks "Discard this Draft?" with **Discard Draft** and **Keep editing**.

The options kata accepts on a workflow and on a step have no field. The card carries them through a save unchanged, and **Open in editor** shows the whole workflow as the JSON kata takes, for anyone who wants to change them:

```json
{"name": "...", "definition": {"version": 1, "steps": [{"key": "review", "kind": "prompt", "prompt": "..."}, {"key": "test", "kind": "command", "command": "make test", "after": ["review"], "retries": 1}]}}
```

What the editor returns is read back into the form. Text that is not valid JSON is refused in the card's foot and the form stays as it was; text that reads as a document but breaks a rule is checked when you save.

Saving checks the form first, and nothing is sent while it fails. The check requires:

- a name;
- at least one step;
- unique step names of letters, digits, `-`, `_` and `.`;
- **After** naming earlier steps only (so steps are listed in the order they run);
- retries from 0 to 100, as a whole number;
- a command for a command step and a prompt for a prompt step.

The foot names the problem and the form keeps what you typed.

A save is a Guarded write on the version you started from. If the workflow changed meanwhile, nothing is written: the card says "Changed since you opened it. Nothing was written; your Draft is kept." and shows your workflow (**Yours**) beside the workflow as it stands now (**Current**), both as JSON. **Keep mine** (`o`) saves yours over it and **Discard mine** (`d`) drops your changes. When the workflow was archived meanwhile, only Discard mine is offered, since a save would bring it back.

## Archive and restore

`x` archives the selected workflow and `r` restores an archived one, both guarded the same way. kata refuses to archive a workflow a live Cron job uses, and the Pane shows its message: "Cannot archive "W": workflow W is used by live job J; update or delete that job first."

To run a workflow on a schedule, make a job whose kind is Workflow on the [Jobs Page](jobs.md). How [the worker](worker-reference.md#workflow-runs) runs the steps is in the worker reference.
