--!strict

-- The Pane commands the window half starts on Windows, the host half's rule for dropping the PowerShell profile from them, and
-- the title with which a Pane asks the window half to interrupt an agent. Both halves read the command strings from here, so a
-- changed command cannot fall out of the match. No Tern call is made here: the rules are tested by `luau test/panecommands_test.luau`.

local M = {}

local SCRIPTS = "https://raw.githubusercontent.com/Dangerboi/tern-kata/main/"

M.SCRIPTS = SCRIPTS
M.KATA = "tern-kata"
M.UPDATE = "tern-kata update --pause"
-- Wrapped in powershell so it also works when the Pane's default shell is cmd, where `;` is no separator.
M.INSTALL_WINDOWS = "powershell -NoProfile -ExecutionPolicy Bypass -Command \"irm " .. SCRIPTS
    .. "install.ps1 | iex; Read-Host 'Press Enter to close'\""

-- The request the Pane makes of the window half to interrupt an agent: the Pane sets its own title to INTERRUPT_PREFIX and the
-- agent's block id as decimal text (internal/data InterruptTitlePrefix), the one thing a Pane can say to the window half, and then
-- sets "tern-kata" again. Returns the block id when the title is such a request, else nil.
M.INTERRUPT_PREFIX = "tern-kata interrupt "

function M.interruptBlock(title: any): string?
    if type(title) ~= "string" or title:sub(1, #M.INTERRUPT_PREFIX) ~= M.INTERRUPT_PREFIX then return nil end
    return title:sub(#M.INTERRUPT_PREFIX + 1):match("^(%d+)$")
end

local COMMANDS: { [string]: boolean } = {
    [M.KATA] = true,
    [M.UPDATE] = true,
    [M.INSTALL_WINDOWS] = true,
}

-- "C:\...\pwsh.exe" and "/usr/bin/pwsh" alike: the file name without its folder or ".exe", lower-cased.
local function shellName(program: string): string
    local name = program:match("([^/\\]*)$") or program
    name = name:lower()
    if name:sub(-4) == ".exe" then name = name:sub(1, -5) end
    return name
end

-- The spec with `-NoProfile` before `-c`, or nil to leave the spawn as it is. Changes only a PowerShell (pwsh or powershell)
-- running `[-NoLogo] -c|-Command <one of this plugin's Pane commands>` and nothing else, without `-NoProfile`; it never raises.
-- (A Pane started from a headless Tern window gets the leading `-NoLogo`; one started through the daemon does not.)
function M.withoutProfile(spec: any): any?
    local ok, result = pcall(function(): any?
        if type(spec) ~= "table" or type(spec.program) ~= "string" or type(spec.args) ~= "table" then return nil end
        local name = shellName(spec.program)
        if name ~= "pwsh" and name ~= "powershell" then return nil end
        local args = spec.args
        for _, arg in args do
            if type(arg) ~= "string" then return nil end
        end
        local at = 1
        if args[1] ~= nil and (args[1] :: string):lower() == "-nologo" then at = 2 end
        if #args ~= at + 1 then return nil end
        local flag = (args[at] :: string):lower()
        if flag ~= "-c" and flag ~= "-command" then return nil end
        if not COMMANDS[args[at + 1]] then return nil end
        local out = {}
        for key, value in spec do out[key] = value end
        local patched = {}
        for i, arg in args do
            if i == at then table.insert(patched, "-NoProfile") end
            table.insert(patched, arg)
        end
        out.args = patched
        return out
    end)
    if ok then return result end
    return nil
end

return M
