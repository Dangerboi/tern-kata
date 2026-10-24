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

## Missing keys

tern-kata lacks these kata tui keys:

| Key | kata tui action |
|---|---|
| `u` | undo the last change |
| `t` | timed assignment |
| `[` and `]` | previous and next comment link |
