--!strict

-- Tests of the owe-or-send rule on plain data, outside Tern: `luau test/owedaction_test.luau`.

local owedaction = require("../owedaction")

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

local function is(got: string, want: string)
    if got ~= want then error(string.format("got %s, want %s", got, want), 2) end
end

test("a Pane the plugin waits on is owed the action, running or not", function()
    is(owedaction.route(true, false), "owe")
    is(owedaction.route(true, true), "owe")
end)

test("a running Pane nobody waits on gets the action now", function()
    is(owedaction.route(false, true), "send")
end)

test("a Pane nobody waits on where kata quit is not reused", function()
    is(owedaction.route(false, false), "launch")
end)

if failures > 0 then error(failures .. " test(s) failed") end
