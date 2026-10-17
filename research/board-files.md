# Which files does Tern's board find, and can the window plugin open one on the session host?

Tern 0.9.2 (23735aa). Probed in an isolated headless `tern serve` (own TERN_CONFIG_DIR, TERN_DAEMON_SOCKET, STENCIL_LOG_DIR, mode-700 temp dir), driven with `tern ctl --control ... carly lua`.

## Answer

1. **`cx.board:boards()` discovers by content, not by name.** For every pane in the window (all tabs), it looks in the pane's cwd and in that pane's checkout (git) root, **top level only**, at `*.md` files that look like a board. Hidden names (`.TODO.md`) count; any name counts (`README.md`, `plan.md`, `board.md`, `KANBAN.md`, `Kanban.md`, `kanban.md`, `tasks.md`, ...). Not found: subdirectories of the root (`sub/TODO.md`, `docs/TODO.md`, `.kata/KANBAN.md`), other extensions (`.txt`, `.markdown`), and files with no list items. A subdirectory file is found only when a pane's cwd is that directory. Boards already open as panes are always listed (any path/extension, even `notes.txt`) with `open=true` and `pane`.
2. **"Looks like a board"** (observed): a `.md` with at least one checkbox item (`- [ ] a`, with or without a heading) or a heading followed by a plain bullet. Not listed: empty file, prose only, heading only, heading + prose, plain bullet with no heading.
3. **`cx.board:open(path)` opens any path, any name, any depth**: `sub/TODO.md`, `prose.md`, `notes.txt`, a directory path (no raise) and a nonexistent `nonexistent.md` all return a pane. Non-board content gives a board with 0 lanes; a nonexistent file gives `exists:false` with 3 default lanes (as documented for `board new`). Opening a path that is already open reveals the existing pane (same pane id returned). Relative paths resolve against the focused pane's cwd (focus in `work/sub/deep`: `h-pip.md` became `work/sub/deep/h-pip.md`). The ctl `board open <path>` opens beside and, unlike the Lua call, opened a second pane for an already-open file.
4. **`how`** accepts `default | beside | below | tab | pip | preview`. Observed: `default`/`beside`/`below` add a pane to the active tab; `tab` makes a new tab with the board alone; `pip` creates a floating picture (`pips[]` entry, mode `focus`); `preview` made a new tab and the pip showed mode `glance` (placement semantics not documented; seen only). `replace` and `split` (valid for `cx:open`/`route.open`) and unknown values **raise** in `cx.board:open`.
5. **Remote session host: unknown, but docs point to "no".** Docs and types say the board API takes "a local Markdown path" / "local pane cwds" (`tern.d.luau:839`, `:859-861`; Board cx section of reference/api). The window half runs `tern.fs`/board work "on the machine showing the window" (guides/io.md, "Which machine, which thread"), and the docs repeat that for remote panes the window's disk answers for the wrong machine (guides/commands and guides/routing: "available reads the disk of the machine the window runs on"). Nothing says `cx.board:open` forwards an absolute path to a pane's host (contrast `route.open`, whose `{path=...}` "opens in a file block on the pane's host"). The likely behavior with a Windows window is that the path is read as a path on the Windows machine. **Not verifiable headlessly**; the user's manual host boards go through the built-in UI, not `cx.board`.

   Safe options for tern-kata if this proves true: have the host half (runs on the session host, where the Board file lives) own file writes and surface it with `route.open`/`cx:open(path)` (host-aware file block), or test once in the real Windows window with `cx.board:open("/abs/path/on/ubuntu/TODO.md")` and check `exists` via `tern ctl state | jq .board_blocks`.

## Evidence

Docs: https://docs.stencil.so/tern/scripts/board.md (`board open <path>` "opens a task file beside the focused pane"; state `exists`, `phase`), reference/api (Board cx: "Board arguments accept a local path or board pane id; relative paths use the focused local pane's cwd. how also accepts default, below, tab, pip, and preview."; `boards()`: "task/Kanban Markdown files in local pane cwds and checkout roots"), https://docs.stencil.so/tern/guides/io.md.

`tern.d.luau` regenerated with `tern plugin types <dir>` (identical to the repo copy):

```
function open(self, path: string, opts: {how: "default"|"beside"|"below"|"tab"|"pip"|"preview"}?): number
function boards(self): Awaitable<{{path: string, pane: number?, open: boolean}}>
```

Probe setup (work dir is a git repo with files described below; `L` = `tern ctl --control $D/control.sock carly lua '<code>'`, then `tern ctl ... state | jq .carly.last_tool.text`; note the ctl line parser eats double quotes, so use `[[...]]` strings):

- `return tern.json.encode(await(cx.board:boards()))` with pane cwd = `work/`. Files present: TODO.md KANBAN.md Kanban.md kanban.md tasks.md board.md TASKS.md BOARD.md todo.md Todo.md plan.md README.md backlog.md roadmap.md .TODO.md (all with `## Todo` + `- [ ] a`), notes.txt, TODO.txt, TODO.markdown, prose.md, `sub/TODO.md`, `sub/deep/KANBAN.md`, `docs/TODO.md`, `.kata/KANBAN.md`. Listed: every top-level `.md` above incl. `.TODO.md`; not listed: `.txt`, `.markdown`, prose files, all subdirectory files.
- Content variants (top-level): listed `c-checks-only.md` (`- [ ] a`), `c-h1-checks.md` (`# Title` + checkbox), `c-head-bullets.md` (`## Todo` + plain bullet), `c-nocard-blank.md` (`## Todo` + checkbox, no blank line); not listed `c-heads-only.md`, `c-head-prose.md`, `c-bullets.md`, `c-empty.md`, `prose.md`.
- New tab with cwd `work/sub/deep` (`cx.layout:new_tab({cwd = [[...]]})`): list gained `work/sub/deep/KANBAN.md` and kept `work/*.md` (git root), but not `work/sub/TODO.md`. Another tab with a non-git cwd `plain/` added `plain/stuff.md`. A repo with a TODO.md that no pane is in (`work2/`) was never listed. So: union over all panes of cwd top level + git root top level.
- `cx.board:open([[<abs>]])` for `sub/TODO.md`, `prose.md`, `notes.txt`, `nonexistent.md` returned panes 8, 10, 12, 14; `ctl state | jq .board_blocks` showed `phase:"ready"`, `lanes` 1/0/0/3, `exists` true/true/true/**false**. After that `boards()` listed all four with `open=true`, pane ids.
- `how` loop, one board per value: tabs after default `[work:1, deep:1, h-default.md:6]`, beside `...:7`, below `...:8`, tab added tab `h-tab.md:1`, pip added `pips[0]{owner:22, mode:"focus"}`, preview added tab `h-preview.md:2` and the pip became `mode:"glance"`. Re-opening beside (18) and pip (24) returned the same pane ids. `how=replace` and `how=bogus`: `pcall` false.
- ctl `board open <abs>/sub/deep/KANBAN.md` after the Lua open of the same path: `board_blocks` listed the path twice.
- Headless host list: one entry `Ubuntu-Coder` (state Connected, address ""), i.e. only the local daemon; no remote host to probe.

## Unverified

- Behavior of `cx.board:open(<absolute posix path>)` from a Windows window while the focused pane is on a remote host (not probe-able headlessly).
- Exact `preview` placement semantics; whether a directory path opens a usable board.
- The exact content rule beyond the cases in the table above.
