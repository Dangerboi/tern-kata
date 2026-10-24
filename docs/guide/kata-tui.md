# Compared with `kata tui`

![kata tui showing the same Project and Issue](../images/kata-tui.png)

tern-kata takes kata tui's movement keys:

- `j`/`k` and the arrows;
- `ctrl+n`/`ctrl+p` to move a row;
- `pgup`/`pgdn` and `alt+v`/`ctrl+v` to page;
- `g`/`G`;
- `I` to open the Inbox (from the list, the Issue detail, the Jobs, Runs and Workflows Pages and the Menu sidebar; tern-kata opens it, where kata tui toggles it).

`?` lists every key. The Emacs keys and `I` are plain text in search and in Popup cards.

## Activity tabs

The Issue detail's Activity tabs are Comments, Events, Links and Attachments, cycled with `tab` and `shift+tab`. Attachments lists the Issue's `tern-kata.attachment.*` meta keys by name: a file held on this machine is a link to it, and one held on another machine reads `<name> on <host>` (see [Attachments](attachments.md)).

`R` on a comment in the Issue detail's Comments tab replies to it; `ctrl+r` in the card cycles reply, confirm, refute and supersede.

## Links

In the Issue detail, `b` adds a Blocking link and `l` a related link to the Issue you name: a short id, `#id` or `project#id`. The field is kata tui's add-link request. kata records a Blocking link from the Issue you add it on, so the field reads "This Issue blocks (Issue id)"; kata tui words the same key "add blocker". When kata refuses the link (an Issue to itself, an archived Project, a Project outside your scope) its reason shows under the field. On the Board and in the Lane view `b` still toggles the Board.

## Timed assignment

`t` in the Issue detail gives the Issue to you for a set time. The field reads "Assignment duration for #id (1m..24h, e.g. 1h)" and takes a Go duration of whole seconds from 1m to 24h (`90m`, `1h`, `24h`); anything else is refused with kata tui's wording, "duration must be a whole number of seconds from 1m through 24h". kata records the claim for your actor, and the Issue detail shows the owner and "until" the claim ends.

## Undo

`u` in the list, on the Board, in the Lane view and in the Issue detail takes back the newest change this Pane made, as in kata tui. The Pane remembers the last 20 completed changes, across Issues, and undoes the newest first. It undoes a close (reopens), a reopen (closes as done, opening the Close card first when kata requires evidence), an owner or priority change, a label added or removed, a body edit, a link added (the link is removed, found by type and its two ends), a Lane Move (its label changes reversed) and a Dates card change (both dates put back, each guarded on the value the change wrote).

Before it writes, `u` checks, with kata tui's words, that it is still the same daemon and account ("daemon instance changed", "daemon principal changed"), the same Issue ("issue identity changed") and that nobody changed the Issue since: the revision the change produced for owner, priority, label, body and link ("issue owner changed since the action" and the like), the dates the change wrote for a Dates card change ("issue dates changed since the action"). A refusal drops that change and says so; the one after it can still be undone. An Issue already as the undo would leave it reads "already restored". A second `u` while one is out says "undo already in progress".

Every other change to an Issue clears the history, and the next `u` says "nothing to undo: <reason>": creating an Issue, a comment or Reply, a timed assignment, a Recurrence change or stop, attaching or detaching a file, and every Cron job or Workflow change. A change whose outcome is unknown (a timeout) clears it too. `u` never undoes an older change across one of these.

## Missing keys

tern-kata lacks these kata tui keys:

| Key | kata tui action |
|---|---|
| `[` and `]` | previous and next comment link |
