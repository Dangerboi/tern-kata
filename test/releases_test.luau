--!strict

-- Tests of the release check's decisions on plain data, outside Tern: `luau test/releases_test.luau`.

local releases = require("../releases")

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

local NOW = 1_000_000

local function check(overrides: { [string]: any }): releases.Check
    local base: { [string]: any } = {envOff = false, linked = false, toggleOff = false, now = NOW, lastCheck = nil}
    for key, value in overrides do base[key] = value end
    return base :: any
end

test("a check is due when there never was one", function()
    assert(releases.shouldCheck(check({})))
end)

test("a check is due across the 24-hour line and not before", function()
    assert(not releases.shouldCheck(check({lastCheck = NOW - releases.DAY + 1})))
    assert(releases.shouldCheck(check({lastCheck = NOW - releases.DAY})))
    assert(releases.shouldCheck(check({lastCheck = NOW - releases.DAY - 1})))
    assert(not releases.shouldCheck(check({lastCheck = NOW})))
end)

test("a check stored in the future is due", function()
    assert(releases.shouldCheck(check({lastCheck = NOW + 100})))
end)

test("the variable wins over a toggle set on", function()
    assert(not releases.shouldCheck(check({envOff = true, toggleOff = false})))
    assert(releases.offBecause(check({envOff = true})) == "variable")
end)

test("the toggle off stops the check", function()
    assert(not releases.shouldCheck(check({toggleOff = true})))
    assert(releases.offBecause(check({toggleOff = true})) == "toggle")
end)

test("a linked plugin never checks, whatever the toggle says", function()
    assert(not releases.shouldCheck(check({linked = true, toggleOff = false})))
    assert(not releases.shouldCheck(check({linked = true, toggleOff = true})))
    assert(not releases.shouldCheck(check({linked = true, lastCheck = NOW - 10 * releases.DAY})))
end)

test("a plugin in tern's plugin dir is installed, any other folder is linked", function()
    assert(not releases.isLinked("/home/u/.config/tern/plugins/tern-kata", "/home/u/.config/tern/plugins"))
    assert(not releases.isLinked("/home/u/.config/tern/plugins/tern-kata", "/home/u/.config/tern/plugins/"))
    assert(not releases.isLinked("C:\\Users\\u\\tern\\plugins\\tern-kata", "C:\\Users\\u\\tern\\plugins"))
    assert(releases.isLinked("/home/u/Code/tern-kata", "/home/u/.config/tern/plugins"))
    assert(releases.isLinked("/home/u/.config/tern/plugins/tern-kata-dev", "/home/u/.config/tern/plugins"))
end)

test("a plugin dir that could not be asked counts the copy as installed", function()
    assert(not releases.isLinked("/home/u/Code/tern-kata", nil))
    assert(not releases.isLinked("/home/u/Code/tern-kata", ""))
end)

test("a newer release is toasted with the specified text", function()
    local text, version = releases.toast("0.7.0\n", "0.6.0", nil)
    assert(text == "tern-kata v0.7.0 is out (this window's plugin is v0.6.0). Run Update tern-kata from the palette.", text)
    assert(version == "0.7.0")
end)

test("a version is toasted once", function()
    assert(releases.toast("0.7.0\n", "0.6.0", "0.7.0") == nil)
    local text, version = releases.toast("0.8.0\n", "0.6.0", "0.7.0")
    assert(text ~= nil and version == "0.8.0")
end)

test("an equal or older release isn't toasted", function()
    assert(releases.toast("0.6.0\n", "0.6.0", nil) == nil)
    assert(releases.toast("0.5.9\n", "0.6.0", nil) == nil)
    assert(releases.toast("0.6.0", "0.10.0", nil) == nil)
end)

test("versions compare by number, not by text", function()
    assert(releases.compare("0.10.0", "0.9.0") == 1)
    assert(releases.compare("v1.0.0", "1.0.0") == 0)
    assert(releases.compare("0.6.0", "0.6.1") == -1)
    assert(releases.compare("garbage", "0.6.0") == nil)
end)

test("an unreadable version.txt isn't toasted", function()
    assert(releases.toast("<html>Not Found</html>", "0.6.0", nil) == nil)
    assert(releases.toast("", "0.6.0", nil) == nil)
    assert(releases.toast("0.7.0-rc1\n", "0.6.0", nil) == nil)
end)

test("the plugin's own version is read from plugin.toml", function()
    local toml = 'schema = 1\nid = "tern-kata"\nversion = "0.6.0"\ndescription = "x"\n'
    assert(releases.pluginVersion(toml) == "0.6.0")
    assert(releases.pluginVersion("schema = 1\n") == nil)
end)

test("the toggle flips the stored state and says why it changes nothing", function()
    local off, text = releases.toggle(check({toggleOff = false}))
    assert(off and text == "tern-kata update checks are off", text)
    local on, onText = releases.toggle(check({toggleOff = true}))
    assert(not on and onText == "tern-kata update checks are on", onText)
    local _, withVar = releases.toggle(check({envOff = true, toggleOff = true}))
    assert(withVar:find("TERN_KATA_NO_UPDATE_CHECK is set, which keeps checks off on this machine", 1, true), withVar)
    local _, linked = releases.toggle(check({linked = true}))
    assert(linked:find("A linked plugin never checks", 1, true), linked)
end)

test("versions compare by number, not by text", function()
    assert(releases.compare("0.10.0", "0.9.0") == 1)
    assert(releases.compare("0.9.0", "0.10.0") == -1)
    assert(releases.compare("0.9.0", "v0.9.0") == 0)
    assert(releases.compare("0.9.0", "latest") == nil)
end)

test("version.txt text is read as a version", function()
    assert(releases.release("v0.7.0\n") == "0.7.0")
    assert(releases.release("<html>Not Found</html>") == nil)
end)

test("a pending key with the plugin's own version means the update worked", function()
    assert(releases.pendingOutcome({version = "0.7.0", at = NOW - 5}, "0.7.0", NOW) == "updated")
    assert(releases.pendingOutcome({version = "0.7.0", at = NOW - 3600}, "0.7.0", NOW) == "updated")
end)

test("a pending key of another version waits while fresh and is dropped after 10 minutes", function()
    assert(releases.pendingOutcome({version = "0.7.0", at = NOW - 60}, "0.6.0", NOW) == "wait")
    assert(releases.pendingOutcome({version = "0.7.0", at = NOW - 600}, "0.6.0", NOW) == "wait")
    assert(releases.pendingOutcome({version = "0.7.0", at = NOW - 601}, "0.6.0", NOW) == "drop")
    assert(releases.pendingOutcome({version = "0.7.0", at = NOW + 60}, "0.6.0", NOW) == "drop")
end)

test("a malformed pending key is dropped", function()
    assert(releases.pendingOutcome("0.7.0", "0.6.0", NOW) == "drop")
    assert(releases.pendingOutcome({version = "0.7.0"}, "0.6.0", NOW) == "drop")
end)

test("the plugin step per OS, from the version's tag and download folder", function()
    local argv, env = releases.pluginStep("0.7.0", "linux", nil)
    assert(table.concat(argv, " ") == "sh -c curl -fsSL https://raw.githubusercontent.com/Dangerboi/tern-kata/v0.7.0/install.sh | sh")
    assert(env.TERN_KATA_PLUGIN_ONLY == "1")
    assert(env.TERN_KATA_BASE_URL == "https://github.com/Dangerboi/tern-kata/releases/download/v0.7.0")
    local win = releases.pluginStep("0.7.0", "windows", "")
    assert(win[1] == "powershell" and win[6] == "irm https://raw.githubusercontent.com/Dangerboi/tern-kata/v0.7.0/install.ps1 | iex", win[6])
end)

test("the user's own TERN_KATA_BASE_URL is kept", function()
    local _, env = releases.pluginStep("0.7.0", "macos", "http://127.0.0.1:8000")
    assert(env.TERN_KATA_BASE_URL == "http://127.0.0.1:8000")
end)

if failures > 0 then
    print(failures .. " failed")
    error("failures")
end
print("all passed")
