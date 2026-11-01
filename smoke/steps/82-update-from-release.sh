# `tern-kata update` updates a release build from a release alone: a binary built with an older linker version sits in a
# temporary folder, a dist/ built by scripts/release.sh at the current version is served from a local port, and update (with
# TERN_KATA_BASE_URL pointing at it) replaces that binary, which then reports the new version. A second update says it is up to
# date. The state folder is the run's own (XDG_STATE_HOME), so the update lock never touches the real one.
up_version=$(sed -n 's/^version *= *"\(.*\)"/\1/p' "$repo/plugin.toml")
up_dist="$tmp/update-dist"
"$repo/scripts/release.sh" "v$up_version" "$up_dist"

python3 -m http.server 0 --bind 127.0.0.1 --directory "$up_dist" >"$tmp/update-server.log" 2>&1 &
up_server=$!
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

TERN_KATA_BASE_URL="$up_url" "$up_bin" update >"$tmp/update.out"
grep -q "Updated tern-kata from v0.0.1 to v$up_version" "$tmp/update.out"
grep -q "Reopen tern-kata Panes to use v$up_version" "$tmp/update.out"
[[ $("$up_bin" --version) == *"v$up_version"* ]]
[[ $(ls -A "$tmp/update-bin") == tern-kata ]]

TERN_KATA_BASE_URL="$up_url" "$up_bin" update >"$tmp/update.out"
grep -q "tern-kata v$up_version is up to date" "$tmp/update.out"

kill "$up_server" 2>/dev/null || true
wait "$up_server" 2>/dev/null || true
