--!strict

-- The host half runs in Tern's session daemon, with or without a window. It keeps `tern-kata worker` running: it starts
-- the worker when the Supervision rule holds, and starts it again, backing off, whenever it exits. A worker it did not
-- start is left running: one its earlier load started (a plugin reload or a Tern update's handoff loads this file again
-- while the worker runs on) or one started by hand. On Tern 0.9.16+ the end of a worker its earlier load started comes as a
-- `process_exit` event: the host half then starts one at once and logs how that worker ended. Elsewhere (older Tern, a
-- worker started by hand, a daemon restart) it starts one within a poll of that worker's end.
-- While this load's worker runs, or the Supervision rule holds and the poll finds a worker already running, it also asks
-- `tern-kata worker status --json` and keeps a short summary of it (workersummary.luau).
-- Which half draws the kata worker widget (workerwidget.luau is its definition): on Tern 0.9.21+ this half does, on the
-- session host, so every window attached to that host shows the session host's worker, a remote one included; the summary
-- stays in memory and the widget is drawn again whenever it changes or goes. Before that this half cannot render widgets: it
-- stores the summary in tern.kv (workersummary.save), stamped with the time of every poll, and the window half draws its own
-- widget of the same id from it, labelled as this machine's worker (window.luau), dropping a summary no host half refreshes.
-- This load's worker is summarized until it exits, even after worker.manual appears, since nothing asks `worker supervised`
-- while it runs; the summary is dropped when no worker runs or, for a worker this load did not start, when the rule does not
-- hold (worker.manual, no worker.toml), and when a running worker's binary can no longer be found. A failed read keeps the
-- last summary (with a fresh stamp in the tern.kv case).
-- At most one question to the binary is out at a time, and every answer arms the next look; see `ask`.
-- Turning it off: create worker.manual beside worker.toml (see docs/guide/worker.md).
-- This file does not decide the rule, find the config folder or see which worker runs: it asks `tern-kata worker
-- supervised`, whose answer is worker.SupervisionOn and worker.RunningWorker in internal/worker/supervision.go (the one
-- place each is held; the Go tests pin them).

local panecommands = require("./panecommands")
local reopen = require("./reopen")
local workersummary = require("./workersummary")
local workerwidget = require("./workerwidget")

local os_name = tern.runtime.os
local windows = os_name == "windows"
local sep = windows and "\\" or "/"
local path_sep = windows and ";" or ":"

local MIN_DELAY = 1000
local MAX_DELAY = 60000
-- A worker that stayed up this long was healthy: its next restart starts the backoff over.
local HEALTHY_MS = 60000
local POLL_MS = 10000
-- The first look at a worker just started: its status is ready within moments, and the widget should not wait a whole poll.
local FIRST_LOOK_MS = 2000
-- tern.process.run can only kill by timeout, and an exited-and-restarted worker is safe (its runs are journaled).
local RUN_LIMIT_MS = 7 * 24 * 3600 * 1000

local running = false
-- worker.toml's text when the worker was last started: a different text on disk means the wait no longer applies.
local started_text: string? = nil
-- A question (`worker supervised` or `worker status --json`) is out; it is the only one, and its answer arms the next look.
local out = false
-- What the question asked about changed while it was out (the worker ended): its answer is dropped and the next look comes at once.
local stale = false
-- A look was asked for while the question was out: the answer's next look comes at once instead of after POLL_MS.
local again = false
local warned_missing = false
local warned_status = false
local warned_summary = false
local last_summary: workersummary.Summary? = nil
-- The worker widget is registered in this half (see the end of the file), and the encoded summary it last showed.
local worker_widget = false
local shown_text = ""
local ASK_LIMIT_MS = 10000
-- `worker supervised` exits with HELD, printing one line that names the worker, when the rule holds and a worker this
-- load did not start already runs. The line is logged once per worker: a reload (a file saved in a checkout linked with
-- `tern plugin link`, `tern plugin reload`) runs this file again, so the last line logged is kept in tern.kv, which outlives the load.
local HELD = 4
local KV_HELD = "supervision.held"
local held_line: string? = nil

local restart_delay = MIN_DELAY
-- Tern's clock (ms) before which no start is made.
local next_start = 0

-- The next start comes at once and at the shortest delay, as after a healthy run.
local function reset_backoff()
    restart_delay = MIN_DELAY
    next_start = 0
end

local function schedule_restart(): number
    local waited = restart_delay
    next_start = tern.now() + waited
    restart_delay = math.min(waited * 2, MAX_DELAY)
    return waited
end

local function join(...: string): string
    return table.concat({...}, sep)
end

local function env(name: string): string?
    local value = tern.getenv(name)
    if value == nil or value == "" then return nil end
    return value
end

local function split_path(): { string }
    local dirs = {}
    for dir in (env("PATH") or ""):gmatch("[^" .. path_sep .. "]+") do
        table.insert(dirs, dir)
    end
    return dirs
end

-- Folders the daemon's own PATH may lack (it takes the PATH of whatever started Tern): where install.sh/install.ps1,
-- Go, Homebrew and Tern's bundled omp put programs.
local function known_dirs(): { string }
    local dirs = {}
    local function add(dir: string?)
        if dir then table.insert(dirs, dir) end
    end
    local home = env("HOME")
    add(env("TERN_KATA_INSTALL_DIR"))
    if windows then
        local local_app = env("LOCALAPPDATA")
        add(local_app and join(local_app, "Programs", "tern-kata"))
        local profile = env("USERPROFILE")
        add(profile and join(profile, "go", "bin"))
        add(local_app and join(local_app, "Tern", "omp", "bin"))
    else
        add(home and join(home, ".local", "bin"))
        add(home and join(home, ".local", "share", "tern", "omp", "bin"))
        add(home and join(home, "Library", "Application Support", "tern", "omp", "bin"))
        add(home and join(home, "go", "bin"))
        add("/opt/homebrew/bin")
        add("/home/linuxbrew/.linuxbrew/bin")
        add("/usr/local/bin")
    end
    return dirs
end

local function find_binary(): string?
    local name = windows and "tern-kata.exe" or "tern-kata"
    for _, list in {split_path(), known_dirs()} do
        for _, dir in list do
            local candidate = join(dir, name)
            if tern.fs.exists(candidate) then return candidate end
        end
    end
    return nil
end

-- The daemon's PATH, then the known folders that exist, so a started program finds omp however Tern was started.
local function tool_path(): string
    local dirs = split_path()
    local seen: { [string]: boolean } = {}
    for _, dir in dirs do seen[dir] = true end
    for _, dir in known_dirs() do
        if not seen[dir] and tern.fs.exists(dir) then
            table.insert(dirs, dir)
            seen[dir] = true
        end
    end
    return table.concat(dirs, path_sep)
end

-- Tern starts a plugin's processes in the plugin's folder, and on Windows a folder some process works in can't be
-- replaced: a worker started there would make every reinstall of the plugin fail ("os error 32") while it runs. The
-- worker and its question start in the home folder instead (Tern's default when there is none).
local function home_dir(): string?
    return env(if windows then "USERPROFILE" else "HOME")
end

local function process_options(timeout_ms: number, extra_env: { [string]: string }?): ProcessOptions
    local vars = { PATH = tool_path() }
    if extra_env then
        for name, value in extra_env do vars[name] = value end
    end
    return { cwd = home_dir(), env = vars, timeout_ms = timeout_ms }
end

local start

local pending: TimerHandle? = nil

local function arm(ms: number)
    if pending then pending:cancel() end
    pending = tern.timer(ms, function()
        pending = nil
        start()
    end)
end

local function last_line(text: string): string
    local line = ""
    for l in text:gmatch("[^\r\n]+") do line = l end
    return line
end

-- The widget rendered here shows `last_summary`; Tern draws it again, in every window attached to this host, only when the
-- summary changed (a poll that finds the same worker makes no refresh).
local function show_worker()
    local text = if last_summary ~= nil then tern.json.encode(last_summary) else ""
    if text == shown_text then return end
    shown_text = text
    local ok, err = pcall(function() tern.chrome.refresh() end)
    if not ok then tern.log.warn("tern-kata: could not redraw the worker widget: " .. tostring(err)) end
end

-- The key may come from an earlier load of this file, so tern.kv is asked, not only what this load stored.
local function clear_summary()
    last_summary = nil
    warned_summary = false
    if worker_widget then show_worker() end
    workersummary.drop(tern.kv)
end

-- The delay after which the next look comes: at once when one was asked for meanwhile, else `soon` or POLL_MS.
local function next_look(soon_ms: number?)
    local asked = again
    again = false
    arm(if asked then 1 else soon_ms or POLL_MS)
end

-- Runs one question and, whatever happens to it, arms the next look (the only place a question's chain is re-armed): an
-- answer that raises is logged, and a question that cannot start is returned as its error text. `on_answer` returns the delay
-- of the next look when it is not POLL_MS. A stale answer (drop_answer was called while it was out) is dropped and the next
-- look comes at once.
local function ask(binary: string, args: { string }, on_answer: (ProcessResult) -> number?): string?
    local command = { binary }
    for _, arg in args do table.insert(command, arg) end
    out = true
    local ok, err = pcall(function()
        tern.process.run(command, process_options(ASK_LIMIT_MS), function(result: ProcessResult)
            out = false
            if stale then
                stale = false
                again = false
                return arm(1)
            end
            local answered_ok, delay = pcall(on_answer, result)
            if not answered_ok then
                tern.log.warn("tern-kata: could not handle the answer of `tern-kata " .. table.concat(args, " ") .. "`: " .. tostring(delay))
                delay = nil
            end
            -- An answer may ask a follow-up question, which arms the next look itself.
            if not out then next_look(delay) end
        end)
    end)
    if ok then return nil end
    out = false
    stale = false
    next_look()
    return tostring(err)
end

local function drop_answer()
    if out then stale = true end
end

local function summarize(binary: string)
    local function failed(why: string)
        if not warned_summary then
            warned_summary = true
            tern.log.warn("tern-kata: could not read the worker's status for the worker widget (" .. why .. "); keeping the last summary")
        end
    end
    local function store(summary: workersummary.Summary)
        if worker_widget then
            last_summary = summary
            show_worker()
            return
        end
        local store_err = workersummary.save(tern.kv, summary, os.time())
        if store_err == nil then
            last_summary = summary
        else
            failed("could not store it: " .. store_err)
        end
    end
    local function failed_read(why: string)
        failed(why)
        if last_summary ~= nil then store(last_summary) end
    end
    local err = ask(binary, {"worker", "status", "--json"}, function(result: ProcessResult): number?
        if result.status ~= 0 or result.timed_out then
            failed_read(if result.timed_out then "timed out" else "status " .. result.status .. ": " .. last_line(result.stderr))
            return nil
        end
        local decoded, status = pcall(tern.json.decode, result.stdout)
        local summary = decoded and workersummary.of(status) or nil
        if summary == nil then
            failed_read("unexpected output")
            return nil
        end
        warned_summary = false
        store(summary)
        return nil
    end)
    if err ~= nil then
        failed_read(err)
    end
end

-- Starts the worker if the backoff allows; returns the delay of the next look. config is worker.toml's text, which `worker
-- supervised` printed.
local function launch(binary: string, config: string): number
    local wait = next_start - tern.now()
    if wait > 0 then return math.min(wait, POLL_MS) end
    started_text = config
    local started = tern.now()
    local function exited(result: ProcessResult)
        running = false
        clear_summary()
        drop_answer()
        local ran = tern.now() - started
        if ran >= HEALTHY_MS or result.timed_out then restart_delay = MIN_DELAY end
        local waited = schedule_restart()
        tern.log.warn("tern-kata: the worker exited with status " .. result.status .. " after " .. ran // 1000 .. " s (" .. last_line(result.stderr) .. "); restarting in " .. waited // 1000 .. " s")
        arm(math.min(next_start - tern.now(), POLL_MS))
    end
    local ok, err = pcall(function()
        tern.process.run({binary, "worker"}, process_options(RUN_LIMIT_MS, { TERN_KATA_SUPERVISOR = "tern" }), exited)
    end)
    if ok then
        running = true
        tern.log.warn("tern-kata: worker started (" .. binary .. ")")
        return FIRST_LOOK_MS
    end
    tern.log.warn("tern-kata: could not start the worker: " .. tostring(err))
    schedule_restart()
    return math.min(next_start - tern.now(), POLL_MS)
end

-- The worker named last has gone (or the rule no longer holds): a worker named next is logged again.
local function forget_held()
    if held_line == nil then return end
    held_line = nil
    pcall(tern.kv.set, KV_HELD, nil)
end

-- The answer of `tern-kata worker supervised`: status 0 and worker.toml's text when the supervision rule holds and no
-- worker runs, HELD and a line naming the worker when one does, status 1 when the rule does not hold. Any other status (a
-- timeout, a binary that cannot run) says nothing about the rule, so the state stays; a status that is neither a timeout
-- nor -1 (killed by a signal) is logged once per daemon: an older tern-kata binary exits 2 on `worker supervised`, and
-- then the worker never starts until the binary is updated. A question still out when the plugin reloads never reaches
-- this function: it ends as a `process_exit` event, which worker_ended does not take for the worker's end. Returns the delay
-- of the next look when it is not POLL_MS (`ask`).
local function answered(binary: string, result: ProcessResult): number?
    if result.status == HELD and not result.timed_out then
        -- worker.lock would refuse a second worker: start none, and look again next poll.
        local line = last_line(result.stdout)
        if line ~= held_line then
            held_line = line
            local ok, logged = pcall(tern.kv.get, KV_HELD)
            if not ok or logged ~= line then
                pcall(tern.kv.set, KV_HELD, line)
                tern.log.warn("tern-kata: " .. line)
            end
        end
        reset_backoff()
        summarize(binary)
        return nil
    end
    if result.status ~= 0 then
        if result.status == 1 and not result.timed_out then
            reset_backoff()
            started_text = nil
            forget_held()
            clear_summary()
        elseif not result.timed_out and result.status ~= -1 and not warned_status then
            warned_status = true
            tern.log.warn("tern-kata: `worker supervised` exited with status " .. result.status .. " (" .. last_line(result.stderr) .. "), so the worker is not started; the tern-kata binary is probably older than this plugin: run `tern-kata update`")
        end
        return nil
    end
    forget_held()
    clear_summary()
    local config = result.stdout
    if started_text ~= nil and config ~= started_text then
        reset_backoff()
        started_text = nil
    end
    return launch(binary, config)
end

function start()
    if out then
        again = true
        return
    end
    local binary = find_binary()
    if binary == nil then
        clear_summary()
        if not running and not warned_missing then
            warned_missing = true
            tern.log.warn("tern-kata: not installed on the daemon's PATH or in a known folder, so a configured worker cannot be started; looking again every " .. POLL_MS // 1000 .. " s")
        end
        return arm(POLL_MS)
    end
    if running then return summarize(binary) end
    warned_missing = false
    local err = ask(binary, {"worker", "supervised"}, function(result: ProcessResult): number?
        return answered(binary, result)
    end)
    if err ~= nil then
        tern.log.warn("tern-kata: could not ask whether the worker is supervised: " .. err)
    end
end

-- Tern 0.9.16+ reports a `tern.process.run` child that ends after the VM that started it is gone as a `process_exit` event
-- in the new VM: the worker a reload or a Tern update in place left running, or a `worker supervised` question that was
-- out. The event always asks for a new look now. It is the worker's end only when its pid is the one in the last held
-- line (`worker <pid> ...`), which the poll logged in this load or an earlier one (tern.kv keeps it): then its status and
-- last stderr line are logged as `exited` would have, and the held line is forgotten. Any other pid logs nothing, and
-- start() asks nothing while a question is out or this load's own worker runs.
local function worker_ended(ev: any)
    local line = held_line
    if line == nil then
        local ok, logged = pcall(tern.kv.get, KV_HELD)
        if ok and type(logged) == "string" then line = logged end
    end
    local pid = line ~= nil and tonumber(line:match("^worker (%d+) ")) or nil
    if pid ~= nil and pid == ev.pid then
        local how = ev.timed_out and "timed out at the run limit" or "exited with status " .. ev.status
        tern.log.warn("tern-kata: worker " .. pid .. " " .. how .. " (" .. last_line(ev.stderr) .. "); starting a new one")
        -- The line may come from tern.kv alone (the reloaded VM's first poll has not answered), so forget_held sees it.
        held_line = line
        forget_held()
        drop_answer()
    end
    start()
end

-- The worker widget is offered from this half where Tern can (0.9.21+; see the header): `render` runs here, with the summary
-- the poll keeps. A window half's widget of the same id wins in its window, so the window half draws its own only when it
-- finds the summary in its tern.kv, which this half stores only while it has no widget to render it. A registration that
-- raises is logged and leaves the rest as it was. Tern before 0.9.21 may have no `tern.chrome` in the host half at all, so the
-- lookup itself must not raise.
local chrome = (tern :: any).chrome
if chrome == nil or chrome.widget == nil then
    tern.log.info("tern-kata: this Tern cannot offer widgets from the host half (before 0.9.21), so the worker widget is the window half's")
else
    local ok, err = pcall(function()
        tern.chrome.widget(workerwidget.ID, workerwidget.spec(tern.plugin.id, true, function() return last_summary end))
    end)
    if ok then
        worker_widget = true
        -- An earlier load, or an older tern-kata, may have left a summary the window half would read as a reason to draw its own.
        workersummary.drop(tern.kv)
    else
        tern.log.warn("tern-kata: could not register the worker widget: " .. tostring(err))
    end
end

-- The first look waits a moment so loading stays well inside the 2 s budget. A daemon restart or a plugin reload loads this
-- file again: it starts the worker, or leaves the one still running and starts one when that ends.
arm(500)

-- Tern before 0.9.16 raises on an event name it does not know, and then the poll alone notices the worker's end.
do
    local ok, err = pcall(function()
        tern.on("process_exit", worker_ended)
    end)
    if not ok then
        tern.log.info("tern-kata: Tern does not report a process that outlived a reload, so a worker's end is noticed by the poll: " .. tostring(err))
    end
end

-- Windows: Tern runs a Pane's command as `pwsh -c <command>`, which loads the user's PowerShell profile first. The Pane
-- commands this plugin starts (panecommands.luau) get `-NoProfile`; every other Pane, the user's shells included, starts as
-- it was. A synchronous filter under a 50 ms budget: pure table work, and any error leaves the spawn as it is.
if windows then
    -- A Tern that rejects the event must not stop the host half (worker supervision) running.
    local ok, err = pcall(function()
        tern.on("spawn", function(spec: any): any?
            return panecommands.withoutProfile(spec)
        end)
    end)
    if not ok then
        tern.log.warn("tern-kata: could not register the spawn filter, so Pane commands load the PowerShell profile: " .. tostring(err))
    end
end

-- A chat Tern opens again (after a restart, or in a fork) runs the command that ran it. For an agent the Pane started that is
-- the whole wrapper line, prompt file included, so the rule (reopen.luau) hands back the line without its prompt word and
-- Tern adds the resume flag and the session. A synchronous filter under a 50 ms budget: string work only. A Tern before 0.9.21
-- raises on the event name, and then a reopened agent runs as Tern puts it back.
do
    local ok, err = pcall(function()
        tern.on("reopen", function(ev: any): string?
            return reopen.command(ev)
        end)
    end)
    if not ok then
        tern.log.info("tern-kata: Tern does not offer the reopen hook, so a reopened agent runs without the tern-kata wrapper: " .. tostring(err))
    end
end
