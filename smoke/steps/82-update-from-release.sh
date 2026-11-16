# `tern-kata update` updates a release build from a release alone: a binary built with an older linker version sits in a
# temporary folder, a dist/ built by scripts/release.sh at the current version is served from a local port, and update (with
# TERN_KATA_BASE_URL pointing at it) replaces that binary, which then reports the new version. A second update says it is up to
# date. The state folder is the run's own (XDG_STATE_HOME), so the update lock never touches the real one.
up_version=$(plugin_version "$repo/plugin.toml")
if [[ -n ${SMOKE_DIST:-} ]]; then
  up_dist=$SMOKE_DIST
else
  up_dist="$tmp/update-dist"
  "$repo/scripts/release.sh" "v$up_version" "$up_dist"
fi

serve_dir "$up_dist" "$tmp/update-server.log"
http_pids+=("$serve_pid")

up_bin="$tmp/update-bin/tern-kata"
(cd "$repo" && go build -ldflags "-X main.version=v0.0.1" -o "$up_bin" ./cmd/tern-kata)
[[ $("$up_bin" --version) == *"v0.0.1"* ]]

# This machine's plugin copy, installed older, comes along. The daemon socket points at nobody, so no running window reloads,
# and TERN_CONFIG_DIR (run.sh's own) keeps the real plugins folder out of it.
up_old_plugin="$tmp/update-old-plugin"
mkdir -p "$up_old_plugin"
copy_plugin "$up_old_plugin"
sed -i 's/^version *=.*/version = "0.0.1"/' "$up_old_plugin/plugin.toml"
export TERN_DAEMON_SOCKET="$tmp/update-dead.sock"
[[ $(tern plugin dir) == "$TERN_CONFIG_DIR/plugins" ]]
tern plugin install "$up_old_plugin" --force >/dev/null
grep -q 'version = "0.0.1"' "$(tern plugin dir)/tern-kata/plugin.toml"

TERN_KATA_BASE_URL="$serve_url" "$up_bin" update >"$tmp/update.out"
grep -q "Updated this machine's tern-kata plugin from 0.0.1 to $up_version" "$tmp/update.out"
grep -q "version = \"$up_version\"" "$(tern plugin dir)/tern-kata/plugin.toml"
grep -q "Updated tern-kata from v0.0.1 to v$up_version" "$tmp/update.out"
grep -q "Reopen tern-kata Panes to use v$up_version" "$tmp/update.out"
[[ $("$up_bin" --version) == *"v$up_version"* ]]
[[ $(ls -A "$tmp/update-bin") == tern-kata ]]

# An up-to-date run brings an older plugin copy along too.
tern plugin install "$up_old_plugin" --force >/dev/null
TERN_KATA_BASE_URL="$serve_url" "$up_bin" update >"$tmp/update.out"
grep -q "tern-kata v$up_version is up to date" "$tmp/update.out"
grep -q "Updated this machine's tern-kata plugin from 0.0.1 to $up_version" "$tmp/update.out"
grep -q "version = \"$up_version\"" "$(tern plugin dir)/tern-kata/plugin.toml"

kill "$serve_pid" 2>/dev/null || true
wait "$serve_pid" 2>/dev/null || true
