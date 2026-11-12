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

test("the Pane's preview of a file in the download folder is claimed, on any machine's paths", function()
    assert(attachmentview.claims("/home/a/.cache/tern-kata/attachments/01J/x/abc/notes.md", "preview"))
    assert(attachmentview.claims("/Users/a/Library/Caches/tern-kata/attachments/01J/x/abc/shot.png", "preview"))
    assert(attachmentview.claims("C:\\Users\\a\\AppData\\Local\\tern-kata\\attachments\\01J\\x\\abc\\notes.md", "preview"))
end)

test("an Attachment opened another way keeps the placement asked for", function()
    for _, how in {"tab", "split", "beside", "default", "glimpse"} do
        assert(not attachmentview.claims("/home/a/.cache/tern-kata/attachments/01J/x/abc/notes.md", how), how)
    end
end)

test("a file outside the download folder is left alone", function()
    assert(not attachmentview.claims("/home/a/Code/tern-kata/GLOSSARY.md", "preview"))
    assert(not attachmentview.claims("/home/a/.cache/tern-kata-attachments/notes.md", "preview"))
    assert(not attachmentview.claims("/home/a/.cache/other/tern-kata/attachmentsx/notes.md", "preview"))
end)

test("the toggle flips the choice and says where they open now", function()
    local toSide, sideText = attachmentview.toggle(false)
    assert(toSide and sideText:find("side panel") ~= nil)
    local toFloat, floatText = attachmentview.toggle(true)
    assert(not toFloat and floatText:find("floating") ~= nil)
end)

if failures > 0 then error(failures .. " test(s) failed") end
