--!strict

-- Tests of which kata agent waits for the user and what the widget bar says, on plain data, outside Tern:
-- `luau test/waitingagents_test.luau`.

local waitingagents = require("../waitingagents")

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

local function same(got: { number }, want: { number })
    assert(table.concat(got, ",") == table.concat(want, ","), "got " .. table.concat(got, ",") .. ", want " .. table.concat(want, ","))
end

local KATA = "omp @/state/agent-prompts/prompt-1.md"
local function isKata(command: string?): boolean
    return command ~= nil and command:find("agent%-prompts/prompt%-") ~= nil
end

local function agent(pane: number, state: string, command: string?): waitingagents.Agent
    return {pane = pane, state = state, command = command or KATA}
end

test("timed agents rank newest first", function()
    same(waitingagents.waiting({agent(1, "waiting_input"), agent(2, "waiting_input"), agent(3, "waiting_input")}, {[1] = 5, [2] = 9, [3] = 7}, isKata), {2, 3, 1})
end)

test("untimed agents follow the timed ones in list order", function()
    same(waitingagents.waiting({agent(4, "waiting_input"), agent(1, "waiting_input"), agent(3, "waiting_input"), agent(2, "waiting_input")}, {[1] = 5, [2] = 9}, isKata), {2, 1, 4, 3})
end)

test("without any times the list order stands", function()
    same(waitingagents.waiting({agent(3, "waiting_input"), agent(1, "waiting_input")}, {}, isKata), {3, 1})
end)

test("agents not started by kata are left out", function()
    same(waitingagents.waiting({agent(1, "waiting_input", "omp"), agent(2, "waiting_input", ""), agent(3, "waiting_input")}, {[1] = 9, [2] = 8, [3] = 1}, isKata), {3})
end)

test("agents that do not wait are left out", function()
    same(waitingagents.waiting({agent(1, "working"), agent(2, "idle"), agent(3, "exited"), agent(4, "waiting_input")}, {[1] = 9, [2] = 8, [3] = 7}, isKata), {4})
end)

test("a closing pane is left out", function()
    same(waitingagents.waiting({agent(1, "waiting_input"), agent(2, "waiting_input")}, {[1] = 2, [2] = 1}, isKata, 1), {2})
end)

test("segment says how many wait", function()
    local segments = waitingagents.segment(3, "plugin.tern-kata.go-to-waiting-agent")
    assert(segments ~= nil and #segments == 1)
    local segment = (segments :: any)[1]
    assert(segment.text == "3 waiting" and segment.tone == "warning" and segment.command == "plugin.tern-kata.go-to-waiting-agent")
end)

test("segment is hidden at 0", function()
    assert(waitingagents.segment(0, "x") == nil)
end)

local GO = "plugin.tern-kata.go-to-waiting-agent"

test("widget says how many wait, with the tooltip and the click", function()
    local view = waitingagents.widget(2, "A\nB", GO)
    assert(view ~= nil)
    assert(view.text == "2 waiting" and view.tone == "warning" and view.title == "A\nB" and view.command == GO)
end)

test("widget shows nothing at 0", function()
    assert(waitingagents.widget(0, "", GO) == nil)
end)

if failures > 0 then error(failures .. " test(s) failed") end
