--!strict

-- The host half runs in Tern's session daemon, with or without a window. It keeps `tern-kata worker` running: it starts
-- the worker when the Supervision rule holds, and starts it again, backing off, whenever it exits.
-- Turning it off: create worker.manual beside worker.toml (see docs/guide/worker.md).
-- This file does not decide the rule or find the config folder: it asks `tern-kata worker supervised`, whose answer is
-- worker.SupervisionOn in internal/worker/supervision.go (the one place the rule is held; the Go tests pin it).

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

-- The answer of `tern-kata worker supervised`: status 0 and worker.toml's text when the supervision rule holds, status 1
-- when it does not. Any other status (a timeout, a binary that cannot run) says nothing about the rule, so the state stays;
-- a status that is neither a timeout nor -1 (killed by a signal) is logged once per daemon: an older tern-kata binary exits
-- 2 on `worker supervised`, and then the worker never starts until the binary is updated.
local function answered(binary: string, result: ProcessResult)
    asking = false
    if result.status ~= 0 then
        if result.status == 1 and not result.timed_out then
            reset_backoff()
            started_text = nil
        elseif not result.timed_out and result.status ~= -1 and not warned_status then
            warned_status = true
            tern.log.warn("tern-kata: `worker supervised` exited with status " .. result.status .. " (" .. last_line(result.stderr) .. "), so the worker is not started; the tern-kata binary is probably older than this plugin: run `tern-kata update`")
        end
        return arm(POLL_MS)
    end
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

-- The first look waits a moment so loading stays well inside the 2 s budget; a daemon restart reloads this file and
-- so starts the worker again.
arm(500)

-- Windows: Tern runs a Pane's command as `pwsh -c <command>`, which loads the user's PowerShell profile first. The Pane
-- commands this plugin starts (panecommands.luau) get `-NoProfile`; every other Pane, the user's shells included, starts as
-- it was. A synchronous filter under a 50 ms budget: pure table work, and any error leaves the spawn as it is.
if windows then
    -- A Tern that rejects the event must not stop the host half (worker supervision) running.
    pcall(function()
        tern.on("spawn", function(spec: any): any?
            return panecommands.withoutProfile(spec)
        end)
    end)
end
