#!/usr/bin/env bash
# Usage: smoke/sweep-daemons.sh [folder]
# Stops kata and Docbank daemons smoke runs started, found by the KATA_HOME or DOCBANK_HOME in their environment. run.sh,
# plugin.sh and supervision.sh keep every daemon home in their mktemp folder and write their pid to smoke-owner there.
# With a folder (a script's cleanup): every daemon whose home is in it, whoever started it (the script, a step, the Pane).
# With none: daemons whose home is in a mktemp folder that is gone, or whose folder's owner no longer runs a smoke script.
# A run killed before its cleanup (SIGKILL from a command timeout, the OOM killer) leaves those running, and nothing else
# stops them. Daemons of live runs, and daemons whose home is not in such a folder (the user's own), are left alone.
# No -e: a process can exit between any two reads of /proc, and a failed read skips that process.
set -uo pipefail
[[ -d /proc ]] || exit 0
folder=${1:-}
scratch=${TMPDIR:-/tmp}
scratch=${scratch%/}

# stale <home>: the home's mktemp folder is gone, or its smoke-owner pid no longer runs a smoke script (a reused pid that
# runs something else counts as gone). A folder without smoke-owner is not a smoke run's, or is one from before it.
stale() {
  local rel=${1#"$scratch"/} root pid args
  [[ $rel != "$1" && $rel == tmp.* ]] || return 1
  root=$scratch/${rel%%/*}
  [[ -d $root ]] || return 0
  read -r pid 2>/dev/null <"$root/smoke-owner" || return 1
  args=$(tr '\0' ' ' 2>/dev/null <"/proc/$pid/cmdline") || return 0
  [[ $args != *smoke/* ]]
}

for dir in /proc/[0-9]*; do
  read -r comm 2>/dev/null <"$dir/comm" || continue
  case $comm in
    kata) var=KATA_HOME ;;
    docbank) var=DOCBANK_HOME ;;
    *) continue ;;
  esac
  args=$(tr '\0' ' ' 2>/dev/null <"$dir/cmdline") || continue
  [[ " $args" == *" daemon "* ]] || continue
  home=$(tr '\0' '\n' 2>/dev/null <"$dir/environ" | sed -n "s/^$var=//p") || continue
  [[ -n $home ]] || continue
  if [[ -n $folder ]]; then
    [[ $home == "$folder"/* ]] || continue
  else
    stale "$home" || continue
  fi
  kill "${dir#/proc/}" 2>/dev/null || true
done
exit 0
