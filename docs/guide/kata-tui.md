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

## Credentials

`C` opens kata tui's Credentials view as a Lettered Page, and the Menu lists it. Its wording, fields and 30-second refresh are kata tui's; see [Pages and the Board](pages-and-board.md#lettered-pages).

## Links

In the Issue detail, `b` adds a Blocking link and `l` a related link to the Issue you name: a short id, `#id` or `project#id`. The field is kata tui's add-link request. kata records a Blocking link from the Issue you add it on, so the field reads "This Issue blocks (Issue id)"; kata tui words the same key "add blocker". When kata refuses the link (an Issue to itself, an archived Project, a Project outside your scope) its reason shows under the field. On the Board and in the Lane view `b` still toggles the Board.

## Timed assignment

`t` in the Issue detail gives the Issue to you for a set time. The field reads "Assignment duration for #id (1m..24h, e.g. 1h)" and takes a Go duration of whole seconds from 1m to 24h (`90m`, `1h`, `24h`); anything else is refused with kata tui's wording, "duration must be a whole number of seconds from 1m through 24h". kata records the claim for your actor, and the Issue detail shows the owner and "until" the claim ends.

## Missing keys

tern-kata lacks these kata tui keys:

| Key | kata tui action |
|---|---|
| `u` | undo the last change |
| `[` and `]` | previous and next comment link |
