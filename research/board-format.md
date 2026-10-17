# What Tern's board block writes for card and Lane extras

Probed against `tern 0.9.2 (23735aa)`, isolated headless `tern serve`
(own `TERN_CONFIG_DIR`, `TERN_DAEMON_SOCKET`, `STENCIL_LOG_DIR`, control socket),
driven with `tern ctl --control … --file` using the `board` commands from
https://docs.stencil.so/tern/scripts/board.md. `board fixture` is unusable
(`set STENCIL_FIXTURE_ROOT …`), so every probe used `board open <own path>` and
`board write`.

## Answer

The board is a **line-preserving editor**, not a re-serializer. It parses the
file into lanes and cards, keeps every raw line, and on save only touches the
lines it was asked to change. So Sync can keep an id and anything else it owns
in the card line or the card body and it survives.

- **Lane** = a line starting `## ` (exactly two `#` and a space). Title = the rest, trimmed,
  trailing `#`s dropped (`## Todo ##` → `Todo`), but the heading line is saved
  verbatim unless the lane is renamed (then `## <new>`). `#`, `###`, `##x`, `#x` are
  not Lane headings: they are plain text.
- **Card** = a column-0 list item (`- `, `* `, `+ `, `1. `). Checkbox `[ ]` / `[x]` / `[X]`
  directly after the marker is the done state; no box = open card. Everything after
  the checkbox is the title, **verbatim** (trailing spaces, tabs, `(abcd)`,
  `<!-- id:abcd -->`, tags, due tokens all kept).
- **Card body** = every following line (blank lines, indented lines, unindented text,
  HTML comments, `---`, `###` headings, `-[ ]` without space, indented bullets) up to
  the next column-0 list item or `## ` heading. A card's body moves with it on a drag
  and survives title edits.
- **Notes / subtasks**: details panel shows the body dedented by 2 spaces, saves it
  indented by 2 spaces; subtasks are the body lines `[-*+] [ ]` / `[x]` / `[X]`
  at any indent (card chip `1/2`). Foreign body lines are kept.
- **Done lane** = the Lane whose preamble contains the line `**Complete**`
  (surrounding whitespace OK, case-sensitive). With no marker anywhere, the last
  Lane is the done lane. Done is a *Lane* property; a card carries only its checkbox.
- **Folded lanes**: UI state only, **never written** to the file.
- **Tags** (`#word`) and **due dates** (`due:YYYY-MM-DD`, `📅 YYYY-MM-DD`) are not
  separate syntax: they are tokens inside the title, rendered as chips, stored raw.
- Save: atomic replace (new inode), preserves CRLF, BOM and a missing final newline.
  Cards write within ~20-70 ms of the action, notes ~0.62 s after the last keystroke.
  Nothing is written on open, on `board write` reload, on fold or on selection.

## Evidence

Helper: python wrapper around `tern ctl --file /dev/stdin` plus a 2-5 ms poller of the
file bytes. All outputs below are exact bytes (`repr`) unless noted.

### Fresh board, first card
`board new` + `board key new_below`, `board type "Buy milk"`, `board commit`:

```
## Todo\n\n- [ ] Buy milk\n\n## Doing\n\n## Done\n
```
Default lanes Todo/Doing/Done, `Done` has **no** marker (implicit last lane;
state `done_lane: 2`, `lanes[2].complete: false`). New file on an empty path
(`board open` of a nonexistent file) writes the same three lanes. An empty existing
file plus `new_below` writes `## N\n` (a lane named after the typed text). Cards
before any `##` heading form an untitled lane with no heading line
(`- [ ] loose A\n` + new card → `- [ ] loose A\n- [ ] N\n`).

### Done lane toggle (`board key done_lane`)
- On Doing (lane 1) from the default file: `## Doing\n\n**Complete**\n\n## Done\n`;
  Done loses its done status (`done_lane: 1`).
- On the implicit last lane: `## Done\n\n**Complete**\n`.
- Toggling off leaves the extra blank behind: `## Doing\n\n\n## Done\n`; last lane
  `## Done\n\n`. Marker is only honoured in the Lane preamble (before its first card);
  `**Complete**` after a card, `**complete**`, `**Complete** yes`, `*Complete*`,
  `<!-- complete -->` are not markers (they become card body / preamble text).
  Two marked Lanes: first one wins (`done_lane: 0`). `**Complete**` directly under the
  heading without blank lines works.
- Marker in Todo: `done_lane: 0`.

### Done cards
- `[x]` card written in a non-done Lane is kept verbatim on save and is `done: true`
  in state. `board key toggle` on it flips it **in place** to `[ ]`.
- `[ ]` card in the done Lane is kept as-is (not normalised).
- Toggle on an open card moves it to the **top of the done Lane** as `[x]`; toggle on
  a card in the done Lane moves it to the **top of the first Lane**, `[ ]`.
- Drag into the done Lane sets `[x]` (keeps `[X]`, adds `[x]` after `* `/`+ `/`1. `
  markers: `* [x] star`, `1. [x] num`, `- plain` → `- [x] plain`; `- [?] odd` → `- [x] odd`).
  Drag out sets `[ ]` (`- [X]`→`- [ ]`, `* [x]`→`* [ ]`, `- [?]` stays `[?]`, no-box
  stays no-box). Drag between two non-done Lanes leaves `[x]` as is
  (`## Doing\n\n- [x] DoneTodo\n- [ ] D1`).

### Tags and due dates (DOM chips: `tern ctl tree '.bd-tag'`, `'.bd-due'`)
Stored raw in the title; the state card `text` is the full raw title.
- Tag: whitespace-led `#` + letter-led word of `[A-Za-z0-9_/-…]`: `#a`, `#a/b`, `#a_b`,
  `#A` (case kept), `#é`, `#a-`, trailing `#a,` / `#a.` (punctuation left as text)
  match. `x#y`, `#1`, `##dbl` do not.
- Due: whitespace-led token `due:YYYY-MM-DD` (case-insensitive `DUE:`), or `📅 YYYY-MM-DD`
  / `📅YYYY-MM-DD`. Not matched: `due: 2026-…` (space), `due:10-21`, `due:today`,
  `due:2026-1-5`, `@2026-10-21`, `@due(…)`, `[due:…]`, `(due:…)`, `due 2026-…`,
  `due:2026-02-30` (invalid date), `due:2026/10/28`. `due:2026-10-26T10:00` matches the
  date part, `T10:00` stays text. Two due tokens: both stripped, first shown.
- Editing a title via `board key edit` + `board type` replaces the whole title, so a
  caller that edits must re-type the tags/due itself.

### Notes and subtasks (details panel, typed with `click textarea.bd-sh-notes`, `type`)
Typing `first note line`, Enter, `- [ ] sub one`, Enter, `- [x] sub two`:
```
- [ ] Alpha\n  first note line\n  - [ ] sub one\n  - [x] sub two\n- [ ] Beta\n
```
Round-trip of a hand-written body (`<!-- kata:abcd -->`, blank inside, 4-space deeper
line, `- plain sub bullet`, `* [x] star sub`): textarea shows it dedented 2; appending
`ZZ` rewrote only the last line, everything else byte-identical (blank line stays
empty, deeper indent kept). Subtask chip counts `[-*+] [ ]` / `[x]` / `[X]` lines, also
tab-indented, 4-space-indented and `1. [ ]`; `-[ ]` and plain bullets are not counted
(`[?]` handling not isolated).

### Title ending `(abcd)`
`- [ ] Alpha (abcd)` is kept byte-exact through drag, toggle, move, details edit and a
title edit (`board type "Alpha renamed (abcd)"` → `- [ ] Alpha renamed (abcd)\n  note A…`,
notes and marker kept, `* ` marker kept). An `<!-- id:abcd -->` inside the title also
survives. A title typed as `[x] sneaky` is written `- [ ] [x] sneaky`, `## hash` as
`- [ ] ## hash` (no escaping).

### Text between headings / foreign lines
Seed (hand-written through the file, `board open`; `board write` for reload checks):
```
<!-- top comment -->\n# Title heading\n\nIntro paragraph text.\n\n## Todo\n\nText between heading and cards.\n\n- [ ] Alpha (abcd)\n  note line\n  - [ ] sub\n  indented foreign\n- [ ] Beta\n\nsome stray text mid-lane\n- not a task bullet\n* star bullet\n1. numbered\n- [ ] Gamma\n\n<!-- html comment -->\n\n---\n\n## Doing\n\n### Subheading h3\n\n- [ ] D\n\n## Done\n\n**Complete**\n\n- [x] F\n\nTrailing paragraph\n
```
After `board drag 1 0 "Beta"` the file differs from the seed **only** by:
`- [ ] Beta\n\nsome stray text mid-lane` (Beta plus its body) moved from Todo to Doing,
inserted after the Lane preamble (`### Subheading h3`) and before `- [ ] D`.
Kept untouched: HTML comments (file top and mid-lane), `# Title heading`, intro
paragraph, text between a heading and its first card (Lane preamble), `###`
subheading, `---`, trailing paragraph, `- not a task bullet` / `* star bullet` /
`1. numbered` (these are **cards** without a checkbox, so they were untouched cards,
not foreign text), the `**Complete**` marker.

Nothing was dropped in any probe. What *moves* with a dragged card is its body, which
includes unindented lines after it (so Sync must not put Lane-level text right after a
card). Further facts:
- `-[ ] nospace`, `  - [ ] indented`, `\t- [ ] tabbed` are body of the previous card, not
  cards. `- [] empty` and `- [ ]notext` are cards titled `[] empty` / `[ ]notext`;
  `- [ ] ` and `- [ ]` are cards with an empty title. `- [?] odd` is an open card titled `odd`.
- Line endings, trailing spaces and tabs inside titles preserved. CRLF files stay CRLF,
  BOM kept, no final newline kept.
- Blank lines: not normalised. A dragged card takes its trailing blank lines with it;
  lane separators are otherwise left (`## Doing\n\n\n## Done` appears after removing a
  card from a spaced layout). Plan for stray blank lines.
- Heading lines are verbatim until renamed (`## Todo ##`, `##   Doing  ` survive a drag).
  `##Done` (no space) is preamble text of the Lane above, not a Lane.
- `new_below` puts the new card after the selected card's whole body; `new_lane` inserts
  `## <title>\n\n` after the current Lane's whole block (preamble, cards, tail text);
  `rename_lane` rewrites only the heading; `delete_lane` deletes heading, preamble,
  cards and trailing text; `move_lane_*` moves whole blocks.
- Duplicate Lane titles are allowed (`Dup`, `Dup`); empty `## ` / `##` Lanes appear
  with an empty title.

### Save timing
Measured with a 2 ms poller around `tern ctl` calls (headless wall clock):

| Action | File changed |
| --- | --- |
| `board commit` (new card) | file changed 7-29 ms *before* the `commit` ctl call returned, i.e. inside the command |
| `board key toggle` | 37-42 ms before the call returned (inside the command) |
| `board drag` | 35 ms before the call returned (inside the command) |
| `board key done_lane` / rename / new_lane / delete / move_lane | first write seen 16-77 ms after the command started |
| notes textarea `type` | 0.62 s after the last `type` returned (0.623, 0.621, 0.624 s), i.e. ~0.6 s debounce |
| `board write` reload | `board write` returns in ~0.1 s; no rewrite afterwards (2 s watched, file stable) |

Writes replace the file (inode changes, no leftover temp file in the dir): a watcher on
the file must handle rename-over, not in-place modify. `fold`, `select`, opening the
board, and external reload write nothing.

### Not verified
- Behaviour when the file changes on disk while the board has an unsaved note edit or
  open inline editor.
- Undo/redo output; 3+ lanes marked complete; extremely large files.
- Real wall-time on the user's live Tern (probes ran headless; timings are headless).
