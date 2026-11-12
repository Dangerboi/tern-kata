--!strict

-- The host half runs in Tern's session daemon, with or without a window. It keeps `tern-kata worker` running: it starts
-- the worker when the Supervision rule holds, and starts it again, backing off, whenever it exits. A worker it did not
-- start is left running: one its earlier load started (a plugin reload or a Tern update's handoff loads this file again
-- while the worker runs on) or one started by hand. On Tern 0.9.16+ the end of a worker its earlier load started comes as a
-- `process_exit` event: the host half then starts one at once and logs how that worker ended. Elsewhere (older Tern, a
-- worker started by hand, a daemon restart) it starts one within a poll of that worker's end.
-- While this load's worker runs, or the Supervision rule holds and the poll finds a worker already running, it also asks
-- `tern-kata worker status --json` and keeps a short summary of it (workersummary.luau) under tern.kv's
-- `worker.summary` for the window half's worker widget, stamped with the time of every poll so the widget can drop a summary
-- that no host half refreshes any more. This load's worker is summarized until it exits, even after
-- worker.manual appears, since nothing asks `worker supervised` while it runs; the key is removed when no worker runs or,
-- for a worker this load did not start, when the rule does not hold (worker.manual, no worker.toml). A failed read stores
-- the last summary again with a fresh stamp.
-- Turning it off: create worker.manual beside worker.toml (see docs/guide/worker.md).
-- This file does not decide the rule, find the config folder or see which worker runs: it asks `tern-kata worker
-- supervised`, whose answer is worker.SupervisionOn and worker.RunningWorker in internal/worker/supervision.go (the one
-- place each is held; the Go tests pin them).

local panecommands = require("./panecommands")
local workersummary = require("./workersummary")

local os_name = tern.runtime.os
local windows = os_name == "windows"
local sep = windows and "\\" or "/"
local path_sep = windows and ";" or ":"

local MIN_DELAY = 1000
local MAX_DELAY = 60000
-- A worker that stayed up this long was healthy: its next restart starts the backoff over.
local HEALTHY_MS = 60000
local POLL_MS = 10000
-- tern.process.run can only kill by timeout, and an exited-and-restarted worker is safe (its runs are journaled).
local RUN_LIMIT_MS = 7 * 24 * 3600 * 1000

local running = false
-- worker.toml's text when the worker was last started: a different text on disk means the wait no longer applies.
local started_text: string? = nil
-- A `worker supervised` question is out; its answer arms the next look.
local asking = false
-- The worker named in the held line ended while a question was out: a HELD answer to that question may name the dead worker,
-- so the next look comes at once instead of after POLL_MS.
local recheck = false
local warned_missing = false
local warned_status = false
-- A `worker status --json` question is out; like the supervised question it is the only one out, and its answer arms the next look.
local summarizing = false
-- A look was asked for while the status question was out: the answer arms it at once instead of after POLL_MS.
local look_again = false
local warned_summary = false
-- Bumped whenever the summary is cleared, so a status read that was out at that moment drops its answer.
local summary_epoch = 0
-- The summary last stored, so a failed read can store it again with a fresh stamp.
local last_summary: workersummary.Summary? = nil
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

-- No worker runs (or the rule no longer holds): the widget has nothing to show. The key may come from an earlier load of this
-- file, so tern.kv is asked, not only what this load stored.
local function clear_summary()
    summary_epoch += 1
    last_summary = nil
    warned_summary = false
    local ok, stored = pcall(tern.kv.get, workersummary.KEY)
    if ok and stored ~= nil then pcall(tern.kv.set, workersummary.KEY, nil) end
end

-- Asks the running worker's status and stores its summary, stamped with the time, on every poll. A failed read (a timeout, an
-- older binary without --json, output that names no worker) stores the last summary again with a fresh stamp and is logged
-- once until a read succeeds. The next look comes after it.
local function summarize(binary: string)
    summarizing = true
    local function done()
        summarizing = false
        local soon = look_again
        look_again = false
        arm(if soon then 1 else POLL_MS)
    end
    local function failed(why: string)
        if not warned_summary then
            warned_summary = true
            tern.log.warn("tern-kata: could not read the worker's status for the worker widget (" .. why .. "); keeping the last summary")
        end
    end
    -- Stamps the summary now and stores it; the stamp is what tells the window half that a host half is still polling.
    local function store(summary: workersummary.Summary)
        summary.at = os.time()
        local stored, storeErr = (pcall :: any)(tern.kv.set, workersummary.KEY, summary)
        if stored then
            last_summary = summary
        else
            failed("could not store it: " .. tostring(storeErr))
        end
    end
    -- A failed read still proves the host half is alive, so the last summary is stored again with a fresh stamp.
    local function restamp()
        if last_summary ~= nil then store(last_summary) end
    end
    -- A read that did not give a summary: it is logged, the last summary is stored again, and the next look is scheduled.
    local function failedRead(why: string)
        failed(why)
        restamp()
        done()
    end
    local epoch = summary_epoch
    local ok, err = pcall(function()
        tern.process.run({binary, "worker", "status", "--json"}, process_options(ASK_LIMIT_MS), function(result: ProcessResult)
            -- The summary was cleared while this read was out (the worker it asked about has ended): its answer says nothing now.
            if epoch ~= summary_epoch then return done() end
            if result.status ~= 0 or result.timed_out then
                return failedRead(if result.timed_out then "timed out" else "status " .. result.status .. ": " .. last_line(result.stderr))
            end
            local decoded, status = pcall(tern.json.decode, result.stdout)
            local summary = decoded and workersummary.of(status) or nil
            if summary == nil then
                return failedRead("unexpected output")
            end
            warned_summary = false
            store(summary)
            done()
        end)
    end)
    if not ok then
        failedRead(tostring(err))
    end
end

-- Starts the worker if the backoff allows. config is worker.toml's text, which `worker supervised` printed.
local function launch(binary: string, config: string)
    local wait = next_start - tern.now()
    if wait > 0 then return arm(math.min(wait, POLL_MS)) end
    started_text = config
    local started = tern.now()
    local function exited(result: ProcessResult)
        running = false
        clear_summary()
        -- A status read about the worker that just ended may be out: its answer must not arm the next look after POLL_MS and
        -- cancel the restart armed below, so it arms at once and the look that follows honours the backoff.
        if summarizing then look_again = true end
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
        arm(POLL_MS)
    else
        tern.log.warn("tern-kata: could not start the worker: " .. tostring(err))
        schedule_restart()
        arm(math.min(next_start - tern.now(), POLL_MS))
    end
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
-- this function: it ends as a `process_exit` event, which worker_ended does not take for the worker's end.
local function answered(binary: string, result: ProcessResult)
    asking = false
    local again = recheck
    recheck = false
    if result.status == HELD and not result.timed_out then
        -- The question may have seen the worker that has since ended: ask again at once, logging nothing for it.
        if again then return arm(1) end
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
        return summarize(binary)
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
        return arm(POLL_MS)
    end
    forget_held()
    clear_summary()
    local config = result.stdout
    if started_text ~= nil and config ~= started_text then
        reset_backoff()
        started_text = nil
    end
    launch(binary, config)
end

function start()
    if summarizing then
        look_again = true
        return
    end
    if asking then return end
    local binary = find_binary()
    if running then
        if binary == nil then return arm(POLL_MS) end
        return summarize(binary)
    end
    if binary == nil then
        clear_summary()
        if not warned_missing then
            warned_missing = true
            tern.log.warn("tern-kata: not installed on the daemon's PATH or in a known folder, so a configured worker cannot be started; looking again every " .. POLL_MS // 1000 .. " s")
        end
        return arm(POLL_MS)
    end
    warned_missing = false
    asking = true
    local ok, err = pcall(function()
        tern.process.run({binary, "worker", "supervised"}, process_options(ASK_LIMIT_MS), function(result: ProcessResult)
            answered(binary, result)
        end)
    end)
    if not ok then
        asking = false
        tern.log.warn("tern-kata: could not ask whether the worker is supervised: " .. tostring(err))
        arm(POLL_MS)
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
        recheck = asking
    end
    start()
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
