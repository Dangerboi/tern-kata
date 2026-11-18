--!strict

-- Tests of what the widgets of the widget bar have in common, on plain data, outside Tern: `luau test/widgets_test.luau`.

local widgets = require("../widgets")
local harness = require("./harness")

local test = harness.test

test("capped keeps up to five lines as they are", function()
    assert(table.concat(widgets.capped({}), "|") == "")
    assert(table.concat(widgets.capped({"1", "2", "3", "4", "5"}), "|") == "1|2|3|4|5")
end)

test("capped shows five lines then how many more", function()
    assert(table.concat(widgets.capped({"1", "2", "3", "4", "5", "6"}), "|") == "1|2|3|4|5|and 1 more")
    assert(table.concat(widgets.capped({"1", "2", "3", "4", "5", "6", "7", "8"}), "|") == "1|2|3|4|5|and 3 more")
end)

test("fit keeps a line that fits and cuts a longer one to the width with an ellipsis", function()
    local exact = string.rep("a", 45)
    assert(widgets.fit(exact) == exact and widgets.fit("") == "")
    assert(widgets.fit(exact .. "b") == string.rep("a", 44) .. "…")
end)

test("fit counts characters, not bytes", function()
    local fits = string.rep("é", 45)
    assert(widgets.fit(fits) == fits)
    assert(widgets.fit(fits .. "é") == string.rep("é", 44) .. "…")
end)

test("fit puts a title with line breaks on one line, so it stays one tooltip line", function()
    assert(widgets.fit("agent\nwaiting\r\nfor you") == "agent waiting for you")
end)

test("capped fits every line it shows", function()
    local long = string.rep("x", 100)
    local shown = widgets.capped({long, "short", long, long, long, long, long})
    assert(#shown == 6 and shown[6] == "and 2 more")
    for index = 1, 5 do assert((utf8.len(shown[index]) :: number) <= 45) end
    assert(shown[2] == "short")
end)

test("wrap breaks at spaces into lines that fit, and gives no lines for empty text", function()
    local lines = widgets.wrap("one two three\nfour", 9, 3)
    assert(table.concat(lines, "|") == "one two|three|four", table.concat(lines, "|"))
    assert(#widgets.wrap("", 9, 3) == 0 and #widgets.wrap("  \n ", 9, 3) == 0)
end)

test("wrap splits a word longer than a line, counting characters", function()
    assert(table.concat(widgets.wrap("abcdefghij k", 4, 5), "|") == "abcd|efgh|ij k")
    assert(table.concat(widgets.wrap(string.rep("é", 6), 4, 2), "|") == string.rep("é", 4) .. "|éé")
end)

test("wrap ends the last line in an ellipsis when text is left over", function()
    local lines = widgets.wrap("aaa bbb ccc ddd eee", 7, 2)
    assert(table.concat(lines, "|") == "aaa bbb|ccc dd…", table.concat(lines, "|"))
    assert(table.concat(widgets.wrap(string.rep("x", 10), 4, 2), "|") == "xxxx|xxx…")
end)

test("merge appends the widget after the others, which keep their place and style", function()
    local list: { any } = {{id = "clock", style = "analog"}, "cpu@ring"}
    local merged, added = widgets.merge(list, {"tern-kata/waiting"})
    assert(#merged == 3 and #added == 1 and added[1] == "tern-kata/waiting")
    assert(merged[1].id == "clock" and merged[1].style == "analog" and merged[2] == "cpu@ring" and merged[3].id == "tern-kata/waiting")
    assert(#list == 2)
end)

test("merge adds nothing twice", function()
    local first = widgets.merge({{id = "clock"}}, {"tern-kata/waiting"})
    local merged, added = widgets.merge(first, {"tern-kata/waiting"})
    assert(#merged == 2 and #added == 0)
    local _, addedPlain = widgets.merge({"tern-kata/waiting@text"}, {"tern-kata/waiting"})
    assert(#addedPlain == 0)
    local _, addedTable = widgets.merge({{id = "tern-kata/waiting", style = "text"}}, {"tern-kata/waiting"})
    assert(#addedTable == 0)
end)

test("merge fills an empty bar with every widget once", function()
    local merged, added = widgets.merge({}, {"tern-kata/waiting", "tern-kata/worker", "tern-kata/waiting"})
    assert(#merged == 2 and #added == 2 and merged[2].id == "tern-kata/worker")
end)

harness.finish()
