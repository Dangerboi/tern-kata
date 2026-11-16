# Sourced by scripts/release.sh, scripts/dev-install.sh and smoke/ctl.sh (POSIX sh): what the plugin folder carries, and its version.

# The plugin folder every release archive carries; `tern plugin install <dir>` installs it. A require("./name") in a plugin file
# needs name.luau beside it, or the installed plugin would fail to load.
plugin_files="plugin.toml window.luau host.luau placement.luau owedaction.luau releases.luau panecommands.luau reopen.luau waitingtext.luau waitingagents.luau widgets.luau workersummary.luau workerwidget.luau attachmentview.luau promptfile.luau"

plugin_version() { # plugin_version <path to a plugin.toml>
	sed -n 's/^version *= *"\(.*\)"/\1/p' "$1"
}
