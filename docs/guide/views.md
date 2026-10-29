# Views from a shell

A view is a Collection with filters: a fixed Collection (Inbox, Today, Upcoming, Deadlines, All Open, Logbook) or a Project, plus any of the Filter card's fields and a search. Saved views live in `views.toml` in tern-kata's config folder, on this machine only, and the Pane's Menu lists them under **Views**. `tern-kata views` lists, shows, adds and removes them without Tern, with the View card's rules and messages:

```sh
tern-kata views                                    # the fixed Collections, then the saved views
tern-kata views show today                         # a fixed Collection, by key or title
tern-kata views show "Work bugs" --json            # a saved view, ignoring case, as JSON
tern-kata views add "Work bugs" --base project:Work --status open --label bug
tern-kata views add Mine --owner @me --replace     # save over a view of that name
tern-kata views rm "Work bugs"                     # asks nothing
```

## List

Prints the fixed Collections by key, then each saved view with its base (a Collection's title, or `project <name>`) and the filters it sets. A view whose base Project is gone or archived says so. When the daemon cannot be reached the Project note is left out; when `views.toml` cannot be read the fixed Collections still print and the reason follows on stderr (exit 1).

## Show

Reads the snapshot the Pane reads, through the daemon: the view's base, today's date and time zone from this machine, then its filters. `@me` in a view's owner is the `--as` actor (else `$KATA_AUTHOR`, `$USER`, `git config user.name`). It needs no Binding, since views span Projects. Text output groups the Issues as the Pane does ("Overdue", "Today", one heading per Project, one group per day, a flat "Results" for a view with a search), one line per Issue: `<project>#<id>`, status, priority, the dates of a dated Collection, title.

`--json` prints an array with one object per group:

```json
[
  {
    "id": "1-today",
    "title": "Today",
    "issues": [
      {
        "ref": "demo#a1b2", "uid": "01J...", "title": "Fix the build", "status": "open",
        "priority": 1, "owner": "amy", "labels": ["bug"],
        "scheduled_on": "2026-11-02", "deadline_on": null, "closed_on": null
      }
    ]
  }
]
```

Priority, owner and the three dates are `null` when unset; labels is `[]` when there are none; the dates are `YYYY-MM-DD` in this machine's time zone. A name that is neither a Collection nor a saved view exits 2 and lists the names.

## Add and remove

| Flag | Meaning |
|---|---|
| `--base <collection>\|project:<name>` | The base: a Collection key or title, or a Project by name. Default `all-open` |
| `--project <name>` | The Filter card's Project |
| `--status open\|closed\|all` | Status filter |
| `--owner <actor>\|@me` | Owner filter; `@me` is kept as typed and means whoever opens the view |
| `--label <label>` | Label filter |
| `--relationship parent\|child\|blocks\|blocked_by\|related` | Relationship filter |
| `--search <text>` | Search text |
| `--replace` | Save over an existing view of that name |

- Projects are looked up by name (ignoring case) in the daemon and saved by their uid with the name they had.
- A name that is empty, longer than 64 characters, a fixed Collection's key or title, or already taken is refused with the card's message; a taken name is only replaced with `--replace`.
- The write is guarded like the card's: a `views.toml` changed since it was read is refused and nothing is written.
- `rm` refuses a fixed Collection and an unknown name.

Exit status is `recur`'s: 0 on success, 1 on a refusal or a daemon failure (reason on stderr), 2 on a bad command line. `--as` and the refusal of an empty one are those of `attach` (see [Attachments](attachments.md#from-a-shell-or-for-an-agent)).
