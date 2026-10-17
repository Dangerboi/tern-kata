# Can the plugin start Sync in the background, open a session-host board, and see it close?

Tern 0.9.2 (23735aa). Probed in an isolated headless `tern serve` (own TERN_CONFIG_DIR, TERN_DAEMON_SOCKET, STENCIL_LOG_DIR, TMPDIR, mode-700 temp dir), driven with `tern ctl --control ... carly lua` and scenario files. A "remote" host was the headless harness's `remote loopback can@devbox` (a real session daemon in the same process, on the same machine and filesystem). Everything that depends on the window and the session host being different machines (Windows window, Ubuntu host) is **unverified** and listed per question.

## Answer

1. **Background start without a visible Pane: yes, with one trap.** `cx.layout:split(<pane>, "right", {command = "setsid -f <cmd> >/dev/null 2>&1 </dev/null"})` runs on the host of `<pane>`, the process survives, and Tern closes the Pane on exit 0. The trap: a plain `setsid nohup <cmd> &` (also `(setsid cmd &)`, `nohup setsid cmd & disown`) is killed when the Pane's shell exits straight away: the child never got to run. `setsid -f` worked 5/5; `... & sleep 0.1` worked 3/3. The survivor is reparented to init (ppid 1486, the user systemd), is its own session leader, and keeps running with the Pane gone. Parking also works (`cx.layout:park(pane)`: process keeps running, Pane leaves the tab) but leaves a card in the session's deck, so it is not invisible. Unverified: the same on the Ubuntu host from a Windows window (the shell Tern uses on that host decides how the command line is parsed).
2. **Opening a host file as a board block from window Lua: partly.**
   - `cx.board:open(path, {how=...})` (any absolute path, any name, `how` = default|beside|below|tab|pip|preview) works, but is documented as "local path"; whether it reads a Windows-window path or the host's cannot be told here (unverified, see manual check).
   - `cx.actions:run("open_board")` runs Tern's Open board… action (the user-visible command; `cx.palette` does not exist, the cx field is `cx.actions`: `list(query)`, `run(id_or_label)`, `describe(id)`, `keys()`). It takes no arguments and cannot be preselected: it opens a picker and returns at once (`"Ran Open board…"`). With the focused pane on the local host it opens a native OS file dialog (`ctl system` shows `title:"Open board"`, directory = pane cwd, filters md/markdown). With the focused pane on the loopback host it opens Tern's own in-app path picker labelled `devbox:/tmp/...` (browses that host; `ctl state` `.picker = {mode:"path", dir:..., selected:...}`), which is the "choice of host" the user sees. Choosing a row opens the board. So the picker always needs a person to pick the file; it is not a way to open `<repo>/KANBAN.md` unattended.
   - `cx:open(path, "beside")` opens a file block (host-aware: `file_blocks[].host` was the loopback host), then `cx.actions:run("open_as_board")` (unavailable until a Markdown file block is focused, `available=true` after) opened a board pane for the same file beside it, no picker. That is the one fully scripted, host-aware route found; its board then reads the path the file block had (unverified for a remote host, the file block stays open as an extra pane).
   - From a **route** (see 3): `tern.route.open` handler calling `cx.board:open(req.path)` works and `req.host` names the host.
3. **Pane to window: yes, two channels, no CLI for boards.**
   - `tern open PATH` run in a host shell (including from a process that outlived its Pane, using the `TERN_PANE_SOCKET` it inherited) is relayed by the daemon to the window and arrives in `tern.route.open` as `{path, host = "devbox", how = "beside", origin = "cli"}`. A window plugin handler that calls `cx.board:open(req.path)` and returns `{handled = true}` opens the board. `tern open` then prints `<path> cannot open in a file block` and exits 1 (documented for `handled`); the board still opens. `--wait` is useless here (no block to wait on).
   - A host plugin half (`host = "host.luau"`, runs in the session daemon on the host; the loopback daemon loaded it) can call `cx:open(path)` on the `EffectCx` of any host hook (probe: a `command_started` hook); it reaches the window as `route.open` with `origin = "link"`, `host = "devbox"`, `how = "default"`. Same route handler opens the board. This needs the plugin's host half installed on the session host.
   - There is no `tern` subcommand that opens a board (`tern help`: `open` = file blocks only; `new`, `split`, `run`, `send`, ... only start or type into terminal blocks). The built-in Open board… picker (palette/action) does **not** pass through `route.open` (nothing logged when picking through it).
4. **Close detection: window yes, host no.**
   - Window: `tern.on("pane_closed", function(ev, cx) ... ev.pane ...)` fires for a board pane closed with `cx.layout:close(p)` and with the control `close` command (the user-equivalent). `cx.board:boards()` then lists it with `open = false` (while open: `open = true`, `pane = <id>`). `pane_created` did **not** fire for board panes opened by `cx.board:open` (only for terminals), so record the pane id from the return value. Event delivery in headless needs frames pumped; real windows draw them.
   - Host: `tern ls --json` / `tern inspect --json` (run in a pane, against `$TERN_PANE_SOCKET`; `env -u TERN_DAEMON_SOCKET` since the probe leaked that variable into the shell) list only the host's terminal blocks: the open board was never in the list. `tern events` (kinds seen in the binary: `title_changed`, `cwd_changed`, `pane_exited`, `pane_closed`, `pane_spawned`, `pane_resized`, `layout_changed`, `client_left`; `--filter` rejects unknown names) printed only terminal `title_changed` lines while a board was opened and closed: no line for a board. Boards are window-side blocks the daemon does not know.
   - Window to host signal: from the `pane_closed` handler, `cx.layout:split(<terminal pane>, "right", {command = "echo closed-<pane> > <file>"})` ran on the host and wrote the file (Pane closed itself). So the window plugin can run a command on the host (for example `tern-kata board stop`) when the board closes. **Guard it**: the split's own Pane closing fires `pane_closed` again, and an unguarded handler looped until stopped (the probe spawned several hundred Panes); only react to ids you recorded.
   - Not testable: the whole window quitting or the connection dropping (neither produced an observable event headless).

## What the user must check in the real Windows window (remote-only)

1. `setsid -f` Pane: from a Windows window, with a Pane focused on the Ubuntu host, run a throwaway command via the plugin or by hand (`setsid -f sleep 600 >/dev/null 2>&1 </dev/null`); confirm the Pane closes and `pgrep sleep` on Ubuntu shows it with ppid 1 or the user systemd.
2. Board path semantics: with the focused Pane on Ubuntu, run **Open board…**; confirm the picker shows host-qualified paths, pick `<repo>/KANBAN.md`, edit a card in the board; check on Ubuntu that the file changed and that no such file or `.md` appeared on Windows. Then, from a throwaway plugin command (`cx.board:open("/home/<user>/<repo>/KANBAN.md")`), check `exists` through the board block (a nonexistent path opens a board with 3 default lanes and `exists = false`).
3. Route: install the window plugin plus (for host-half opens) the host half on Ubuntu; in an Ubuntu Pane run `tern open /home/<user>/<repo>/KANBAN.md`; confirm the route sees `host` = the Ubuntu host's name and the board opens (and that the board is the Ubuntu file, as in 2).
4. Close: close that board with its close button and, separately, quit the Windows window; confirm the plugin's `pane_closed` handler ran (a toast is enough) in the first case, and see what survives in the second (the daemonized Sync keeps running, as in 1).

## Evidence

Setup (all paths under a mode-700 `$D`):

```
. $D/env.sh        # TERN_CONFIG_DIR, TERN_DAEMON_SOCKET, STENCIL_LOG_DIR, TMPDIR=$D/tmp, STENCIL_FIXTURE_ROOT=$D/fx/crates/tern
umask 077          # loopback daemon refuses a socket dir that is not private (mode 775 failed)
tern serve --control $D/control.sock --out $D/shots --dir $D/work
tern ctl --control $D/control.sock remote loopback can@devbox        # {"ok":true}, state hosts.current = devbox
tern ctl --control $D/control.sock plugins fixtures                  # loads $D/fx/crates/plugins/fixtures/probe (window.luau, host.luau)
```

A headless `serve` without `remote loopback` has no daemon-backed panes: `tern whoami` said "not inside a Tern pane" and `tern ls --json` printed `{"sessions":[],"detached":[]}`. After loopback the devbox pane's env had `TERN_PANE=2`, `TERN_PANE_SOCKET=$D/tmp/tern-loopback-<pid>-0/daemon.sock`.

Probe window half (route + events) and host half (the `command_started` hook that calls `cx:open(target)` for lines starting `hostopen `) logged to files; the route handler was:

```lua
tern.route.open(function(req, cx)
    log("route.open " .. tern.json.encode(req))
    if req.path:match("PROBE%.md$") then
        local pane = cx.board:open(req.path, {how = "beside"})
        return {handled = true}
    end
    return nil
end)
```

1. Background start. Pane command strings, each in `cx.layout:split(2, "right", {command = [[...]]})`, loop script writing its pid and a heartbeat file:
   - `setsid nohup $D/loop.sh >/dev/null 2>&1 </dev/null &` x3: `DEAD/NEVER` (pid file never written); `(setsid $D/loop.sh ... &)`, `nohup setsid ... & disown`: same.
   - `setsid -f $D/loop.sh >/dev/null 2>&1 </dev/null` x6: `SURVIVED`, `ps -o pid,ppid,pgid,sid,stat` = `216457 1486 216457 216457 SNs /bin/sh .../loop.sh`; heartbeat kept advancing after the Pane closed (`tabs[0].panes` back to 1).
   - `... & sleep 0.1` and `... & sleep 0.3`, 3 runs each: survived.
   - Visible variant: `split(2,"right",{command=[[$D/loop.sh]]})` then `cx.layout:park(50)` returned `true`; `ctl state` showed the tab with 1 Pane and `park.cards = [{pane:50, label:".../loop.sh", ...}]` (a card in the deck pile); heartbeat kept advancing; `unpark(50)` returned `true` and the tab had 2 Panes again; `cx.layout:close(50)` ended the process.
   - A split on the loopback host's pane runs in that host's daemon: the survivor's env had that host's `TERN_PANE_SOCKET`.
2. Board opens.
   - `cx.actions:list("board")`: `new_board_block` "New board block", `open_board` "Open board…", `open_as_board` "Open as board" (unavailable until a Markdown file block is focused), `cx.actions:list("host")`: `switch_host`, `new_session_on_host`, `add_remote_host`, `disconnect_host`.
   - Local focus: `cx.actions:run("open_board")` -> `ctl system | jq .dialogs` = `[{"title":"Open board","button":"Open","directory":"$D/work","pick":"files","filters":[Markdown (*.md), All files], "kind":"open"}]`; `ctl dialog open $D/work/KANBAN.md` opened a board (`board_blocks[0].path`).
   - Loopback focus: same call -> `ctl system` had no dialog; `ctl state | jq .picker` = `{"mode":"path","dir":"$D/work","rows":3,...}`, tree text `devbox:/tmp/bl.../`, rows `..`, `KANBAN.md`, `PROBE.md`. `type "PROBE"` then `picker enter` opened `board_blocks[].path = $D/work/PROBE.md`; the route handler logged nothing for it.
   - `cx:open([[$D/work/KANBAN.md]], [[beside]])` -> `file_blocks[0] = {path, host:1, kind:"markdown"}`; `cx.actions:run("open_as_board")` -> `Ran Open as board.`; pane list gained `board $D/work/KANBAN.md` beside the `file` pane.
3. Channels. In the devbox pane (zsh, TERN_DAEMON_SOCKET unset for the command):
   - `tern open $D/work/PROBE.md` -> output `$D/work/PROBE.md cannot open in a file block`, `rc=1`; window log: `route.open {"host":"devbox","how":"beside","origin":"cli","path":"$D/work/PROBE.md"}` then `board:open -> 4`; `cx.board:boards()` listed `{"open":true,"pane":4,"path":".../PROBE.md"}`.
   - Typing `hostopen $D/work/PROBE.md`: host log `hostopen ...`; window log `route.open {"host":"devbox","how":"default","origin":"link","path":...}` then `board:open -> 6`.
   - Process outliving its Pane (`setsid -f $D/bgrun.sh`, sleeps 3 s, then `env | grep ^TERN_`; `tern ls --json`; `tern open $D/work/PROBE.md`): env still had `TERN_PANE=6` and the loopback `TERN_PANE_SOCKET`; `tern ls --json` answered; `tern open` produced the same route entry and a board (`board:open -> 8`).
   - `tern help`, `tern new|split|run|send|events|inspect|close|keep-open|wait|process|pip|dock|move|capture|browser --help`: none takes a board or file-type argument; `tern open --help`: file blocks only.
4. Close detection.
   - `cx.layout:close(4)`, then pumping with `ctl run "true"` x3: window log `pane_closed 4`; `boards()` -> `{"open":false,"path":".../PROBE.md"}`. Board 6 closed with `ctl close` (focus on it): `pane_closed 6`. No `pane_created` line for the board panes in the log.
   - `tern events` started in the pane (`tern events > $D/events.log &`): after opening and closing boards the log held only `{"event":"title_changed","pane":2,"title":"true"}`-style lines from the pumping commands. `tern ls --json` while board 6 was open: `blocks: [{"id":2,"program":"zsh","live":true}]` only; `tern inspect --json` same single block.
   - `pane_closed` -> split command: `stop.signal` contained `closed-17592186047268`; the same handler, unguarded, produced a `pane_created`/`pane_closed`/`split launched` cycle for every spawned Pane (log of about 4000 lines, then the serve was quit).

## Unverified

- Everything with a Windows window and an Ubuntu host: whether `cx.board:open(<host path>)`, Open board… (picker pick) and `open_as_board` read and write the host's file, and `exists`/`phase` of the resulting board; the host's shell parsing of the Pane command (zsh here); `tern open` from an Ubuntu Pane reaching a Windows window with the same `req` fields.
- Whether the plugin's host half would be loaded on a real remote host, and that its `cx:open` arrives at the window with the host name of that machine (the loopback host name was `devbox`).
- Window quit / network drop and what Sync should do then.
- `how = "preview"` placement, and a user dragging a board into another tab or into a picture-in-picture (no probe).
