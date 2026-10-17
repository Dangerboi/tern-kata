# What Tern's board does when its file changes during an unsaved edit

Probed against `tern 0.9.2 (23735aa)`, isolated headless `tern serve` (own
`TERN_CONFIG_DIR`, `TERN_DAEMON_SOCKET`, `STENCIL_LOG_DIR`, control socket in a
mode-700 dir, all `TERN_*` pane/window vars unset), driven with `tern ctl --control … --file`.
The external write is always "write `TODO.md.tmp`, `os.replace` onto `TODO.md`".
A fresh `tern serve` per case: a second `board open` of the same path creates a
second board block and `board_blocks[0]` is then the stale one, which silently
ruined my first attempt.

## Answer

**The board never discards either side's change, as long as Sync touches a
different card.** While an inline edit is open the board *defers* the reload
(its model and the `lanes` in `ctl state` stay stale, `editing` is `card`/`new`);
on commit it merges its edit onto the *current disk content*, not onto its stale
copy. Notes (details panel) do not defer: the board reloads at once, keeps the
textarea, and saves ~0.5-0.65 s after the last keystroke on top of the new file.

| Case | Reload | Edit kept | External change kept | Final file |
|---|---|---|---|---|
| a. title edit open, ext changes *other* card (Beta→Beta EXT, +Gamma) | deferred (state stale, 3×0.7 s) | yes | yes | `Alpha EDITED / Beta EXT / Gamma EXT` |
| a'. ext inserts card *above* edited card (Zed) | deferred | yes (applied to right card) | yes | `Zed / Alpha / Beta EDITED` |
| a''. ext changes only the edited card's *body* (note added) | deferred | yes | yes | `Beta EDITED` + `  note` kept |
| b. notes typed, ext write 0.15-0.35 s later, other card | immediate | yes (saved at 0.5-0.65 s) | yes | `Alpha / my note / Beta EXT / Gamma EXT` |
| b'. same but ext retitles the card whose notes are typed | immediate | yes | yes | `Alpha RENAMED / my note` |
| b''. ext adds note line to same card's body while typing | immediate | yes | **no**: board's textarea overwrites the body (`ext note` gone) | `Alpha / my note` |
| b'''. ext lands between two typing bursts | immediate | yes (`one two`) | yes | as above |
| c. new card typed (`new_below`), ext changes other cards | deferred | yes (appended at end of lane) | yes | `Alpha / Beta EXT / Gamma EXT / NEW card` |
| c'. ext deletes the anchor card | deferred | yes | yes | `Alpha / NEW card` |
| d. ext changes the very card being edited (title) | deferred | yes, but as a **new card** | yes | `Alpha EDITED / Alpha EXTERNAL / Beta` (duplicate) |
| d'. ext deletes the edited card | deferred | re-created | n/a | `Alpha EDITED / Beta` |
| d''. ext swaps order / moves card to Doing | deferred | duplicate (`Alpha EDITED, Beta, Alpha` / Doing keeps `Alpha`) | yes | no loss, but a copy |
| Escape (cancel) after ext write | applied right after cancel | edit dropped | yes | ext content as written |

Final bytes for a (exact): `## Todo\n\n- [ ] Alpha EDITED\n- [ ] Beta EXT\n- [ ] Gamma EXT\n\n## Doing\n\n## Done\n`.
Final bytes for d: `## Todo\n\n- [ ] Alpha EDITED\n- [ ] Alpha EXTERNAL\n- [ ] Beta\n\n## Doing\n\n## Done\n`.
Final bytes for b: `## Todo\n\n- [ ] Alpha\n  my note\n- [ ] Beta EXT\n- [ ] Gamma EXT\n\n## Doing\n\n## Done\n`.

## Recommended Sync write rule

1. **Read-modify-write the whole file, change only the lines Sync owns, atomic
   rename** (tmp + `mv`, same as the board does). Never write from a cached copy.
   Case a/a'/a''/b/b'/c show this loses nothing on either side, even with the
   board mid-edit, so Sync needs no knowledge of `editing`/details (which only the
   window has anyway).
2. **Do not rewrite a card the user may be editing** (Sync-owned changes to the
   card's own title line, or its notes body): d-cases give a duplicate card, b'' loses
   Sync's body line. Sync should keep its edits to lines that the board's UI does
   not edit concurrently, or, if it must change a card, change it via line-level
   edit and accept the duplicate/overwrite risk; it cannot detect it from the file.
3. **Do not re-write on every file event.** After the board's own save Sync sees a
   file change; compare to its last-written/last-seen bytes and skip if there is
   nothing of its own to change (avoids ping-pong). Board saves land ≤70 ms after
   card actions and ~0.6 s after the last note keystroke; a debounce of ~1 s on
   the board-originated event is enough to coalesce.
4. Rename races are lost-update windows: the board and Sync both do
   read→rename. Probe `race`: Sync appending a card every 3 ms during a commit
   lost data in either direction (edit lost in 1/3 run, some Sync cards lost in
   the others, each loop being racy by itself); at 50 ms and 200 ms spacing 3/3 runs
   lost nothing. So Sync must not write in a tight loop; batch into one write per
   cycle.

The rule (whole-file RMW, atomic, own lines only, no tight loop) holds in
probes a, a', a'', b, b', c, c', b''' and race at ≥50 ms. It does **not** protect
from case d (same line) or b'' (same notes body).

## Evidence: harness and commands

`/tmp/m29k_h.py` (not committed): wrappers around
`tern ctl --control $D/control.sock --file s.ctl` (lines below), `tern ctl … state`
(`board_blocks[0]`: `lanes`, `cursor`, `editing` = `card`|`new`|null, `details`,
`can_undo`), and a 50 ms poller of the file bytes. Per case: `tern ctl quit`,
write seed, `tern serve …`, `board open <TODO.md>`.

Seed: `## Todo\n\n- [ ] Alpha\n- [ ] Beta\n\n## Doing\n\n## Done\n`.
EXT: `…- [ ] Alpha\n- [ ] Beta EXT\n- [ ] Gamma EXT\n…`.

```
a:  board select "Alpha" / board key edit / board type "Alpha EDITED"
    (external atomic write of EXT) / run true ×3 with 0.7 s sleeps -> state lanes still Alpha,Beta, editing:card, file = EXT
    board commit -> file = Alpha EDITED, Beta EXT, Gamma EXT
b:  board select "Alpha" / board key details / click textarea.bd-sh-notes / type "my note"
    (ext write immediately, and +0.15 s) -> poll: 0.0 s EXT bytes, 0.5-0.65 s `  my note` inserted under Alpha, state lanes = reloaded
c:  board select "Beta" / board key new_below / board type "NEW card" -> editing:new; ext write; board commit
d:  board select "Alpha" / board key edit / board type "Alpha EDITED"; ext retitles Alpha to "Alpha EXTERNAL"; board commit
    -> `Alpha EDITED / Alpha EXTERNAL / Beta`
```
Escape is `key Escape` (not `board key clear`, which left the editor open).

Notes on limits: a'/a'' were run once each; merge internals were not reverse
engineered (d-cases look like a diff against the pre-edit base re-applied to disk,
inserting at the old position when the anchor line changed).
