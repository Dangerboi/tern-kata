# Sourced by run.sh, plugin.sh, supervision.sh and release-check.sh, which set $repo and $tmp; run.sh, plugin.sh and
# release-check.sh serve Tern on "$tmp/control.sock".
# Tern's own waits (`ready`, `expect`, ...) give up after a fixed 20 s that `tern ctl` cannot lengthen, and a shell
# starting while a dozen shards run at once can need longer. They are asked again until wait_limit seconds have passed;
# a condition that never comes still fails, and every other command runs once.
wait_limit=${SMOKE_WAIT_LIMIT:-120}
source "$repo/scripts/plugin-info.sh"
# Microseconds since the epoch; EPOCHREALTIME's radix character follows the locale.
epoch_us() { printf -v "$1" '%s' "${EPOCHREALTIME//[.,]/}"; }
# trace_failures: names the file, line and command of any command that ends a script run by itself (plugin.sh, release-check.sh),
# which has no step name for run.sh to print. A command whose failure is handled (`||`, `if`) is not named.
trace_failures() {
  set -E
  trap 'printf "%s:%s failed: %s\n" "${BASH_SOURCE[0]##*/}" "$LINENO" "$BASH_COMMAND" >&2' ERR
}
# wait_for [-t seconds] [-i interval] <command...>: polls (every 0.1 s unless -i gives the seconds) until the command succeeds,
# for up to wait_limit seconds, and names the command when time runs out. Headless Tern runs plugin timers only on frames, so a
# caller waiting on one keeps frames coming.
wait_for() {
  local limit=$wait_limit interval=0.1 now deadline
  while :; do
    case ${1:-} in
      -t) limit=$2; shift 2 ;;
      -i) interval=$2; shift 2 ;;
      *) break ;;
    esac
  done
  epoch_us deadline
  deadline=$((deadline + limit * 1000000))
  until "$@"; do
    epoch_us now
    (( now < deadline )) || { printf 'timed out after %s s waiting for: %s\n' "$limit" "$*" >&2; return 1; }
    sleep "$interval"
  done
}
# wait_settled <command...>: polls the command (every 0.05 s, through wait_for) until what it prints is the same on three reads in
# a row, which is when whatever it reads has stopped moving. Callers that expect a change wait for it first, as a late effect
# would otherwise pass as settled.
wait_settled() {
  local prev=. same=0
  settled_read() {
    local cur
    cur=$("$@")
    if [[ $cur == "$prev" ]]; then
      (( ++same < 3 )) || return 0
    else
      same=0
    fi
    prev=$cur
    return 1
  }
  wait_for -i 0.05 settled_read "$@"
}
# socket_ready <pid> <socket>: the process has made its socket; a process that exited ends the smoke at once.
socket_ready() {
  [[ -S $2 ]] && return 0
  kill -0 "$1" 2>/dev/null || { printf 'process %s exited before it made %s\n' "$1" "$2" >&2; exit 1; }
  return 1
}
# tern_at_least <version>: the tern on PATH is that version or newer.
tern_at_least() {
  : "${tern_version:=$(tern --version | sed -n 's/^tern \([0-9.]*\).*/\1/p')}"
  [[ -n $tern_version ]] || { echo "cannot read tern's version" >&2; exit 1; }
  [[ $(printf '%s\n%s\n' "$tern_version" "$1" | sort -V | sed -n 1p) == "$1" ]]
}
# tree_has <text>: the rendered tree (toasts included, which are not terminal text) holds the text.
tree_has() { ctl tree | jq -e --arg text "$1" 'tostring | contains($text)' >/dev/null; }
# serve_dir <dir> <log>: serves the folder on a free local port, which python reports in the log; sets serve_pid and serve_url.
serve_dir() {
  serve_log=$2
  : >"$serve_log"
  python3 -m http.server 0 --bind 127.0.0.1 --directory "$1" >"$serve_log" 2>&1 &
  serve_pid=$!
  wait_for serve_url_logged
}
serve_url_logged() {
  serve_url=$(sed -n 's|^Serving HTTP on 127.0.0.1 port \([0-9]*\).*|http://127.0.0.1:\1|p' "$serve_log")
  [[ -n $serve_url ]]
}
# copy_plugin <dir>: the files a release carries, as `tern plugin install` would get them.
copy_plugin() {
  local f
  for f in $plugin_files; do cp "$repo/$f" "$1/"; done
}
# release_dist <var> <dir>: sets the variable to the release folder to serve: SMOKE_DIST when the run has one, else one built
# in <dir> at the plugin's current version.
release_dist() {
  if [[ -n ${SMOKE_DIST:-} ]]; then
    printf -v "$1" '%s' "$SMOKE_DIST"
  else
    build_release_dist "$2"
    printf -v "$1" '%s' "$2"
  fi
}
build_release_dist() { "$repo/scripts/release.sh" "v$(plugin_version "$repo/plugin.toml")" "$1" >/dev/null; }
# build_old_release <out>: the binary built with a release version older than any real one.
build_old_release() { (cd "$repo" && go build -ldflags "-X main.version=v0.0.1" -o "$1" ./cmd/tern-kata); }
# rect <selector>: the first match's "x y w h", whole pixels.
rect() { ctl tree "$1" | jq -er '.nodes[0].rect | map(floor) | "\(.[0]) \(.[1]) \(.[2]) \(.[3])"'; }
# drag <selector> <x> <y>: presses on the element's middle, moves in steps and releases at x y.
drag() {
  local ex ey ew eh
  read -r ex ey ew eh < <(rect "$1")
  local sx=$((ex + ew / 2)) sy=$((ey + eh / 2))
  printf 'down %s %s\nmove %s %s\nmove %s %s\nmove %s %s\nup %s %s\n' \
    "$sx" "$sy" $((sx + 6)) $((sy + 6)) $(((sx + $2) / 2)) $(((sy + $3) / 2)) "$2" "$3" "$2" "$3" >"$tmp/drag.ctl"
  ctl --file "$tmp/drag.ctl"
}
ctl_call() {
  case "${1:-}" in
    ready | expect | git-read | link-read | completing) ctl_waiting "$@" ;;
    plugins) if [[ ${2:-} == expect ]]; then ctl_waiting "$@"; else tern ctl --control "$tmp/control.sock" "$@"; fi ;;
    *) tern ctl --control "$tmp/control.sock" "$@" ;;
  esac
}
ctl_waiting() {
  local out deadline=$((SECONDS + wait_limit))
  while :; do
    if out=$(tern ctl --control "$tmp/control.sock" "$@"); then
      printf '%s\n' "$out"
      return 0
    fi
    if [[ $out != *'timed out after'* ]] || (( SECONDS >= deadline )); then
      printf '%s\n' "$out"
      return 1
    fi
  done
}
