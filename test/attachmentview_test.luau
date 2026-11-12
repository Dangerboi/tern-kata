--!strict

-- Tests of where a kata Attachment opens, on plain data, outside Tern: `luau test/attachmentview_test.luau`.

local attachmentview = require("../attachmentview")

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

test("an Attachment in the download folder floats by default, on any machine's paths", function()
    assert(attachmentview.route("/home/a/.cache/tern-kata/attachments/01J/x/abc/notes.md", false) == "glimpse")
    assert(attachmentview.route("/Users/a/Library/Caches/tern-kata/attachments/01J/x/abc/shot.png", false) == "glimpse")
    assert(attachmentview.route("C:\\Users\\a\\AppData\\Local\\tern-kata\\attachments\\01J\\x\\abc\\notes.md", false) == "glimpse")
end)

test("the side panel choice leaves every open as Tern does it", function()
    assert(attachmentview.route("/home/a/.cache/tern-kata/attachments/01J/x/abc/notes.md", true) == nil)
end)

test("a file outside the download folder is left alone", function()
    assert(attachmentview.route("/home/a/Code/tern-kata/GLOSSARY.md", false) == nil)
    assert(attachmentview.route("/home/a/.cache/tern-kata-attachments/notes.md", false) == nil)
    assert(attachmentview.route("/home/a/.cache/other/tern-kata/attachmentsx/notes.md", false) == nil)
end)

test("the toggle flips the choice and says where they open now", function()
    local toSide, sideText = attachmentview.toggle(false)
    assert(toSide and sideText:find("side panel") ~= nil)
    local toFloat, floatText = attachmentview.toggle(true)
    assert(not toFloat and floatText:find("floating") ~= nil)
end)

if failures > 0 then error(failures .. " test(s) failed") end
