# Glossary

**Pane**: The Tern area the program draws into. _Avoid_: block (in Tern a block is a pane type; in kata `blocks` is a link).

**Issue**: One unit of tracked work in kata, as kata defines it.

**Project**: kata's namespace for issues, links, labels and events.

**Collection**: A named set of issues the user browses: Inbox, Today, Upcoming, Deadlines, All Open, Logbook, or one project.

**Menu**: The popup inside the Pane that switches the current Collection: a type-to-filter list with the current Collection marked and the bound Project pinned first. A directory bound to a Project has it instead of the Menu sidebar; unbound directories keep the sidebar. _Avoid_: palette (Tern's own command palette).

**Blocking link**: kata's `blocks` / `blocked_by` relationship between two issues. _Avoid_: block, blocker.

**Draft**: An unsaved edit to an issue's text or fields, kept until the user saves or discards it, even if the issue changes underneath.
