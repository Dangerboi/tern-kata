--!strict

-- The host half runs in Tern's session daemon, with or without a window. It keeps `tern-kata worker` running: it starts
-- the worker when the Supervision rule holds, and starts it again, backing off, whenever it exits. A worker it did not
-- start is left running: one its earlier load started (a plugin reload or a Tern update's handoff loads this file again
-- while the worker runs on, and Tern never calls the old exit callback) or one started by hand. It starts one within a
-- poll of that worker's end.
-- Turning it off: create worker.manual beside worker.toml (see docs/guide/worker.md).
-- It also keeps a local Docbank running, the same way: a second loop asks `tern-kata docbank supervised` and, when it
-- answers on, runs `docbank daemon status` and `docbank daemon start` when no Docbank runs. Docbank's own detached start
-- is used, so the host half never holds its process and a running Docbank, whoever started it, is left alone. Turning it
-- off: `tern-kata docbank supervise off` (see docs/guide/attachments.md).
-- This file does not decide the rule, find the config folder or see which worker runs: it asks `tern-kata worker
-- supervised`, whose answer is worker.SupervisionOn and worker.RunningWorker in internal/worker/supervision.go (the one
-- place each is held; the Go tests pin them).

local panecommands = require("./panecommands")

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
local warned_missing = false
local warned_status = false
local ASK_LIMIT_MS = 10000
-- `worker supervised` exits with HELD, printing one line that names the worker, when the rule holds and a worker this
-- load did not start already runs. The line is logged once per worker: a reload (each `git status` in a checkout linked
-- with `tern plugin link`) runs this file again, so the last line logged is kept in tern.kv, which outlives the load.
local HELD = 4
local KV_HELD = "supervision.held"
local held_line: string? = nil

-- Restart timing of one supervised program: the delay the next restart waits and the time before which none starts.
type Backoff = { delay: number, next: number }

local function new_backoff(): Backoff
    return { delay = MIN_DELAY, next = 0 }
end

local worker_backoff = new_backoff()

-- The next start comes at once and at the shortest delay, as after a healthy run.
local function reset_backoff(backoff: Backoff)
    backoff.delay = MIN_DELAY
    backoff.next = 0
end

-- Books the next start after the current delay and doubles the delay for the one after. Returns the delay waited.
local function schedule_restart(backoff: Backoff): number
    local waited = backoff.delay
    backoff.next = tern.now() + waited
    backoff.delay = math.min(waited * 2, MAX_DELAY)
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

local function find_binary(base: string?): string?
    local stem = base or "tern-kata"
    local name = windows and stem .. ".exe" or stem
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

-- What every process the host half starts gets: the home folder, the plugin's PATH, a time limit, extra variables.
local function process_options(timeout_ms: number, extra_env: { [string]: string }?)
    local vars = { PATH = tool_path() }
    if extra_env then
        for name, value in extra_env do vars[name] = value end
    end
    return { cwd = home_dir(), env = vars, timeout_ms = timeout_ms }
end

local start

-- A timer slot: arming it cancels the one pending, and when it fires it calls fire().
local function scheduler(fire: () -> ()): (number) -> ()
    local pending: TimerHandle? = nil
    return function(ms: number)
        if pending then pending:cancel() end
        pending = tern.timer(ms, function()
            pending = nil
            fire()
        end)
    end
end

local arm = scheduler(function() start() end)

local function last_line(text: string): string
    local line = ""
    for l in text:gmatch("[^\r\n]+") do line = l end
    return line
end

-- Starts the worker if the backoff allows. config is worker.toml's text, which `worker supervised` printed.
local function launch(binary: string, config: string)
    local wait = worker_backoff.next - tern.now()
    if wait > 0 then return arm(math.min(wait, POLL_MS)) end
    started_text = config
    local started = tern.now()
    local function exited(result: ProcessResult)
        running = false
        local ran = tern.now() - started
        if ran >= HEALTHY_MS or result.timed_out then worker_backoff.delay = MIN_DELAY end
        local waited = schedule_restart(worker_backoff)
        tern.log.warn("tern-kata: the worker exited with status " .. result.status .. " after " .. ran // 1000 .. " s (" .. last_line(result.stderr) .. "); restarting in " .. waited // 1000 .. " s")
        arm(math.min(worker_backoff.next - tern.now(), POLL_MS))
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
        schedule_restart(worker_backoff)
        arm(math.min(worker_backoff.next - tern.now(), POLL_MS))
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
-- then the worker never starts until the binary is updated.
local function answered(binary: string, result: ProcessResult)
    asking = false
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
        reset_backoff(worker_backoff)
        return arm(POLL_MS)
    end
    if result.status ~= 0 then
        if result.status == 1 and not result.timed_out then
            reset_backoff(worker_backoff)
            started_text = nil
            forget_held()
        elseif not result.timed_out and result.status ~= -1 and not warned_status then
            warned_status = true
            tern.log.warn("tern-kata: `worker supervised` exited with status " .. result.status .. " (" .. last_line(result.stderr) .. "), so the worker is not started; the tern-kata binary is probably older than this plugin: run `tern-kata update`")
        end
        return arm(POLL_MS)
    end
    forget_held()
    local config = result.stdout
    if started_text ~= nil and config ~= started_text then
        reset_backoff(worker_backoff)
        started_text = nil
    end
    launch(binary, config)
end

-- Called by the timer: asks the Go worker whether the supervision rule holds, and starts the worker when it does and none runs.
function start()
    if running then return arm(POLL_MS) end
    if asking then return end
    local binary = find_binary()
    if binary == nil then
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

-- ---- Docbank ----
-- The rule lives in Go (`tern-kata docbank supervised`: exit 0 and the line "on", or exit 1). The host half holds the
-- schedule only: the worker's poll and backoff, one log line per change (a start, a failed start), never one per poll.

local DOCBANK_START_LIMIT_MS = 30000
local docbank_backoff = new_backoff()
local docbank_started_at = 0
local docbank_failing = false
local docbank_asking = false
local docbank_warned_missing = false
local docbank_warned_status = false

local docbank_check
local docbank_arm = scheduler(function() docbank_check() end)

local function docbank_reset()
    reset_backoff(docbank_backoff)
    docbank_failing = false
end

-- Docbank is not running and the backoff allows: its own detached start, from the home folder with the plugin's PATH.
local function docbank_start(docbank: string)
    local wait = docbank_backoff.next - tern.now()
    if wait > 0 then return docbank_arm(math.min(wait, POLL_MS)) end
    schedule_restart(docbank_backoff)
    docbank_started_at = tern.now()
    local function failed(why: string)
        -- What is left of the backoff is the real wait before the retry; the timer only looks at least every poll.
        local left = math.max(docbank_backoff.next - tern.now(), 0)
        if not docbank_failing then
            docbank_failing = true
            tern.log.warn("tern-kata: could not start Docbank (" .. why .. "); trying again in " .. left // 1000 .. " s")
        end
        -- Never 0 ms: the smallest delay armed elsewhere in the plugin is 1 ms, and 0 is not documented to fire.
        docbank_arm(math.max(math.min(left, POLL_MS), 1))
    end
    local ok, err = pcall(function()
        tern.process.run({docbank, "daemon", "start"}, process_options(DOCBANK_START_LIMIT_MS), function(result: ProcessResult)
            if result.status == 0 and not result.timed_out then
                docbank_failing = false
                tern.log.warn("tern-kata: Docbank was not running; started it (" .. docbank .. " daemon start)")
                docbank_arm(POLL_MS)
            else
                failed("status " .. result.status .. ", " .. last_line(result.stderr ~= "" and result.stderr or result.stdout))
            end
        end)
    end)
    if not ok then failed(tostring(err)) end
end

-- `docbank daemon status` exits 0 when a daemon runs, whoever started it.
local function docbank_looked(docbank: string, result: ProcessResult)
    if result.status == 0 and not result.timed_out then
        -- A Docbank that has stayed up was healthy: its next start begins the backoff over.
        if tern.now() - docbank_started_at >= HEALTHY_MS then docbank_reset() end
        return docbank_arm(POLL_MS)
    end
    if result.timed_out then return docbank_arm(POLL_MS) end
    docbank_start(docbank)
end

local function docbank_answered(result: ProcessResult)
    docbank_asking = false
    if result.status ~= 0 or result.timed_out then
        if result.status == 1 and not result.timed_out then
            docbank_reset()
        elseif not result.timed_out and result.status ~= -1 and not docbank_warned_status then
            docbank_warned_status = true
            tern.log.warn("tern-kata: `docbank supervised` exited with status " .. result.status .. " (" .. last_line(result.stderr) .. "), so Docbank is not started; the tern-kata binary is probably older than this plugin: run `tern-kata update`")
        end
        return docbank_arm(POLL_MS)
    end
    local docbank = find_binary("docbank")
    if docbank == nil then
        if not docbank_warned_missing then
            docbank_warned_missing = true
            tern.log.warn("tern-kata: Docbank is supervised but `docbank` is not on the daemon's PATH or in a known folder; looking again every " .. POLL_MS // 1000 .. " s")
        end
        return docbank_arm(POLL_MS)
    end
    docbank_warned_missing = false
    local ok, err = pcall(function()
        tern.process.run({docbank, "daemon", "status"}, process_options(ASK_LIMIT_MS), function(status: ProcessResult)
            docbank_looked(docbank, status)
        end)
    end)
    if not ok then
        tern.log.warn("tern-kata: could not ask Docbank whether it runs: " .. tostring(err))
        docbank_arm(POLL_MS)
    end
end

-- Called by the timer: asks Go whether the plugin keeps Docbank running, and when it does, whether Docbank runs.
function docbank_check()
    if docbank_asking then return end
    local binary = find_binary()
    if binary == nil then return docbank_arm(POLL_MS) end -- the worker loop says so
    docbank_asking = true
    local ok, err = pcall(function()
        tern.process.run({binary, "docbank", "supervised"}, process_options(ASK_LIMIT_MS), docbank_answered)
    end)
    if not ok then
        docbank_asking = false
        tern.log.warn("tern-kata: could not ask whether Docbank is supervised: " .. tostring(err))
        docbank_arm(POLL_MS)
    end
end

-- The first look waits a moment so loading stays well inside the 2 s budget. A daemon restart or a plugin reload loads this
-- file again: it starts the worker, or leaves the one still running and starts one when that ends.
arm(500)
docbank_arm(700)

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
