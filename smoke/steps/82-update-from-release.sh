# `tern-kata update` updates a release build from a release alone: a binary built with an older linker version sits in a
# temporary folder, a dist/ built by scripts/release.sh at the current version is served from a local port, and update (with
# TERN_KATA_BASE_URL pointing at it) replaces that binary, which then reports the new version. A second update says it is up to
# date. The state folder is the run's own (XDG_STATE_HOME), so the update lock never touches the real one.
up_version=$(sed -n 's/^version *= *"\(.*\)"/\1/p' "$repo/plugin.toml")
if [[ -n ${SMOKE_DIST:-} ]]; then
  up_dist=$SMOKE_DIST
else
  up_dist="$tmp/update-dist"
  "$repo/scripts/release.sh" "v$up_version" "$up_dist"
fi

python3 -m http.server 0 --bind 127.0.0.1 --directory "$up_dist" >"$tmp/update-server.log" 2>&1 &
up_server=$!
http_pids+=("$up_server")
up_url=
for ((attempt = 0; attempt < 100; attempt++)); do
  up_url=$(sed -n 's|^Serving HTTP on 127.0.0.1 port \([0-9]*\).*|http://127.0.0.1:\1|p' "$tmp/update-server.log")
  [[ -z $up_url ]] || break
  sleep 0.1
done
[[ -n $up_url ]]

up_bin="$tmp/update-bin/tern-kata"
(cd "$repo" && go build -ldflags "-X main.version=v0.0.1" -o "$up_bin" ./cmd/tern-kata)
[[ $("$up_bin" --version) == *"v0.0.1"* ]]

# This machine's plugin copy, installed older, comes along. The daemon socket points at nobody, so no running window reloads,
# and TERN_CONFIG_DIR (run.sh's own) keeps the real plugins folder out of it.
up_old_plugin="$tmp/update-old-plugin"
mkdir -p "$up_old_plugin"
cp "$repo/plugin.toml" "$repo"/*.luau "$up_old_plugin/" 2>/dev/null || true
sed -i 's/^version *=.*/version = "0.0.1"/' "$up_old_plugin/plugin.toml"
export TERN_DAEMON_SOCKET="$tmp/update-dead.sock"
[[ $(tern plugin dir) == "$TERN_CONFIG_DIR/plugins" ]]
tern plugin install "$up_old_plugin" --force >/dev/null
grep -q 'version = "0.0.1"' "$(tern plugin dir)/tern-kata/plugin.toml"

TERN_KATA_BASE_URL="$up_url" "$up_bin" update >"$tmp/update.out"
grep -q "Updated this machine's tern-kata plugin from 0.0.1 to $up_version" "$tmp/update.out"
grep -q "version = \"$up_version\"" "$(tern plugin dir)/tern-kata/plugin.toml"
grep -q "Updated tern-kata from v0.0.1 to v$up_version" "$tmp/update.out"
grep -q "Reopen tern-kata Panes to use v$up_version" "$tmp/update.out"
[[ $("$up_bin" --version) == *"v$up_version"* ]]
[[ $(ls -A "$tmp/update-bin") == tern-kata ]]

# An up-to-date run brings an older plugin copy along too.
tern plugin install "$up_old_plugin" --force >/dev/null
TERN_KATA_BASE_URL="$up_url" "$up_bin" update >"$tmp/update.out"
grep -q "tern-kata v$up_version is up to date" "$tmp/update.out"
grep -q "Updated this machine's tern-kata plugin from 0.0.1 to $up_version" "$tmp/update.out"
grep -q "version = \"$up_version\"" "$(tern plugin dir)/tern-kata/plugin.toml"

kill "$up_server" 2>/dev/null || true
wait "$up_server" 2>/dev/null || true
