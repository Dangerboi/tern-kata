--!strict

-- The host half runs in Tern's session daemon, with or without a window. It keeps `tern-kata worker` running: it starts
-- the worker when the Supervision rule holds, and starts it again, backing off, whenever it exits. A worker it did not
-- start is left running: one its earlier load started (a plugin reload or a Tern update's handoff loads this file again
-- while the worker runs on, and Tern never calls the old exit callback) or one started by hand. It starts one within a
-- poll of that worker's end.
-- Turning it off: create worker.manual beside worker.toml (see docs/guide/worker.md).
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
local delay = MIN_DELAY
local next_start = 0
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
local handle: TimerHandle? = nil

-- The next start comes at once and at the shortest delay, as after a healthy run.
local function reset_backoff()
    delay = MIN_DELAY
    next_start = 0
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

-- The daemon's PATH, then the known folders that exist, so the worker finds omp however Tern was started.
local function worker_path(): string
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

local start

local function arm(ms: number)
    if handle then handle:cancel() end
    handle = tern.timer(ms, function()
        handle = nil
        start()
    end)
end

local function last_line(text: string): string
    local line = ""
    for l in text:gmatch("[^\r\n]+") do line = l end
    return line
end

-- Starts the worker if the backoff allows. config is worker.toml's text, which `worker supervised` printed.
local function launch(binary: string, config: string)
    local wait = next_start - tern.now()
    if wait > 0 then return arm(math.min(wait, POLL_MS)) end
    started_text = config
    local started = tern.now()
    local function exited(result: ProcessResult)
        running = false
        local ran = tern.now() - started
        if ran >= HEALTHY_MS or result.timed_out then delay = MIN_DELAY end
        tern.log.warn("tern-kata: the worker exited with status " .. result.status .. " after " .. ran // 1000 .. " s (" .. last_line(result.stderr) .. "); restarting in " .. delay // 1000 .. " s")
        next_start = tern.now() + delay
        delay = math.min(delay * 2, MAX_DELAY)
        arm(math.min(next_start - tern.now(), POLL_MS))
    end
    local ok, err = pcall(function()
        tern.process.run({binary, "worker"}, {
            cwd = home_dir(),
            env = { PATH = worker_path(), TERN_KATA_SUPERVISOR = "tern" },
            timeout_ms = RUN_LIMIT_MS,
        }, exited)
    end)
    if ok then
        running = true
        tern.log.warn("tern-kata: worker started (" .. binary .. ")")
        arm(POLL_MS)
    else
        tern.log.warn("tern-kata: could not start the worker: " .. tostring(err))
        next_start = tern.now() + delay
        delay = math.min(delay * 2, MAX_DELAY)
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
        reset_backoff()
        return arm(POLL_MS)
    end
    if result.status ~= 0 then
        if result.status == 1 and not result.timed_out then
            reset_backoff()
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
        reset_backoff()
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
        tern.process.run({binary, "worker", "supervised"}, {
            cwd = home_dir(),
            env = { PATH = worker_path() },
            timeout_ms = ASK_LIMIT_MS,
        }, function(result: ProcessResult) answered(binary, result) end)
    end)
    if not ok then
        asking = false
        tern.log.warn("tern-kata: could not ask whether the worker is supervised: " .. tostring(err))
        arm(POLL_MS)
    end
end

-- The first look waits a moment so loading stays well inside the 2 s budget. A daemon restart or a plugin reload loads this
-- file again: it starts the worker, or leaves the one still running and starts one when that ends.
arm(500)

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
