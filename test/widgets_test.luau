--!strict

-- Tests of what the widgets of the widget bar have in common, on plain data, outside Tern: `luau test/widgets_test.luau`.

local widgets = require("../widgets")

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

test("capped keeps up to five lines as they are", function()
    assert(table.concat(widgets.capped({}), "|") == "")
    assert(table.concat(widgets.capped({"1", "2", "3", "4", "5"}), "|") == "1|2|3|4|5")
end)

test("capped shows five lines then how many more", function()
    assert(table.concat(widgets.capped({"1", "2", "3", "4", "5", "6"}), "|") == "1|2|3|4|5|and 1 more")
    assert(table.concat(widgets.capped({"1", "2", "3", "4", "5", "6", "7", "8"}), "|") == "1|2|3|4|5|and 3 more")
end)

test("merge appends the widget after the others, which keep their place and style", function()
    local list: { any } = {{id = "clock", style = "analog"}, "cpu@ring"}
    local merged, added = widgets.merge(list, {"waiting"})
    assert(#merged == 3 and #added == 1 and added[1] == "waiting")
    assert(merged[1].id == "clock" and merged[1].style == "analog" and merged[2] == "cpu@ring" and merged[3].id == "tern-kata/waiting")
    assert(#list == 2)
end)

test("merge adds nothing twice", function()
    local first = widgets.merge({{id = "clock"}}, {"waiting"})
    local merged, added = widgets.merge(first, {"waiting"})
    assert(#merged == 2 and #added == 0)
    local _, addedPlain = widgets.merge({"tern-kata/waiting@text"}, {"waiting"})
    assert(#addedPlain == 0)
    local _, addedTable = widgets.merge({{id = "tern-kata/waiting", style = "text"}}, {"waiting"})
    assert(#addedTable == 0)
end)

test("merge fills an empty bar with every widget once", function()
    local merged, added = widgets.merge({}, {"waiting", "worker", "waiting"})
    assert(#merged == 2 and #added == 2 and merged[2].id == "tern-kata/worker")
end)

if failures > 0 then error(failures .. " test(s) failed") end
