--!strict

-- Tests of what the kata worker widget shows, on plain data, outside Tern: `luau test/workerwidget_test.luau`.

local workersummary = require("../workersummary")
local widgets = require("../widgets")
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

local HELD = "plugin.tern-kata.show-held"
local OPEN = "plugin.tern-kata.open"

local function summary(running: number, held: { workersummary.Held }?, capacity: number?, inUse: number?): workersummary.Summary
    return {executor = "box", pid = 42, at = 0, running = running, capacity = capacity or 4, inUse = inUse or running, held = held or {}}
end

local function heldRuns(count: number): { workersummary.Held }
    local list: { workersummary.Held } = {}
    for index = 1, count do table.insert(list, {ref = "r" .. index, job = "job" .. index, reason = "why" .. index}) end
    return list
end

local function view(s: workersummary.Summary?): widgets.WidgetView
    return assert(workerwidget.view(s, HELD, OPEN))
end

test("no summary shows nothing", function()
    assert(workerwidget.view(nil, HELD, OPEN) == nil)
end)

test("nothing running is idle, muted, and opens kata", function()
    local v = view(summary(0))
    assert(v.text == "idle" and v.tone == "muted" and v.command == OPEN)
end)

test("running runs show muted and open kata", function()
    local v = view(summary(3))
    assert(v.text == "3 running" and v.tone == "muted" and v.command == OPEN)
end)

test("held runs show the warning tone and the held command", function()
    local v = view(summary(1, heldRuns(2)))
    assert(v.text == "2 held" and v.tone == "warning" and v.command == HELD)
end)

test("value is in use over capacity, and 0 for no capacity", function()
    assert(view(summary(1, nil, 4, 1)).value == 0.25)
    assert(view(summary(0, nil, 0, 0)).value == 0)
end)

test("the title starts with the worker", function()
    local first = (view(summary(0)).title :: string):match("^[^\n]*")
    assert(first == "Worker box on this machine (process 42)", tostring(first))
end)

test("the title lists up to five held runs then how many more, then the running count", function()
    local lines = (view(summary(2, heldRuns(7))).title :: string):split("\n")
    assert(#lines == 8, #lines)
    assert(lines[2] == "r1 job1: why1" and lines[6] == "r5 job5: why5")
    assert(lines[7] == "and 2 more" and lines[8] == "2 running")
end)

test("five held runs get no more line", function()
    local lines = (view(summary(0, heldRuns(5))).title :: string):split("\n")
    assert(#lines == 7 and lines[7] == "0 running")
end)

if failures > 0 then error(failures .. " failed") end
print("all passed")
