# The install scripts install from a release alone: build dist/ with scripts/release.sh at the current version, serve it
# from a local port, and run install.sh against it with no git clone. Then plugin-only mode, which installs the plugin
# and nothing else, with no tern on PATH (a refusal) and over a linked copy (left as it is). Each run has its own
# TERN_CONFIG_DIR, which moves `tern plugin dir`, so the real plugins folder is never touched; the daemon socket points
# at a socket nobody listens on, so the installs reload no running window.
in_version=$(sed -n 's/^version *= *"\(.*\)"/\1/p' "$repo/plugin.toml")
if [[ -n ${SMOKE_DIST:-} ]]; then
  in_dist=$SMOKE_DIST
else
  in_dist="$tmp/install-dist"
  "$repo/scripts/release.sh" "v$in_version" "$in_dist"
fi
[[ $(<"$in_dist/version.txt") == "$in_version" ]]
in_plugin_files=$(sed -n 's/^plugin_files="\(.*\)"/\1/p' "$repo/scripts/release.sh")
[[ -n $in_plugin_files ]]
for in_archive in "$in_dist"/tern-kata_*; do
  case "$in_archive" in
    *.zip) in_listing=$(unzip -Z1 "$in_archive") ;;
    *) in_listing=$(tar -tzf "$in_archive") ;;
  esac
  for in_file in $in_plugin_files; do
    grep -qx "plugin/$in_file" <<<"$in_listing" || { echo "$in_archive lacks plugin/$in_file" >&2; return 1; }
  done
done

# in_serve DIR NAME: serves DIR on a free local port; sets in_url and in_server.
in_serve() {
  # The log exists before the server starts: a sed on a missing file would end the step under set -e.
  : >"$tmp/install-server-$2.log"
  python3 -m http.server 0 --bind 127.0.0.1 --directory "$1" >"$tmp/install-server-$2.log" 2>&1 &
  in_server=$!
  http_pids+=("$in_server")
  in_url=
  for ((attempt = 0; attempt < 100; attempt++)); do
    in_url=$(sed -n 's|^Serving HTTP on 127.0.0.1 port \([0-9]*\).*|http://127.0.0.1:\1|p' "$tmp/install-server-$2.log")
    [[ -z $in_url ]] || break
    sleep 0.1
  done
  [[ -n $in_url ]]
}
in_serve "$in_dist" main

# The scripts run with a PATH of their own tools, plus tern where the run has one.
in_tools="$tmp/install-tools"
mkdir -p "$in_tools/with-tern" "$in_tools/without-tern"
for in_tool in sh uname mktemp rm awk cut sha256sum tar gzip mkdir cp chmod mv cat sed curl dirname; do
  ln -s "$(command -v "$in_tool")" "$in_tools/without-tern/$in_tool"
  ln -s "$(command -v "$in_tool")" "$in_tools/with-tern/$in_tool"
done
ln -s "$(command -v tern)" "$in_tools/with-tern/tern"

# in_run PATH_DIR CONFIG_DIR INSTALL_DIR [ENV…]: install.sh against the served dist/, away from a terminal.
in_run() {
  local path_dir=$1 config=$2 install=$3
  shift 3
  env -i HOME="$tmp/install-home" PATH="$path_dir" TERN_CONFIG_DIR="$config" TERN_DAEMON_SOCKET="$tmp/install-none.sock" \
    TERN_KATA_BASE_URL="$in_url" TERN_KATA_INSTALL_DIR="$install" "$@" /bin/sh "$repo/install.sh" </dev/null
}
in_plugin_ok() { # CONFIG_DIR: the plugin folder holds the release's plugin files and the manifest's version
  local plugin="$1/plugins/tern-kata"
  for in_file in $in_plugin_files; do [[ -f "$plugin/$in_file" ]]; done
  [[ $(sed -n 's/^version *= *"\(.*\)"/\1/p' "$plugin/plugin.toml") == "$in_version" ]]
  [[ ! -e "$plugin/.git" ]]
}
mkdir -p "$tmp/install-home"

# Full install: the binary, and the plugin from the archive. No kata daemon answers for its own HOME, so the installed
# binary's pre-flight warns that it cannot check kata, and the install still succeeds.
in_run "$in_tools/with-tern" "$tmp/install-config-full" "$tmp/install-bin-full" >"$tmp/install-full.out" 2>"$tmp/install-full.err"
[[ $("$tmp/install-bin-full/tern-kata" --version) == *"v$in_version"* ]]
in_plugin_ok "$tmp/install-config-full"
grep -q "^warning: cannot check this machine's kata daemon (tern-kata v$in_version needs kata v" "$tmp/install-full.err"

# Against the run's own kata daemon, which has every feature, the pre-flight has nothing to say.
in_run "$in_tools/with-tern" "$tmp/install-config-full" "$tmp/install-bin-full" KATA_HOME="$KATA_HOME" >"$tmp/install-daemon.out" 2>&1
if grep -q '^warning:' "$tmp/install-daemon.out"; then
  echo "install against a current kata daemon printed a warning:" >&2
  cat "$tmp/install-daemon.out" >&2
  return 1
fi

# Plugin-only: the plugin and nothing else.
in_run "$in_tools/with-tern" "$tmp/install-config-only" "$tmp/install-bin-only" TERN_KATA_PLUGIN_ONLY=1 >"$tmp/install-only.out"
in_plugin_ok "$tmp/install-config-only"
[[ ! -e "$tmp/install-bin-only" ]]

# Plugin-only with no tern on PATH fails and says so.
if in_run "$in_tools/without-tern" "$tmp/install-config-none" "$tmp/install-bin-none" TERN_KATA_PLUGIN_ONLY=1 >"$tmp/install-none.out" 2>&1; then
  echo "plugin-only install without tern must fail" >&2
  return 1
fi
grep -q 'tern is not on PATH' "$tmp/install-none.out"
[[ ! -e "$tmp/install-bin-none" && ! -e "$tmp/install-config-none/plugins/tern-kata" ]]

# A linked copy is left as it is, with a message and exit 0, in plugin-only mode and in a full install.
mkdir -p "$tmp/install-linked-src"
for in_file in $in_plugin_files; do cp "$repo/$in_file" "$tmp/install-linked-src/"; done
TERN_CONFIG_DIR="$tmp/install-config-linked" TERN_DAEMON_SOCKET="$tmp/install-none.sock" tern plugin link "$tmp/install-linked-src" --json >/dev/null
in_run "$in_tools/with-tern" "$tmp/install-config-linked" "$tmp/install-bin-linked" TERN_KATA_PLUGIN_ONLY=1 >"$tmp/install-linked.out" 2>&1
grep -q 'is linked' "$tmp/install-linked.out"
[[ -f "$tmp/install-config-linked/plugins/tern-kata.path" && ! -e "$tmp/install-config-linked/plugins/tern-kata" ]]
in_run "$in_tools/with-tern" "$tmp/install-config-linked" "$tmp/install-bin-linked" >"$tmp/install-linked.out" 2>&1
grep -q 'is linked' "$tmp/install-linked.out"
[[ -x "$tmp/install-bin-linked/tern-kata" && -f "$tmp/install-config-linked/plugins/tern-kata.path" ]]

kill "$in_server" 2>/dev/null || true
wait "$in_server" 2>/dev/null || true

# A release without plugin/ (every release before the window plugin shipped) still installs the binary, which keeps
# rolling back to one possible; plugin-only mode against it refuses and says why.
in_old="$tmp/install-old-dist"
mkdir -p "$in_old" "$tmp/install-old-unpacked"
case $(uname -m) in aarch64 | arm64) in_cpu=arm64 ;; *) in_cpu=amd64 ;; esac
in_asset=tern-kata_$(uname -s | tr A-Z a-z)_$in_cpu.tar.gz
tar -xzf "$in_dist/$in_asset" -C "$tmp/install-old-unpacked"
rm -rf "$tmp/install-old-unpacked/plugin"
(cd "$tmp/install-old-unpacked" && tar -czf "$in_old/$in_asset" tern-kata LICENSE)
printf '%s  %s\n' "$(sha256sum "$in_old/$in_asset" | cut -d ' ' -f 1)" "$in_asset" >"$in_old/checksums.txt"
in_serve "$in_old" old
in_run "$in_tools/with-tern" "$tmp/install-config-old" "$tmp/install-bin-old" >"$tmp/install-old.out" 2>&1
[[ $("$tmp/install-bin-old/tern-kata" --version) == *"v$in_version"* ]]
grep -q 'no Tern window plugin' "$tmp/install-old.out"
[[ ! -e "$tmp/install-config-old/plugins/tern-kata" ]]
if in_run "$in_tools/with-tern" "$tmp/install-config-old" "$tmp/install-bin-old2" TERN_KATA_PLUGIN_ONLY=1 >"$tmp/install-old.out" 2>&1; then
  echo "plugin-only install from a release without a plugin must fail" >&2
  return 1
fi
grep -q 'carries no window plugin' "$tmp/install-old.out"
kill "$in_server" 2>/dev/null || true
wait "$in_server" 2>/dev/null || true
