# Changelog

All notable changes to tern-kata are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [semantic versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `tern-kata attach <ref> <file>...` and `tern-kata detach <ref> <name>` attach files to an Issue from a shell or an agent, without Tern. Each file is copied into the Project's Attachment store (`.kata-attachments` in the primary checkout, kept out of git) and an `attachment.<name>` meta key holding its host and path points to it.
- The Issue detail has an Attachments tab after Links. It lists the Issue's `attachment.*` meta keys by name; a file held on this machine is a link to the file, one held elsewhere reads "<name> on <host>", and an Issue with none says "No attachments, u to attach".
- `u` on the selected Issue, in the list or the Issue detail, asks for a file's path and attaches it. `U` lists the Issue's Attachments by name and removes the one chosen, deleting its copy when it is on this machine, and says what happened to a file that was left or already gone. Both keys explain why when the directory is not bound to the Issue's Project, and `U` on an Issue with none says so. The footer and help sheet list them beside the label keys.

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

[Unreleased]: https://github.com/Dangerboi/tern-kata/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Dangerboi/tern-kata/compare/v0.1.1...v0.2.0
