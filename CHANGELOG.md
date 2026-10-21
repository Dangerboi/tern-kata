# Changelog

All notable changes to tern-kata are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [semantic versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `b` in a Project Collection switches the list to a Board: a column for No lane (open Issues with none of the Lane labels, even one labelled only `bug`), one per Lane (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, each with its card count) and Done (Issues closed in the last 14 days, at most 20, newest closed first; the header reads "Done 20 of 192" when more qualify). Each Lane header is a badge in its own theme tone (No lane muted, `needs-triage` warning, `needs-info` error, `ready-for-agent` info, `ready-for-human` user, Done success; custom Lanes take the other tones in turn). A card shows its short id as a badge in its Lane's tone, then its priority as a badge (`P0` error, `P1` warning, `P2` info, other priorities muted; none without a priority, kept on Done cards), its title over at most two lines (muted on a Done card; a recurring Issue's starts with "↻ ", as in the list) and its labels that aren't Lane labels as info badges; an Issue with several Lane labels shows once, in the first Lane, with "also: …", and a Done card says which Lane `r` reopens it into. The selected card shows its full title, its owner as a user badge and every label. `↑`/`↓`/`j`/`k` move within a Lane, `←`/`→` across Lanes (skipping empty ones), a click selects a card, and the Issue keys, `x`, `r`, the filters and the live refresh work as in the list. `b` again returns to the list with the same Issue selected, and `b` outside a Project Collection says Board mode needs one. The Lanes share the Pane's width, from 140 columns up, with card text wrapping to fit, and each Lane's cards scroll inside a list that fills the Lane down to the footer, sized from the Pane's rows, so the top bar stays in view. The footer lists `+`, `-`, `a` and `A` for the selected card on the Board and in the Lane view. An open card with a deadline or scheduled date shows one Date badge after its priority badge, for the most pressing date ("overdue 2d" error, "due today" warning, "scheduled 3d ago" warning, "scheduled today" accent, "due tomorrow" warning, "due 10-24" and "scheduled 10-28" muted; another year reads YYYY-MM-DD), and the selected card shows a Dates line ("scheduled 2026-10-28 · due 2026-11-02"); both read dates as the Today, Upcoming and Deadlines Collections do and move on at local midnight.
- `>` and `<` on the Board move the selected card one Lane right or left among No lane and the Lanes. The target label is added first, then the other Lane labels are removed, so a failure leaves the Issue in two Lanes, never none; an Issue with two Lane labels ends with one. The card shows in its new Lane at once, a failure leaves a Notice with the daemon's reason, and a Move is refused while the daemon is disconnected.
- An open Board card that an open Issue blocks shows a "blocked by 8gm4" line in the warning tone, on every card whether selected or not. It lists every open blocker (qualified `project#id` outside the Project, cut to three and "+N"), counts blockers the Board doesn't draw, and never shows on Done cards or once the blockers are closed.
- Selecting a Board card highlights the cards related to it: each gets a badge beside its short id (`parent`, `child`, `blocks this`, `blocked by this`, `related`, as seen from the selected card) and a ring in the Relationship's tone (accent, info, error, warning, neutral). Several Relationships show every tag, and the ring follows blocks this, blocked by this, parent, child, related. The selected card keeps its ring, Done cards stay muted, and the Lane view shows no highlight.
- The selected Board card shows a Relationships line: its parent, its child count ("6 children, 2 open"), the open Issues it blocks, its open blockers and its related Issues, with empty parts left out and ids qualified outside the Project and cut to three and "+N". Each id is clickable and selects that Issue's card, revealing it in its Lane; an Issue the Board doesn't draw leaves the Notice "<id> isn't on the Board.". `^` on the Board or in the Lane view selects the parent's card the same way, and leaves "<id> has no parent." without one. The help sheet's Board group lists `^`.
- Enter or a double-click on a Board card opens its Lane view: that Lane's Issues as a full-width list under a Lane header, with the Issue detail beside it at 140 columns and wider. It has the list's keys; `←`/`→` switch Lanes, `<` and `>` Move the card and select the next card in the Lane, and Esc returns to the Board with the card kept.
- A Pane under 140 columns opens Board mode in the Lane view, on the selected Issue's Lane, so `B` and the Move keys work there too.
- `B` on the Board or in the Lane view opens the Lanes card: a wide card with a Lane list section (the Project's Lane list as comma-separated names, its help under it) and a Done section ("Days back" and "Most cards", with the defaults 14 and 20 as placeholders; `tab` moves between fields; blank restores the default 14 and 20, anything but a positive whole number is refused naming the field). `enter` saves it to the Lane list file and the Board's columns change; `esc` cancels. Names are tidied to labels (`In review` reads `in-review`), and an empty name, a name over 64 characters or a duplicate is refused with a message naming the entry. An empty field restores the default list. A muted "Not Lanes yet" line under the Lane list names the labels on loaded Issues that aren't Lanes. `B` waits, with a Notice, until the saved list has loaded. Saving never changes a label in kata.
- The Board's Lanes come from the Project's table in `lanes.toml` under tern-kata's folder of the user config directory, with optional `done_days` and `done_cards` beside the labels, read each time the Board turns on or the Project changes; a missing file or Project gives the default Lanes, and an unreadable one gives the default plus a Notice and is never overwritten.
- `d` on the selected Issue (list, Board, Lane view, Issue detail) opens the Dates card: Scheduled and Deadline fields, prefilled, taking `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd` and `+Nw` against the Pane calendar's date with a muted weekday preview; blank clears and anything else is refused naming the field. Saving writes the changed dates only to the Issue's `scheduled_on` and `deadline_on` metadata, each guarded on the value the card opened with, so a date changed elsewhere is reported and not overwritten; a date kata holds with a time is left alone unless edited.
- The Create card (`n`, `N`) has Scheduled and Deadline fields after Priority, taking the same `YYYY-MM-DD`, `today`, `tomorrow`, `+Nd` and `+Nw` input with a muted weekday preview as the Dates card; an invalid date is refused naming the field and nothing is created. The dates are sent in the one create request as the Issue's `scheduled_on` and `deadline_on` metadata, and none when the fields are blank.
- Every single-line text field has a movable caret: Create, Close, Recurrence, New Project, the single-field forms, the Lanes card, the Dates card, the Filter form, the Menu filter and Search. `←`/`→` move it a character, `Home`/`End` or `ctrl+a`/`ctrl+e` to the ends; typed and pasted text goes in at the caret, Backspace deletes before it, Delete after it, `ctrl+u` clears the field. The Menu keeps `Home`/`End` for its entries (`ctrl+a`/`ctrl+e` move the caret), and the Filter and Recurrence cards keep `←`/`→` for their choice fields.

## [0.3.0] - 2026-10-20

### Added

- `tern-kata attach <ref> <file>...` and `tern-kata detach <ref> <name>` attach files to an Issue and remove them again from a shell or an agent, without Tern. `attach` prints each stored absolute path; both need the directory to be bound to the Issue's Project and exit 1 on any failure.
- An Attachment is a copy of the file in `.kata-attachments` at the root of the Project's primary checkout, which every linked worktree shares and which stays out of git; an empty `.gitignore` there is rewritten with `*` on the next attach. An `attachment.<name>` meta key on the Issue holds the copy's host and path. Detach clears the key, including one with an unreadable value, and deletes the copy only when it is on this machine and in that Issue's folder, saying what became of the file. A detach clears the key only while it still holds what was read: a pointer rewritten meanwhile survives with its file, and the refusal says the Attachment changed. When the key is cleared but the copy can't be deleted, `U` closes the form and `tern-kata detach` prints "Removed <name>; could not delete <path>: <reason>" on stdout, then exits 1 with the error on stderr.
- The Issue detail has an Attachments tab after Links. It lists the Issue's Attachments by name, each on its own row whatever the name; a file held on this machine is a link to the file (one on a Windows network share links as `file://server/share/...`), one held elsewhere reads "<name> on <host>", and an Issue with none says "No attachments, u to attach".
- `u` on the selected Issue, in the list or the Issue detail, asks for a file's path and attaches it. `U` lists the Issue's Attachments by name and removes the one chosen, and says so instead of opening a picker when there are none. Both keys explain why when the directory is not bound to the Issue's Project. Attaching is refused when this machine's host name can't be read, and a pointer with an empty host is never treated as on this machine. The footer and help sheet list them beside the label keys. A successful `u` shows "Attached <stored name> to <short id>", so a file stored as `shot-2.png` is named.
- Two agent skills ship with tern-kata: `kata` for working with the kata CLI and `tern-kata` for reading and attaching Attachments. `npx skills add Dangerboi/tern-kata -g` installs them (`bunx` works too), and the install scripts offer to run it when they run in a terminal, with `bunx` when Node.js is missing.

## [0.2.0] - 2026-10-19

### Added

- Install scripts (`install.sh`, `install.ps1`) download the latest release, verify its checksum and install the window plugin when `tern` is on `PATH`. The **Install tern-kata** palette command runs them in a new Pane.
- `tern-kata --help` lists the flags.
- `R` on a comment in the Comments tab opens the comment Draft as a reply. `ctrl+r` cycles the kind (reply, confirm, refute, supersede), and a confirm or refute under 40 characters is refused before it is sent. A duplicate reply shows kata's message and offers to send anyway.
- Replies in the Comments tab show their kind, whose comment they answer and, for a comment with replies, how many. `enter` on a reply jumps to the answered comment, even in another Issue.
- `O` on the selected Issue opens the Recurrence card. It makes an Issue recur with a form for frequency, interval, weekdays, monthly and yearly day, Start date, end (a date or a number of occurrences) and time zone, and it changes or stops the recurrence of an Issue that already recurs.
- The Recurrence and Filter cards respond to clicks on their choices, and the Recurrence card's Every field names its unit.
- The Issue detail reads a recurrence in words with the next date ("Weekly on Mon, Thu · next 2026-10-26"), or "stopped", and recurring Issues carry a marker in the list.
- An owner with a timed claim reads "owner until 14:30" wherever an owner shows (list, children table, Issue detail).
- `I` opens the Inbox. `ctrl+n` and `ctrl+p` move a row, and `ctrl+v` and `alt+v` page.
- The children table has an owner column, a header row and alternating shading.

### Changed

- tern-kata requires the kata v0.19.0 daemon. Against an older daemon the Pane refuses to connect and says to upgrade.
- Saving a Draft and posting a comment or reply conflict only when the Issue's text changed. An agent claiming the Issue, a claim expiring or an owner change no longer causes a conflict. Closing succeeds while the Issue is open, and if someone closed it first the Close card says so.
- The Pane finds the daemon the way the kata CLI does, so it works without `kata` on `PATH`. Connecting gives up after 10 seconds instead of hanging.
- A workspace bound to a Project the daemon does not know says to run `kata init`, for local and remote daemons alike. Only an unbound workspace opens the Inbox.
- The Logbook, Today, Overdue, Upcoming and Deadlines groups, detail times and the Recurrence card's Start date use your local time zone.
- Child counts arrive with the Collection, so a live refresh downloads nothing extra and a failed count no longer discards a Collection that loaded.
- The footer shows each place's hints in kata tui's order and words and wraps them into a grid instead of hiding the ones that do not fit. The Issue detail footer adds label, owner, parent and priority hints.
- Going back after a jump returns to the tab and row you left.
- Compact activity lists the oldest entry first.
- A kata link opened in Tern reuses a kata Pane only in the current tab.
- Every card button names its keys, for example "Keep editing (n / esc)", including the Filter card's Apply and Cancel.
- Comments, replies, new Issues and closes can be sent again safely after a lost answer, without duplicating. A Recurrence change that keeps losing a race to another edit says so in plain words.

### Fixed

- Today and the other date groups move to the new day at local midnight, also when the machine slept through it.
- Text written in `$EDITOR` is kept in a file, and its path is printed, when Tern cannot be reached after the editor closes.
- `--as` and `--issue` refuse a value that is another flag, so `--as --issue x` no longer starts tern-kata as the actor `--issue`.
- `install.ps1` parses, keeps `%VAR%` entries in `PATH` and says when it changes `PATH`, so programs started from Explorer, such as Tern, see it.
- **Open kata**, `F9` and kata links open tern-kata in a new tab, in the focused Pane's directory, when Tern can't split the focused Pane, instead of doing nothing.

[Unreleased]: https://github.com/Dangerboi/tern-kata/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Dangerboi/tern-kata/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Dangerboi/tern-kata/compare/v0.1.1...v0.2.0
