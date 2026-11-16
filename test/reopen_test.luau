--!strict

-- Tests of the host half's rule for reopened agent chats, on plain data, outside Tern: `luau test/reopen_test.luau`.

local reopen = require("../reopen")
local promptfile = require("../promptfile")
local harness = require("./harness")

local test = harness.test

local function same(got: string?, want: string?)
    assert(got == want, "got " .. tostring(got) .. ", wanted " .. tostring(want))
end

local function event(fields: any): any
    local ev: any = { pane = 4, cwd = "/work", shell = "/bin/zsh", agent = "omp", session = "/s/a.jsonl", how = "restore", command = "omp" }
    for key, value in fields do ev[key] = value end
    return ev
end

local EXTENSION = "-e /home/me/.local/state/tern-kata/tern-kata.ts"
local PROMPT = "/state/agent-prompts/prompt-1-2.md"
local WINDOWS_PROMPT = "C:\\Users\\me\\state\\agent-prompts\\prompt-1-2.md"

test("an agent Pane's line loses the prompt word and names the file by flag", function()
    same(reopen.command(event({
        command = "tern-kata agent --as=me --address-prefix=me/omp --forward-comments '--ref=widget-lab#derq' -- omp --model x " .. EXTENSION .. " @" .. PROMPT,
    })), "tern-kata agent --prompt-file=" .. PROMPT .. " --as=me --address-prefix=me/omp --forward-comments '--ref=widget-lab#derq' -- omp --model x " .. EXTENSION)
end)

test("route flags and quoted words are kept as written", function()
    same(reopen.command(event({
        command = "tern-kata agent --home=/data/my\\ kata --server='http://h:7474' '--ref=p#1' -- omp --append-system-prompt 'two words' \"-e\" /x.ts '@/tmp/a b/agent-prompts/prompt-1.md'",
    })), "tern-kata agent '--prompt-file=/tmp/a b/agent-prompts/prompt-1.md' --home=/data/my\\ kata --server='http://h:7474' '--ref=p#1' -- omp --append-system-prompt 'two words' \"-e\" /x.ts")
end)

test("a command that is the wrapper without a prompt word needs no change", function()
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp -e /x.ts" })), nil)
end)

test("a line typed at a shell replaces the bare omp command", function()
    local line = "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp -e /x.ts"
    same(reopen.command(event({ command = "omp", line = line })), line)
    same(reopen.command(event({ command = "omp", line = line .. " @" .. PROMPT })),
        "tern-kata agent --prompt-file=" .. PROMPT .. " --address-prefix=me/omp --ref=p#1 -- omp -e /x.ts")
end)

test("a word that only looks like a prompt stays", function()
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp @ -e /x.ts" })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- @" .. PROMPT })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp @spec.md" })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp @" .. PROMPT .. ".token" })), nil)
end)

test("anything else is left alone", function()
    same(reopen.command(event({ command = "omp" })), nil)
    same(reopen.command(event({ command = "omp", line = "sandbox -k omp" })), nil)
    same(reopen.command(event({ command = "tern acp", agent = "acp" })), nil)
    same(reopen.command(event({ command = "tern-kata worker -- omp @" .. PROMPT })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 omp @" .. PROMPT })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp 'oops @" .. PROMPT })), nil)
    same(reopen.command(event({ command = "my-tern-kata agent --ref=p#1 -- omp @" .. PROMPT })), nil)
end)

test("a malformed event does not raise", function()
    same(reopen.command(nil), nil)
    same(reopen.command({}), nil)
    same(reopen.command({ shell = "/bin/zsh", command = 5 }), nil)
end)

test("a program given by path is still the wrapper", function()
    same(reopen.command(event({ command = "/home/me/.local/bin/tern-kata agent --ref=p#1 -- omp @" .. PROMPT })),
        "/home/me/.local/bin/tern-kata agent --prompt-file=" .. PROMPT .. " --ref=p#1 -- omp")
end)

test("Windows: tern-kata.exe by path, PowerShell quoting and backslashes", function()
    local shell = "C:\\Program Files\\PowerShell\\7\\pwsh.exe"
    same(reopen.command(event({
        shell = shell,
        command = "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent '--ref=p#1' -- omp -e C:\\Users\\me\\state\\tern-kata.ts '@" .. WINDOWS_PROMPT .. "'",
    })), "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent '--prompt-file=" .. WINDOWS_PROMPT .. "' '--ref=p#1' -- omp -e C:\\Users\\me\\state\\tern-kata.ts")
    same(reopen.command(event({
        shell = shell,
        command = "C:\\Users\\me\\bin\\TERN-KATA.EXE agent --ref=p#1 -- omp @" .. WINDOWS_PROMPT,
    })), "C:\\Users\\me\\bin\\TERN-KATA.EXE agent --prompt-file=" .. WINDOWS_PROMPT .. " --ref=p#1 -- omp")
end)

test("a quote the shell escapes does not end a word early", function()
    same(reopen.command(event({
        command = "tern-kata agent --ref=p#1 -- omp --append-system-prompt \"say \\\"hi\\\" @now\" @" .. PROMPT,
    })), "tern-kata agent --prompt-file=" .. PROMPT .. " --ref=p#1 -- omp --append-system-prompt \"say \\\"hi\\\" @now\"")
end)

test("a fork keeps its address flags but does not coordinate", function()
    same(reopen.command(event({
        how = "fork",
        command = "tern-kata agent --as=me --address-prefix=me/omp --forward-comments --message-from=bot --message-from boss --coordinator=me/lead-3 --coordinating '--ref=p#1' --home=/h -- omp -e /x.ts @" .. PROMPT,
    })), "tern-kata agent --as=me --address-prefix=me/omp --forward-comments --message-from=bot --message-from boss --coordinator=me/lead-3 '--ref=p#1' --home=/h -- omp -e /x.ts")
    same(reopen.command(event({
        how = "fork",
        command = "omp",
        line = "tern-kata agent --address-prefix me/omp --coordinating --ref=p#1 -- omp",
    })), "tern-kata agent --address-prefix me/omp --ref=p#1 -- omp")
end)

test("a fork of a line naming a secret file names the block it copies", function()
    for _, flag in { "--token-file=/s/prompt-1.md.token", "--dsn-file /s/prompt-1.md.dsn", "'--env-file=/s/prompt-1.md.env'" } do
        same(reopen.command(event({
            how = "fork",
            command = "tern-kata agent --address-prefix=me/omp --ref=p#1 " .. flag .. " -- omp @" .. PROMPT,
        })), "tern-kata agent --forked-from=4 --address-prefix=me/omp --ref=p#1 " .. flag .. " -- omp")
    end
end)

test("a fork of a fork names its own parent, and a reopened fork keeps its mark", function()
    local fork = "tern-kata agent --forked-from=4 --ref=p#1 --token-file=/state/agent-prompts/prompt-1.md.token -- omp -e /x.ts"
    same(reopen.command(event({ how = "fork", pane = 12, command = fork })),
        "tern-kata agent --forked-from=12 --ref=p#1 --token-file=/state/agent-prompts/prompt-1.md.token -- omp -e /x.ts")
    same(reopen.command(event({ how = "fork", pane = 12, command = "tern-kata agent --forked-from 4 --ref=p#1 --env-file /e -- omp" })),
        "tern-kata agent --forked-from=12 --ref=p#1 --env-file /e -- omp")
    same(reopen.command(event({ how = "restore", pane = 12, command = fork })), nil)
end)

test("PowerShell: a fork's mark goes after the agent word of a called path", function()
    same(reopen.command(event({
        how = "fork",
        shell = "C:\\Program Files\\PowerShell\\7\\pwsh.exe",
        command = "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent --address-prefix=me/omp '--ref=p#1' --token-file C:\\s\\prompt-1.md.token -- omp '@" .. WINDOWS_PROMPT .. "'",
    })), "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent --forked-from=4 --address-prefix=me/omp '--ref=p#1' --token-file C:\\s\\prompt-1.md.token -- omp")
end)

test("a resume keeps every flag", function()
    for _, how in { "resume", "restore" } do
        same(reopen.command(event({
            how = how,
            command = "tern-kata agent --address-prefix=me/omp --coordinating --ref=p#1 -- omp @" .. PROMPT,
        })), "tern-kata agent --prompt-file=" .. PROMPT .. " --address-prefix=me/omp --coordinating --ref=p#1 -- omp")
    end
end)

test("the prompt word of a reopened chat becomes a flag that still names the file", function()
    local reopened = reopen.command(event({ command = "tern-kata agent --ref=p#1 --token-file=" .. PROMPT .. ".token -- omp @" .. PROMPT }))
    same(reopened, "tern-kata agent --prompt-file=" .. PROMPT .. " --ref=p#1 --token-file=" .. PROMPT .. ".token -- omp")
    assert(promptfile.has(reopened), "the reopened line no longer names its prompt file")
    -- Tern may run the hook on the line it was given back: nothing more to change.
    same(reopen.command(event({ command = reopened })), nil)
end)

test("a line that names its prompt file by flag keeps one flag", function()
    same(reopen.command(event({ command = "tern-kata agent --prompt-file " .. PROMPT .. " --ref=p#1 -- omp @" .. PROMPT })),
        "tern-kata agent --prompt-file " .. PROMPT .. " --ref=p#1 -- omp")
end)

test("a fork drops the prompt-file flag, either form, and gets none back", function()
    same(reopen.command(event({ how = "fork", command = "tern-kata agent --prompt-file=" .. PROMPT .. " --ref=p#1 -- omp" })), "tern-kata agent --ref=p#1 -- omp")
    same(reopen.command(event({ how = "fork", command = "tern-kata agent --ref=p#1 --prompt-file " .. PROMPT .. " -- omp @" .. PROMPT })), "tern-kata agent --ref=p#1 -- omp")
end)

test("a fork's flags after the -- belong to omp and stay", function()
    same(reopen.command(event({
        how = "fork",
        command = "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp --coordinating --forked-from=9 --token-file=/t @" .. PROMPT,
    })), "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp --coordinating --forked-from=9 --token-file=/t")
end)

test("a fork names its block from a number or a numeric string, and from nothing else: never a rounded or malformed id", function()
    local line = "tern-kata agent --ref=p#1 --token-file=/t -- omp @" .. PROMPT
    same(reopen.command(event({ how = "fork", pane = "12", command = line })), "tern-kata agent --forked-from=12 --ref=p#1 --token-file=/t -- omp")
    same(reopen.command(event({ how = "fork", pane = "9007199254740993", command = line })),
        "tern-kata agent --forked-from=9007199254740993 --ref=p#1 --token-file=/t -- omp")
    local unusable: { any } = { "x", "", true, 9007199254740993, 2 ^ 53, 2 ^ 60, -1, 1.5, 0 / 0 }
    for _, pane in unusable do
        same(reopen.command(event({ how = "fork", pane = pane, command = line })), "tern-kata agent --ref=p#1 --token-file=/t -- omp")
    end
end)

test("a fork of a fork Tern names no usable pane for keeps its mark", function()
    local line = "tern-kata agent --forked-from=4 --ref=p#1 --token-file=/t -- omp"
    local unusable: { any } = { "x", "", 2 ^ 60, true }
    for _, pane in unusable do
        same(reopen.command(event({ how = "fork", pane = pane, command = line .. " @" .. PROMPT })), line)
    end
    local ev = event({ how = "fork", command = "tern-kata agent --forked-from 4 --ref=p#1 --token-file=/t -- omp @" .. PROMPT })
    ev.pane = nil
    same(reopen.command(ev), "tern-kata agent --forked-from 4 --ref=p#1 --token-file=/t -- omp")
end)

harness.finish()
