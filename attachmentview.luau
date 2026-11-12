--!strict

-- Where a kata Attachment opens in Tern, as plain data: no Tern call is made here, so the rule is tested by
-- `luau test/attachmentview_test.luau`. The kata Pane opens an Attachment Tern can show with `tern open --preview <path>`, the
-- path in tern-kata's download folder on the Pane's machine (internal/data attachmentCacheDir: <user cache>/tern-kata/attachments).
-- window.luau's `tern.route.open` asks `claims` about each open and moves those to Tern's floating view unless the user chose
-- the side panel.

local M = {}

-- The open's path is in the download folder. The Pane's machine may be Windows while the window's is not, so either separator counts.
function M.isAttachment(path: string): boolean
    return path:find("[/\\]tern%-kata[/\\]attachments[/\\]") ~= nil
end

-- The open is the Pane's: a preview of a file in the download folder. An Attachment opened another way (`tern open --tab`, the
-- Files explorer) keeps the placement asked for.
function M.claims(path: string, how: string): boolean
    return how == "preview" and M.isAttachment(path)
end

-- The palette toggle: the new choice and its toast.
function M.toggle(sidePanel: boolean): (boolean, string)
    local side = not sidePanel
    return side, if side then "kata Attachments open in the side panel beside the Pane" else "kata Attachments open floating over the tab"
end

return M
