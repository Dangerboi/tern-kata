--!strict

-- Tests of the host half's rule for reopened agent chats, on plain data, outside Tern: `luau test/reopen_test.luau`.

local reopen = require("../reopen")

local failures = 0
local function test(name: string, body: () -> ())
    local ok, err = (pcall :: any)(body)
    if ok then
        print("ok   " .. name)
    else
        failures += 1
        print("FAIL " .. name .. ": " .. tostring(err))
    end
end

local function same(got: string?, want: string?)
    assert(got == want, "got " .. tostring(got) .. ", wanted " .. tostring(want))
end

local function event(fields: any): any
    local ev: any = { pane = 4, cwd = "/work", shell = "/bin/zsh", agent = "omp", session = "/s/a.jsonl", how = "restore", command = "omp" }
    for key, value in fields do ev[key] = value end
    return ev
end

local EXTENSION = "-e /home/me/.local/state/tern-kata/tern-kata.ts"

test("an agent Pane's line loses only the prompt word", function()
    same(reopen.command(event({
        command = "tern-kata agent --as=me --address-prefix=me/omp --forward-comments '--ref=widget-lab#derq' -- omp --model x " .. EXTENSION .. " @/state/agent-prompts/prompt-1.md",
    })), "tern-kata agent --as=me --address-prefix=me/omp --forward-comments '--ref=widget-lab#derq' -- omp --model x " .. EXTENSION)
end)

test("route flags and quoted words are kept as written", function()
    same(reopen.command(event({
        command = "tern-kata agent --home=/data/my\\ kata --server='http://h:7474' '--ref=p#1' -- omp --append-system-prompt 'two words' \"-e\" /x.ts '@/tmp/a b/prompt.md'",
    })), "tern-kata agent --home=/data/my\\ kata --server='http://h:7474' '--ref=p#1' -- omp --append-system-prompt 'two words' \"-e\" /x.ts")
end)

test("a command that is the wrapper without a prompt word needs no change", function()
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp -e /x.ts" })), nil)
end)

test("a line typed at a shell replaces the bare omp command", function()
    local line = "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp -e /x.ts"
    same(reopen.command(event({ command = "omp", line = line })), line)
    same(reopen.command(event({ command = "omp", line = line .. " @/tmp/prompt.md" })), line)
end)

test("a word that only looks like a prompt stays", function()
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp @ -e /x.ts" })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- @/tmp/prompt.md" })), nil)
end)

test("anything else is left alone", function()
    same(reopen.command(event({ command = "omp" })), nil)
    same(reopen.command(event({ command = "omp", line = "sandbox -k omp" })), nil)
    same(reopen.command(event({ command = "tern acp", agent = "acp" })), nil)
    same(reopen.command(event({ command = "tern-kata worker -- omp @/x.md" })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 omp @/x.md" })), nil)
    same(reopen.command(event({ command = "tern-kata agent --ref=p#1 -- omp 'oops @/x.md" })), nil)
    same(reopen.command(event({ command = "my-tern-kata agent --ref=p#1 -- omp @/x.md" })), nil)
end)

test("a malformed event does not raise", function()
    same(reopen.command(nil), nil)
    same(reopen.command({}), nil)
    same(reopen.command({ shell = "/bin/zsh", command = 5 }), nil)
end)

test("a program given by path is still the wrapper", function()
    same(reopen.command(event({ command = "/home/me/.local/bin/tern-kata agent --ref=p#1 -- omp @/x.md" })),
        "/home/me/.local/bin/tern-kata agent --ref=p#1 -- omp")
end)

test("Windows: tern-kata.exe by path, PowerShell quoting and backslashes", function()
    local shell = "C:\\Program Files\\PowerShell\\7\\pwsh.exe"
    same(reopen.command(event({
        shell = shell,
        command = "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent '--ref=p#1' -- omp -e C:\\Users\\me\\state\\tern-kata.ts 'C:\\a b\\x.md'",
    })), nil)
    same(reopen.command(event({
        shell = shell,
        command = "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent '--ref=p#1' -- omp -e C:\\Users\\me\\state\\tern-kata.ts '@C:\\Users\\me\\a b\\prompt.md'",
    })), "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent '--ref=p#1' -- omp -e C:\\Users\\me\\state\\tern-kata.ts")
    same(reopen.command(event({
        shell = shell,
        command = "C:\\Users\\me\\bin\\TERN-KATA.EXE agent --ref=p#1 -- omp @C:\\Users\\me\\prompt.md",
    })), "C:\\Users\\me\\bin\\TERN-KATA.EXE agent --ref=p#1 -- omp")
end)

test("a quote the shell escapes does not end a word early", function()
    same(reopen.command(event({
        command = "tern-kata agent --ref=p#1 -- omp --append-system-prompt \"say \\\"hi\\\" @now\" @/x.md",
    })), "tern-kata agent --ref=p#1 -- omp --append-system-prompt \"say \\\"hi\\\" @now\"")
end)

test("a fork drops the flags that give the chat an Agent address and names the block it copies", function()
    same(reopen.command(event({
        how = "fork",
        command = "tern-kata agent --as=me --address-prefix=me/omp --forward-comments --message-from=bot --message-from boss --coordinator=me/lead-3 --coordinating '--ref=p#1' --home=/h -- omp -e /x.ts @/state/prompt.md",
    })), "tern-kata agent --forked-from=4 --as=me '--ref=p#1' --home=/h -- omp -e /x.ts")
    same(reopen.command(event({
        how = "fork",
        command = "omp",
        line = "tern-kata agent --address-prefix me/omp --ref=p#1 -- omp",
    })), "tern-kata agent --forked-from=4 --ref=p#1 -- omp")
end)

test("a fork of a fork names its own parent, and a reopened fork keeps its mark", function()
    local fork = "tern-kata agent --forked-from=4 --ref=p#1 --token-file=/state/agent-prompts/prompt-1.md.token -- omp -e /x.ts"
    same(reopen.command(event({ how = "fork", pane = 12, command = fork })),
        "tern-kata agent --forked-from=12 --ref=p#1 --token-file=/state/agent-prompts/prompt-1.md.token -- omp -e /x.ts")
    same(reopen.command(event({ how = "fork", pane = 12, command = "tern-kata agent --forked-from 4 --ref=p#1 -- omp" })),
        "tern-kata agent --forked-from=12 --ref=p#1 -- omp")
    same(reopen.command(event({ how = "restore", pane = 12, command = fork })), nil)
end)

test("a fork Tern names no pane for still loses the address flags", function()
    same(reopen.command(event({ how = "fork", pane = "x", command = "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp @/x.md" })),
        "tern-kata agent --ref=p#1 -- omp")
end)

test("PowerShell: a fork's mark goes after the agent word of a called path", function()
    same(reopen.command(event({
        how = "fork",
        shell = "C:\\Program Files\\PowerShell\\7\\pwsh.exe",
        command = "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent --address-prefix=me/omp '--ref=p#1' -- omp '@C:\\Users\\me\\prompt.md'",
    })), "& 'C:\\Users\\me\\bin\\tern-kata.exe' agent --forked-from=4 '--ref=p#1' -- omp")
end)

test("a resume keeps the address flags", function()
    for _, how in { "resume", "restore" } do
        same(reopen.command(event({
            how = how,
            command = "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp @/x.md",
        })), "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp")
    end
end)

test("a fork's flags after the -- belong to omp and stay", function()
    same(reopen.command(event({
        how = "fork",
        command = "tern-kata agent --address-prefix=me/omp --ref=p#1 -- omp --coordinating --forked-from=9 @/x.md",
    })), "tern-kata agent --forked-from=4 --ref=p#1 -- omp --coordinating --forked-from=9")
end)

if failures > 0 then
    print(failures .. " failed")
    error("reopen tests failed")
end
