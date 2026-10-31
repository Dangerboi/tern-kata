--!strict

-- Tests of the agent tab placement rule on plain data, outside Tern: `luau smoke/placement_test.luau` (the Luau runtime,
-- https://luau.org; `brew install luau`).

local placement = require("../placement")

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

local function same(got: { placement.Move }, want: { { number } })
    local text = {}
    for _, move in got do table.insert(text, move.tab .. "@" .. move.position) end
    local wantText = {}
    for _, move in want do table.insert(wantText, move[1] .. "@" .. move[2]) end
    assert(table.concat(text, ",") == table.concat(wantText, ","), "moves " .. table.concat(text, ",")
        .. ", want " .. table.concat(wantText, ","))
end

-- Tabs 1..n in session 1, in order; a pane per tab with the same number plus 100 (so ids differ from tab ids).
local function tabs(n: number): { placement.TabRec }
    local list = {}
    for i = 1, n do table.insert(list, {id = i, session = 1, position = i}) end
    return list
end

local function snapshot(overrides: { [string]: any }): placement.Snapshot
    local base = {
        tabs = {},
        panes = {},
        agents = {},
        hidden = {},
        placed = {},
        focused = nil,
    }
    for key, value in overrides do (base :: any)[key] = value end
    return base :: placement.Snapshot
end

local function kata(pane: number, tab: number): placement.PaneRec
    return {pane = pane, tab = tab, kata = true}
end

local function plain(pane: number, tab: number): placement.PaneRec
    return {pane = pane, tab = tab, kata = false}
end

local function agent(pane: number, tab: number, marker: number): placement.PaneRec
    return {pane = pane, tab = tab, kata = false, marker = marker}
end

test("an agent tab at the end is moved beside its starter", function()
    -- Tabs: 1 kata (pane 10), 2 plain, 3 agent (pane 30).
    local plan = placement.plan(snapshot({
        tabs = tabs(3),
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 10)},
        agents = {{pane = 30, marker = 10}},
    }))
    assert(#plan.tried == 1 and plan.tried[1] == 30)
    same(plan.moves, {{3, 2}})
end)

test("an agent tab not listed yet is no candidate but keeps the run going", function()
    -- Tab 3 holds an agent seen only by its running line; tab 4 is the listed one. Both sit at the end behind a plain tab,
    -- so the listed one moves and the unlisted one is left alone.
    local plan = placement.plan(snapshot({
        tabs = tabs(4),
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 10), agent(40, 4, 10)},
        agents = {{pane = 40, marker = 10}},
    }))
    assert(#plan.tried == 1 and plan.tried[1] == 40)
    same(plan.moves, {{4, 2}})
end)

test("a trailing run of agent tabs after a kata Pane's tab stays where it is", function()
    -- Kata tab 2 sits directly before the run of agent tabs 3 and 4: Tern's append is already right of it.
    local plan = placement.plan(snapshot({
        tabs = tabs(4),
        panes = {plain(20, 1), kata(10, 2), agent(30, 3, 10), agent(40, 4, 10)},
        agents = {{pane = 30, marker = 10}, {pane = 40, marker = 10}},
    }))
    same(plan.moves, {})
    assert(#plan.tried == 2, "both agents are still tried")
end)

test("a trailing run of agent tabs after a plain tab moves in the order it was opened", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(5),
        panes = {
            kata(10, 1), plain(20, 2), plain(25, 3),
            agent(30, 4, 10), agent(40, 5, 10),
        },
        agents = {{pane = 30, marker = 10}, {pane = 40, marker = 10}},
    }))
    -- Highest first: tab 5 goes to 2, then tab 4 goes to 2, ahead of it: the final order is 1, 4, 5, 2, 3.
    same(plan.moves, {{5, 2}, {4, 2}})
end)

test("a tab after the run that is not an agent leaves the run alone", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(4),
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 10), plain(40, 4)},
        agents = {{pane = 30, marker = 10}},
    }))
    same(plan.moves, {})
end)

test("an agent is tried once: a pane already placed is skipped", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(3),
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 10)},
        agents = {{pane = 30, marker = 10}},
        placed = {[30] = true},
    }))
    assert(#plan.tried == 0)
    same(plan.moves, {})
end)

test("an agent in the hidden session is tried but never moved", function()
    local list = tabs(3)
    list[3].session = 2
    local plan = placement.plan(snapshot({
        tabs = list,
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 10)},
        agents = {{pane = 30, marker = 10}},
        hidden = {[3] = true},
    }))
    assert(#plan.tried == 1)
    same(plan.moves, {})
end)

test("an agent whose command carries no marker is not tried", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(3),
        panes = {kata(10, 1), plain(20, 2), plain(30, 3)},
        agents = {{pane = 30, marker = nil}},
    }))
    assert(#plan.tried == 0)
    same(plan.moves, {})
end)

test("a guessed starter: the focused kata Pane when the marker names no pane here", function()
    -- Two kata Panes (tabs 1 and 2); the marker 999 is a pane of the session host's machine.
    local plan = placement.plan(snapshot({
        tabs = tabs(4),
        panes = {kata(10, 1), kata(11, 2), plain(20, 3), agent(30, 4, 999)},
        agents = {{pane = 30, marker = 999}},
        focused = 10,
    }))
    -- Tab 3 (plain) is before the run, so the agent tab moves, beside the focused kata Pane's tab 1.
    same(plan.moves, {{4, 2}})
end)

test("a guessed starter: the only kata Pane when the focused pane is no kata Pane", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(3),
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 999)},
        agents = {{pane = 30, marker = 999}},
        focused = 20,
    }))
    same(plan.moves, {{3, 2}})
end)

test("a guessed starter: no home with several kata Panes and none focused leaves the tab", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(4),
        panes = {kata(10, 1), kata(11, 2), plain(20, 3), agent(30, 4, 999)},
        agents = {{pane = 30, marker = 999}},
        focused = 20,
    }))
    same(plan.moves, {})
end)

test("an exact starter beats the focused pane", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(4),
        panes = {kata(10, 1), kata(11, 2), plain(20, 3), agent(30, 4, 11)},
        agents = {{pane = 30, marker = 11}},
        focused = 10,
    }))
    same(plan.moves, {{4, 3}})
end)

test("a tab already right of its starter's tab is not moved", function()
    local plan = placement.plan(snapshot({
        tabs = tabs(3),
        panes = {plain(20, 1), kata(10, 2), agent(30, 3, 10)},
        agents = {{pane = 30, marker = 10}},
    }))
    same(plan.moves, {})
end)

test("tabs of another session are left alone", function()
    local list = tabs(3)
    list[1].session = 2
    local plan = placement.plan(snapshot({
        tabs = list,
        panes = {kata(10, 1), plain(20, 2), agent(30, 3, 10)},
        agents = {{pane = 30, marker = 10}},
    }))
    same(plan.moves, {})
end)

test("kataPaneFor prefers the exact starter, then the fallback, then the only one", function()
    local panes = {kata(10, 1), kata(11, 2), plain(20, 3)}
    assert((placement.kataPaneFor(panes, 11, 10) :: placement.PaneRec).pane == 11)
    assert((placement.kataPaneFor(panes, 999, 10) :: placement.PaneRec).pane == 10)
    assert(placement.kataPaneFor(panes, 999, 20) == nil)
    assert((placement.kataPaneFor({kata(10, 1)}, nil, nil) :: placement.PaneRec).pane == 10)
end)

if failures > 0 then
    print(failures .. " failed")
    error("placement tests failed", 0)
end
print("placement tests passed")
