# Sourced by run.sh, plugin.sh, supervision.sh and release-check.sh, which set $repo and $tmp; run.sh, plugin.sh and
# release-check.sh serve Tern on "$tmp/control.sock".
# Tern's own waits (`ready`, `expect`, ...) give up after a fixed 20 s that `tern ctl` cannot lengthen, and a shell
# starting while a dozen shards run at once can need longer. They are asked again until wait_limit seconds have passed;
# a condition that never comes still fails, and every other command runs once.
wait_limit=${SMOKE_WAIT_LIMIT:-120}
source "$repo/scripts/plugin-info.sh"
# Microseconds since the epoch; EPOCHREALTIME's radix character follows the locale.
epoch_us() { printf -v "$1" '%s' "${EPOCHREALTIME//[.,]/}"; }
# wait_for [-t seconds] <command...>: polls every 0.1 s until the command succeeds, for up to wait_limit seconds, and names
# the command when time runs out. Headless Tern runs plugin timers only on frames, so a caller waiting on one keeps frames coming.
wait_for() {
  local limit=$wait_limit now deadline
  if [[ ${1:-} == -t ]]; then limit=$2; shift 2; fi
  epoch_us deadline
  deadline=$((deadline + limit * 1000000))
  until "$@"; do
    epoch_us now
    (( now < deadline )) || { printf 'timed out after %s s waiting for: %s\n' "$limit" "$*" >&2; return 1; }
    sleep 0.1
  done
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
