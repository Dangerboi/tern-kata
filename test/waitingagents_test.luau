--!strict

-- Tests of which kata agent waits for the user, on plain data, outside Tern: `luau test/waitingagents_test.luau`.

local waitingagents = require("../waitingagents")
local harness = require("./harness")

local test = harness.test

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

harness.finish()
