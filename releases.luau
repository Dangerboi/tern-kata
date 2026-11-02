--!strict

-- The release check's decisions as plain data: no Tern call is made here, so they are tested by
-- `luau test/releases_test.luau`. window.luau reads the clock, the environment, the store and the network, and acts on
-- what these functions answer.

export type Check = {
    -- `TERN_KATA_NO_UPDATE_CHECK` is non-empty.
    envOff: boolean,
    -- The window half's own folder is not `<tern plugin dir>/tern-kata`: the plugin is linked to a checkout.
    linked: boolean,
    -- The palette toggle says off.
    toggleOff: boolean,
    -- Seconds since the epoch (`os.time()`).
    now: number,
    -- When the last check was made, nil when there never was one.
    lastCheck: number?,
}

local M = {}

M.DAY = 24 * 60 * 60

-- The numbers of "0.7.0" or "v0.7.0", surrounding space ignored; nil for anything else, a pre-release included.
function M.parse(text: string): { number }?
    local a, b, c = text:match("^%s*v?(%d+)%.(%d+)%.(%d+)%s*$")
    if not a then return nil end
    return {tonumber(a) :: number, tonumber(b) :: number, tonumber(c) :: number}
end

-- -1, 0 or 1 as `a` is older than, the same as or newer than `b`; nil when either is not a version.
function M.compare(a: string, b: string): number?
    local left, right = M.parse(a), M.parse(b)
    if not left or not right then return nil end
    for i = 1, 3 do
        if left[i] < right[i] then return -1 end
        if left[i] > right[i] then return 1 end
    end
    return 0
end

-- `version = "0.6.0"` of a plugin.toml.
function M.pluginVersion(toml: string): string?
    return toml:match('^version%s*=%s*"([^"\n]+)"') or toml:match('\nversion%s*=%s*"([^"\n]+)"')
end

-- Path separators and a trailing one do not tell two spellings of a folder apart.
local function normalize(path: string): string
    local text = path:gsub("\\", "/"):gsub("/+$", "")
    return text
end

-- A copy installed from a release lives in `<tern plugin dir>/tern-kata`; any other folder is a checkout linked with
-- `tern plugin link`. A plugin dir that could not be asked (nil) counts the copy as installed.
function M.isLinked(pluginDir: string, ternPluginDir: string?): boolean
    if ternPluginDir == nil or normalize(ternPluginDir) == "" then return false end
    return normalize(pluginDir) ~= normalize(ternPluginDir) .. "/tern-kata"
end

-- What a stopped check is: the variable first, then a linked plugin, then the toggle; nil when checks are on.
function M.offBecause(check: Check): "variable" | "linked" | "toggle" | nil
    if check.envOff then return "variable" end
    if check.linked then return "linked" end
    if check.toggleOff then return "toggle" end
    return nil
end

-- Due when never made, over 24 hours ago, or made in the future (the clock was set back).
function M.checkDue(now: number, lastCheck: number?): boolean
    return lastCheck == nil or lastCheck > now or now - lastCheck >= M.DAY
end

function M.shouldCheck(check: Check): boolean
    return M.offBecause(check) == nil and M.checkDue(check.now, check.lastCheck)
end

-- The toast for the release `version.txt` names, and the version to store as toasted; nil when the release is not
-- newer than this plugin, was toasted before, or is no version. A newer release than the one toasted toasts again.
function M.toast(releaseText: string, own: string, toasted: string?): (string?, string?)
    local release = M.release(releaseText)
    if not release then return nil, nil end
    if M.compare(release, own) ~= 1 then return nil, nil end
    if toasted ~= nil and M.compare(release, toasted) ~= 1 then return nil, nil end
    local ownText = own:match("^%s*v?(.-)%s*$") :: string
    return "tern-kata v" .. release .. " is out (this window's plugin is v" .. ownText
        .. "). Run Update tern-kata from the palette.", release
end

-- The palette toggle: the state it stores and the toast that tells it.
function M.toggle(check: Check): (boolean, string)
    local off = not check.toggleOff
    local text = off and "tern-kata update checks are off" or "tern-kata update checks are on"
    if check.envOff then
        text ..= ". TERN_KATA_NO_UPDATE_CHECK is set, which keeps checks off on this machine"
    elseif check.linked then
        text ..= ". A linked plugin never checks"
    end
    return off, text
end

-- The version a `version.txt` names, as "0.7.0"; nil when the text is no version.
function M.release(text: string): string?
    local numbers = M.parse(text)
    if not numbers then return nil end
    return numbers[1] .. "." .. numbers[2] .. "." .. numbers[3]
end

-- The window machine's plugin step. Its pending key is `{version, at}`; a plugin loading finds it and says what to do.
M.PENDING_TTL = 10 * 60

-- "updated": this plugin is the version the step installed (toast, clear the key). "drop": the key is unusable or over
-- 10 minutes old (clear it, silently). "wait": an install may still be running (leave the key).
function M.pendingOutcome(pending: any, own: string, now: number): "updated" | "wait" | "drop"
    if type(pending) ~= "table" or type(pending.version) ~= "string" or type(pending.at) ~= "number" then return "drop" end
    if M.compare(pending.version, own) == 0 then return "updated" end
    if pending.at > now or now - pending.at > M.PENDING_TTL then return "drop" end
    return "wait"
end

local RAW = "https://raw.githubusercontent.com/Dangerboi/tern-kata/v"
local DOWNLOADS = "https://github.com/Dangerboi/tern-kata/releases/download/v"

-- The plugin-only install of `version`, run without a terminal: the command, its added environment and the text for its
-- stdin (nil when it needs none). The script comes from the version's tag; the files it installs come from that version's
-- download folder unless the user set their own.
-- On Windows the script is not on the command line (`powershell -Command "irm … | iex"`): starting a PowerShell whose
-- command line downloads and runs a script takes Windows 0.2 s to over 0.5 s to allow on the thread that starts it, which
-- in the window half is the UI thread, and a hook may take 0.5 s at most. Read from stdin it starts in a few ms.
function M.pluginStep(version: string, os: string, configuredBase: string?): ({ string }, { [string]: string }, string?)
    local env = {
        TERN_KATA_PLUGIN_ONLY = "1",
        TERN_KATA_BASE_URL = if configuredBase ~= nil and configuredBase ~= "" then configuredBase else DOWNLOADS .. version,
    }
    if os == "windows" then
        return {"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "-"}, env,
            "irm " .. RAW .. version .. "/install.ps1 | iex\r\n"
    end
    return {"sh", "-c", "script=$(curl -fsSL " .. RAW .. version .. "/install.sh) && printf '%s\\n' \"$script\" | sh"}, env, nil
end

return M
