#!/usr/bin/env bash
# sweep-daemons.sh [owner pid]: stops kata and Docbank daemons that smoke scripts started. run.sh, plugin.sh and
# supervision.sh export SMOKE_OWNER=$$ before they start one, and a daemon keeps that variable in its environment.
# With an owner: the daemons that script started (its cleanup; this also stops those a failed step started and never
# stopped). Without one: the daemons whose owner script is gone. A run killed before its cleanup (SIGKILL from a command
# timeout, the OOM killer) leaves its daemons running after the script is gone, and nothing else stops them. They then
# pile up in the cgroup of whatever started the run.
set -uo pipefail
[[ -d /proc ]] || exit 0
owner=${1:-}
for dir in /proc/[0-9]*; do
  read -r comm 2>/dev/null <"$dir/comm" || continue
  [[ $comm == kata || $comm == docbank ]] || continue
  args=$(tr '\0' ' ' 2>/dev/null <"$dir/cmdline") || continue
  [[ " $args" == *" daemon "* ]] || continue
  started_by=$(tr '\0' '\n' 2>/dev/null <"$dir/environ" | sed -n 's/^SMOKE_OWNER=//p') || continue
  [[ -n $started_by ]] || continue
  if [[ -n $owner ]]; then
    [[ $started_by == "$owner" ]] || continue
  else
    # The owner is alive while its pid still runs one of the smoke scripts; a reused pid runs something else.
    owner_args=$(tr '\0' ' ' 2>/dev/null <"/proc/$started_by/cmdline") || owner_args=
    [[ $owner_args != *run.sh* && $owner_args != *plugin.sh* && $owner_args != *supervision.sh* ]] || continue
  fi
  kill "${dir#/proc/}" 2>/dev/null || true
done
exit 0
