--!strict

-- The host half's rule for an agent chat Tern opens again after a restart, or in a fork. Tern runs or types the command that
-- ran the chat, then `--resume`, `--restore` or `--fork` and the session. An agent the Pane started runs as
-- `tern-kata agent ... -- omp ... @<prompt file>`, and Tern hands that whole line over as the command: the prompt word would
-- come before the resume flag, so omp would be sent the task again (or fail on a prompt file the Pane has deleted). The rule
-- gives the line back without that word. The line is cut where it stands, never quoted again, so each shell's own quoting
-- stays as Tern wrote it. No Tern call is made here: the rule is tested by `luau test/reopen_test.luau`.

local M = {}

type Word = { s: number, e: number, text: string }

-- The flags a fork's line loses. Without the first five the wrapper does not make the chat an Agent with its own address, so a
-- fork never takes the original's Agent address; `--forked-from` names the chat a fork was made from, and a fork of a fork gets
-- its own in place of its parent's. A flag that takes a value may give it as the next word.
local FORK_DROPS: { [string]: boolean } = {
    ["address-prefix"] = true,
    ["forward-comments"] = false,
    ["message-from"] = true,
    ["coordinator"] = true,
    ["coordinating"] = false,
    ["forked-from"] = true,
}

-- "C:\...\pwsh.exe" and "/usr/bin/zsh" alike: the file name without its folder or ".exe", lower-cased.
local function baseName(program: string): string
    local name = (program:match("([^/\\]*)$") or program):lower()
    if name:sub(-4) == ".exe" then name = name:sub(1, -5) end
    return name
end

-- The character that escapes the next one outside single quotes: PowerShell's backtick, none for cmd (so a Windows path's
-- backslashes stay what they are), a backslash for every other shell.
local function escapeOf(shell: string): string?
    local name = baseName(shell)
    if name == "pwsh" or name == "powershell" then return "`" end
    if name == "cmd" then return nil end
    return "\\"
end

-- The words of a command line with where each one starts and ends, and its text with the quotes taken off; nil when a quote
-- is left open. Quoted runs next to each other (PowerShell's doubled quote) join into one word.
local function words(line: string, escape: string?): { Word }?
    local out: { Word } = {}
    local n = #line
    local i = 1
    while i <= n do
        if line:sub(i, i):find("^%s") then
            i += 1
        else
            local start = i
            local text = ""
            while i <= n and not line:sub(i, i):find("^%s") do
                local c = line:sub(i, i)
                if c == "'" then
                    local close = line:find("'", i + 1, true)
                    if close == nil then return nil end
                    text ..= line:sub(i + 1, close - 1)
                    i = close + 1
                elseif c == '"' then
                    i += 1
                    while true do
                        if i > n then return nil end
                        local d = line:sub(i, i)
                        if d == '"' then break end
                        if escape ~= nil and d == escape and i < n then
                            i += 1
                            d = line:sub(i, i)
                        end
                        text ..= d
                        i += 1
                    end
                    i += 1
                elseif escape ~= nil and c == escape and i < n then
                    text ..= line:sub(i + 1, i + 1)
                    i += 2
                else
                    text ..= c
                    i += 1
                end
            end
            table.insert(out, { s = start, e = i - 1, text = text })
        end
    end
    return out
end

-- Where the wrapper's `--` and its `agent` word are when the line is `[&] tern-kata agent ... -- <command>`, else nil.
local function wrapperSeparator(ws: { Word }): (number?, number)
    local at = 1
    if ws[1] ~= nil and ws[1].text == "&" then at = 2 end
    local program, subcommand = ws[at], ws[at + 1]
    if program == nil or subcommand == nil then return nil, 0 end
    if baseName(program.text) ~= "tern-kata" or subcommand.text ~= "agent" then return nil, 0 end
    for k = at + 2, #ws - 1 do
        if ws[k].text == "--" then return k, at + 1 end
    end
    return nil, 0
end

-- The flags that name a private file the Pane wrote; a fork reads its own copy of each (`--forked-from`).
local SECRET_FILE_FLAGS: { [string]: boolean } = { ["token-file"] = true, ["dsn-file"] = true, ["env-file"] = true }

-- The line without its trailing `@<prompt file>` word and, for a fork, without the flags that give the chat an Agent address and,
-- when the line names a secret file, with `--forked-from=<forkedFrom>` (the block the fork copies, when Tern names it) after
-- `agent`, so the wrapper reads its own copies of the Pane's secret files. Nil when the line is not the wrapper's.
local function rewrite(line: string, shell: string, fork: boolean, forkedFrom: string?): string?
    local ws = words(line, escapeOf(shell))
    if ws == nil then return nil end
    local sep, agentWord = wrapperSeparator(ws)
    if sep == nil then return nil end
    local dropped: { [number]: boolean } = {}
    local last = ws[#ws]
    if #ws > sep + 1 and #last.text > 1 and last.text:sub(1, 1) == "@" then dropped[#ws] = true end
    local secrets = false
    if fork then
        local k = 1
        while k < sep do
            local name = ws[k].text:match("^%-%-([^=]+)")
            local takesValue = name ~= nil and FORK_DROPS[name]
            if name ~= nil and SECRET_FILE_FLAGS[name] then secrets = true end
            if name ~= nil and takesValue ~= nil then
                dropped[k] = true
                if takesValue and not ws[k].text:find("=", 1, true) and k + 1 < sep then
                    k += 1
                    dropped[k] = true
                end
            end
            k += 1
        end
    end
    if not secrets then forkedFrom = nil end
    local out = ""
    local previousEnd = 0
    for k, word in ws do
        if not dropped[k] then out ..= line:sub(previousEnd + 1, word.e) end
        if k == agentWord and forkedFrom ~= nil then out ..= " --forked-from=" .. forkedFrom end
        previousEnd = word.e
    end
    return (out:gsub("^%s+", ""))
end

-- The command line Tern should run for a reopened chat in place of ev.command, or nil to leave the event alone: for a
-- tern-kata agent wrapper in ev.command (an agent Pane's own command) or ev.line (the line typed at a shell, where
-- ev.command is the bare `omp`). Never raises: the hook is a synchronous filter under a 50 ms budget.
function M.command(ev: any): string?
    local ok, result = pcall(function(): string?
        if type(ev) ~= "table" or type(ev.shell) ~= "string" then return nil end
        local fork = ev.how == "fork"
        local forkedFrom: string? = nil
        if fork and type(ev.pane) == "number" and ev.pane >= 0 and ev.pane == math.floor(ev.pane) then
            forkedFrom = string.format("%d", ev.pane)
        end
        for _, source in { ev.command, ev.line } do
            if type(source) == "string" then
                local line = rewrite(source, ev.shell, fork, forkedFrom)
                if line ~= nil then
                    if line == ev.command then return nil end
                    return line
                end
            end
        end
        return nil
    end)
    if ok then return result end
    return nil
end

return M
