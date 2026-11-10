# Sourced by run.sh, plugin.sh and release-check.sh, which set $tmp and serve Tern on "$tmp/control.sock".
# Tern's own waits (`ready`, `expect`, ...) give up after a fixed 20 s that `tern ctl` cannot lengthen, and a shell
# starting while a dozen shards run at once can need longer. They are asked again until wait_limit seconds have passed;
# a condition that never comes still fails, and every other command runs once.
wait_limit=${SMOKE_WAIT_LIMIT:-120}
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
