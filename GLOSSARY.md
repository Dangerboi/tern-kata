# Glossary

**Pane**: The Tern area the program draws into. _Avoid_: block (in Tern a block is a pane type; in kata `blocks` is a link).

**Issue**: One unit of tracked work in kata, as kata defines it.

**Project**: kata's namespace for issues, links, labels and events.

**Collection**: A named set of issues the user browses: Inbox, Today, Upcoming, Deadlines, All Open, Logbook, or one project.

**Menu**: The popup inside the Pane that switches the current Collection: a type-to-filter list of the fixed Collections, the bound Project pinned first, a **Projects** entry that opens the list of kata Projects, and New Project, with the current Collection marked. It stands in for the Menu sidebar, which only an unbound directory shows, and only while a fixed Collection is current. _Avoid_: palette (Tern's own command palette).

**Blocking link**: kata's `blocks` / `blocked_by` relationship between two issues. _Avoid_: block, blocker.

**Draft**: An unsaved edit to an issue's text or fields, kept until the user saves or discards it, even if the issue changes underneath.

**Popup card**: A card drawn over the columns that takes the keyboard until closed: Create, Close, the Draft editor, the single-field form, New Project, the Menu and the Filter form. When several are open, one order (`TopCard`) decides which takes the keys, is drawn and holds the text caret; the help sheet covers them all. Each card declares which global chords it lets through (only `ctrl+c`, from the Menu, Close and New Project). _Avoid_: layer, modal, overlay (Tern's element name).

**Issue detail**: The column showing one Issue: its properties, Children and Activity tabs, with its own row cursor and jump history.

**Pane driver**: The step that turns one Tern input or daemon result into the next state, the view to draw, the Tern ops to send and the daemon commands to run.

**Daemon gateway**: The module that runs one command against the kata daemon and returns exactly one result message.

**Guarded write**: A write the Daemon gateway applies only if the issue has not changed in a way that matters since the user saw it: saving a Draft of the issue's text, posting a comment, and closing. A Draft or comment conflicts only when the issue's text differs from the text the Draft started against; a revision that moved for another reason (owner, status, claim, expiry) is written against as it now stands. Closing succeeds while the issue is open and reports an issue closed meanwhile without writing.

**Collection snapshot**: What the daemon says the shown Collection holds for the current Collection and filters. One path loads it for the first load, a Collection switch, a filter change, an id search and a live refresh; a result is applied only if it answers the current query.

**Reply**: A comment that answers another comment, with a kind: reply, confirm, refute or supersede.

**Recurrence**: kata's schedule that creates the next Occurrence of an Issue when the current one closes as done. Stopping it creates no further Occurrences and leaves the existing ones as they are.

**Occurrence**: An Issue a Recurrence created, or the Issue that was made the first one.
