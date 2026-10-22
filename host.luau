--!strict

-- The host half runs in Tern's session daemon, with or without a window. It keeps `tern-kata worker` running: it starts
-- the worker when worker.toml exists, and starts it again, backing off, whenever it exits. With no worker.toml it does nothing.
-- Turning it off: create worker.manual beside worker.toml (see the README).

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
local handle: TimerHandle? = nil

local function join(...: string): string
    return table.concat({...}, sep)
end

local function env(name: string): string?
    local value = tern.getenv(name)
    if value == nil or value == "" then return nil end
    return value
end

-- The folder os.UserConfigDir gives the binary, plus tern-kata.
local function config_dir(): string?
    local base: string?
    if windows then
        base = env("APPDATA")
    elseif os_name == "macos" then
        local home = env("HOME")
        base = home and join(home, "Library", "Application Support")
    else
        local home = env("HOME")
        base = env("XDG_CONFIG_HOME") or (home and join(home, ".config"))
    end
    return base and join(base, "tern-kata")
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

local function enabled(): boolean
    local dir = config_dir()
    if dir == nil then return false end
    return tern.fs.exists(join(dir, "worker.toml")) and not tern.fs.exists(join(dir, "worker.manual"))
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

-- Called by the timer: starts the worker when it is wanted and not running, else looks again later.
function start()
    if running then return arm(POLL_MS) end
    if not enabled() then return arm(POLL_MS) end
    local wait = next_start - tern.now()
    if wait > 0 then return arm(math.min(wait, POLL_MS)) end
    local binary = find_binary()
    if binary == nil then
        tern.log.warn("tern-kata: the worker is configured but tern-kata is not installed on the daemon's PATH or in a known folder; trying again in " .. delay // 1000 .. " s")
        next_start = tern.now() + delay
        delay = math.min(delay * 2, MAX_DELAY)
        return arm(math.min(next_start - tern.now(), POLL_MS))
    end
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

-- The first look waits a moment so loading stays well inside the 2 s budget; a daemon restart reloads this file and
-- so starts the worker again.
arm(500)
