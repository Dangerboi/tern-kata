--!strict

-- Tests of the host half's rule for dropping the PowerShell profile, on plain data, outside Tern: `luau test/panecommands_test.luau`.

local panecommands = require("../panecommands")

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

local function spec(program: string, args: { string }): any
    return {program = program, args = args, cwd = "C:\\Users\\a", env = {TERN_PANE = "7"}}
end

local function equal(a: { string }, b: { string })
    assert(#a == #b, "length " .. #a .. " vs " .. #b)
    for i, v in a do assert(v == b[i], "arg " .. i .. ": " .. v .. " vs " .. b[i]) end
end

for _, program in {"C:\\Program Files\\PowerShell\\7\\pwsh.exe", "pwsh", "/usr/bin/pwsh",
    "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe", "powershell"} do
    for label, command in {kata = panecommands.KATA, update = panecommands.UPDATE, install = panecommands.INSTALL_WINDOWS} do
        test("adds -NoProfile: " .. label .. " under " .. program, function()
            local out = panecommands.withoutProfile(spec(program, {"-c", command}))
            assert(out ~= nil)
            equal(out.args, {"-NoProfile", "-c", command})
            assert(out.program == program and out.cwd == "C:\\Users\\a" and out.env.TERN_PANE == "7")
        end)
    end
end

test("-Command spelling and upper-case program", function()
    local out = panecommands.withoutProfile(spec("C:\\X\\PWSH.EXE", {"-Command", panecommands.KATA}))
    assert(out ~= nil)
    equal(out.args, {"-NoProfile", "-Command", panecommands.KATA})
end)

test("a leading -NoLogo is kept and -NoProfile goes before -c", function()
    local out = panecommands.withoutProfile(spec("powershell.exe", {"-NoLogo", "-c", panecommands.UPDATE}))
    assert(out ~= nil)
    equal(out.args, {"-NoLogo", "-NoProfile", "-c", panecommands.UPDATE})
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-NoLogo", "-NoProfile", "-c", panecommands.KATA})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-NoLogo", "-c", "ping localhost"})) == nil)
end)

test("the spec given is not changed", function()
    local input = spec("pwsh.exe", {"-c", panecommands.KATA})
    panecommands.withoutProfile(input)
    equal(input.args, {"-c", panecommands.KATA})
end)

test("another command is left alone", function()
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c", "ping localhost"})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c", "tern-kata worker"})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c", "tern-kata update"})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c", "echo tern-kata"})) == nil)
end)

test("a plain shell is left alone", function()
    assert(panecommands.withoutProfile(spec("pwsh.exe", {})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-NoLogo"})) == nil)
end)

test("another program is left alone", function()
    for _, program in {"cmd.exe", "C:\\Windows\\System32\\cmd.exe", "/bin/sh", "/bin/bash", "/usr/bin/zsh", "pwsh-preview.exe"} do
        assert(panecommands.withoutProfile(spec(program, {"-c", panecommands.KATA})) == nil, program)
    end
end)

test("-NoProfile already present is left alone", function()
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-NoProfile", "-c", panecommands.KATA})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c", panecommands.KATA, "-NoProfile"})) == nil)
end)

test("an unexpected argument layout is left alone", function()
    assert(panecommands.withoutProfile(spec("pwsh.exe", {panecommands.KATA})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-x", panecommands.KATA})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c"})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-c", panecommands.KATA, "extra"})) == nil)
    assert(panecommands.withoutProfile(spec("pwsh.exe", {"-NoExit", "-c", panecommands.KATA})) == nil)
end)

test("a malformed spec returns nil and does not raise", function()
    assert(panecommands.withoutProfile(nil) == nil)
    assert(panecommands.withoutProfile("pwsh") == nil)
    assert(panecommands.withoutProfile({}) == nil)
    assert(panecommands.withoutProfile({program = 5, args = {}}) == nil)
    assert(panecommands.withoutProfile({program = "pwsh", args = "-c"}) == nil)
    assert(panecommands.withoutProfile({program = "pwsh", args = {"-c", 3}}) == nil)
end)

if failures > 0 then
    print(failures .. " failed")
    error("failures")
end
print("all passed")
