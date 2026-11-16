--!strict

-- Tests of what the kata worker widget shows, on plain data, outside Tern: `luau test/workerwidget_test.luau`.

local workersummary = require("../workersummary")
local workerwidget = require("../workerwidget")

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

local HELD = "plugin.tern-kata.show-held-runs"
local OPEN = "plugin.tern-kata.open"

local function summary(running: number, held: { workersummary.Held }?, capacity: number?, inUse: number?): workersummary.Summary
    return {executor = "box", pid = 42, running = running, capacity = capacity or 4, inUse = inUse or running, held = held or {}}
end

local function heldRuns(count: number): { workersummary.Held }
    local list: { workersummary.Held } = {}
    for index = 1, count do table.insert(list, {ref = "r" .. index, reason = "why" .. index}) end
    return list
end

local function view(s: workersummary.Summary?, onHost: boolean?): WidgetView?
    local current = s
    local spec = workerwidget.spec("tern-kata", if onHost == nil then true else onHost, function() return current end)
    return spec.render(nil)
end

local function shown(s: workersummary.Summary?, onHost: boolean?): WidgetView
    return assert(view(s, onHost))
end

local function titleLines(s: workersummary.Summary, onHost: boolean?): { string }
    return (shown(s, onHost).title :: string):split("\n")
end

test("the widget's id, name, icon and styles are the ones Tern sees", function()
    local spec = workerwidget.spec("tern-kata", true, function() return nil end)
    assert(workerwidget.ID == "worker" and spec.name == "kata worker" and spec.icon == "server")
    assert(#(spec.styles :: any) == 2 and (spec.styles :: any)[1] == "text" and (spec.styles :: any)[2] == "ring")
end)

test("the click commands are the plugin commands Tern runs", function()
    assert(workerwidget.OPEN_ID == "open" and workerwidget.SHOW_HELD_ID == "show-held-runs")
    assert(shown(summary(0)).command == "plugin.tern-kata.open")
    assert(shown(summary(0, heldRuns(1))).command == "plugin.tern-kata.show-held-runs")
    assert(shown(summary(0, heldRuns(1)), false).command == "plugin.tern-kata.show-held-runs")
end)

test("no summary shows nothing", function()
    assert(view(nil, true) == nil)
end)

test("nothing running is idle, muted, and opens kata", function()
    local v = shown(summary(0))
    assert(v.text == "idle" and v.tone == "muted" and v.command == OPEN)
end)

test("running runs show muted and open kata", function()
    local v = shown(summary(3))
    assert(v.text == "3 running" and v.tone == "muted" and v.command == OPEN)
end)

test("held runs show the warning tone and the held command", function()
    local v = shown(summary(1, heldRuns(2)))
    assert(v.text == "2 held" and v.tone == "warning" and v.command == HELD)
end)

test("value is in use over capacity, and 0 for no capacity", function()
    assert(shown(summary(1, nil, 4, 1)).value == 0.25)
    assert(shown(summary(0, nil, 0, 0)).value == 0)
end)

test("the host-drawn title names the session host's worker, not this machine", function()
    assert(titleLines(summary(0), true)[1] == "Worker on the session host")
end)

test("the window half's fallback title says the worker is on this machine", function()
    assert(titleLines(summary(0), false)[1] == "Worker on this machine")
end)

test("host and fallback views differ only in the title's place", function()
    local host, fallback = shown(summary(1, heldRuns(1)), true), shown(summary(1, heldRuns(1)), false)
    assert(host.text == fallback.text and host.tone == fallback.tone and host.command == fallback.command and host.value == fallback.value)
    assert(host.title ~= fallback.title)
end)

test("the title lists up to five held runs then how many more, then the running count, process and executor", function()
    local lines = titleLines(summary(2, heldRuns(7)))
    assert(#lines == 8, #lines)
    assert(lines[2] == "why1 r1" and lines[6] == "why5 r5")
    assert(lines[7] == "and 2 more" and lines[8] == "2 running, process 42, box")
end)

test("five held runs get no more line", function()
    local lines = titleLines(summary(0, heldRuns(5)))
    assert(#lines == 7 and lines[7] == "0 running, process 42, box")
end)

test("every title line fits Tern's width for a long executor, Issue ref and reason", function()
    local long = {
        executor = string.rep("e", 80), pid = 4242, running = 3, capacity = 4, inUse = 3,
        held = {
            {ref = "algonaut-trading-platform#a1b2c3d4", reason = "secret_unavailable: the token for the project is gone"},
            {ref = "r", reason = "agent_quota: " .. string.rep("w", 90)},
        },
    }
    for _, onHost in {true, false} do
        for _, line in titleLines(long, onHost) do
            assert((utf8.len(line) or #line) <= 45, line)
        end
    end
    local lines = titleLines(long)
    assert(lines[1] == "Worker on the session host")
    assert(lines[2]:sub(1, 19) == "secret_unavailable:" and lines[3]:sub(1, 13) == "agent_quota: ")
    assert(lines[4]:sub(1, 29) == "3 running, process 4242, eeee", lines[4])
end)

if failures > 0 then error(failures .. " test(s) failed") end
