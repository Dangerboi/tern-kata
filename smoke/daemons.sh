#!/usr/bin/env bash
# The kata and Docbank daemons smoke runs start, found by the KATA_HOME or DOCBANK_HOME in their environment: every home a
# run uses lies in its mktemp folder, whoever starts the daemon (the script, a step, the Pane, kata's own `daemon recover`).
#   own <folder>   records the calling script (pid and start time) as the folder's owner, in <folder>/smoke-owner
#   stop <folder>  stops every daemon whose home is in <folder>: a script's cleanup
#   sweep          stops daemons whose mktemp folder is gone or whose owner has exited. A run killed before its cleanup
#                  (SIGKILL from a command timeout, the OOM killer) leaves those running, and nothing else stops them.
#                  Daemons of live runs, and daemons whose home is in no mktemp folder (the user's own), are left alone.
# Usage: smoke/daemons.sh own <folder> | stop <folder> | sweep
set -euo pipefail

# start_of <pid>: the process's start time (field 22 of /proc/<pid>/stat), which a reused pid does not share.
start_of() {
  local stat
  read -r stat 2>/dev/null <"/proc/$1/stat" || return 1
  stat=${stat##*) }
  set -- $stat
  printf '%s' "${20}"
}

# folder_of <home>: the mktemp folder (the first tmp.* component of the path) that holds <home>.
folder_of() {
  local parts part path=
  IFS=/ read -ra parts <<<"${1#/}"
  for part in "${parts[@]}"; do
    path+=/$part
    if [[ $part == tmp.* ]]; then
      printf '%s' "$path"
      return 0
    fi
  done
  return 1
}

# stale <home>: the home's folder is gone, or its owner no longer runs. A folder without an owner is not a smoke run's.
stale() {
  local folder pid start
  folder=$(folder_of "$1") || return 1
  [[ -d $folder ]] || return 0
  read -r pid start 2>/dev/null <"$folder/smoke-owner" || return 1
  [[ -n ${start:-} ]] || return 1
  [[ $(start_of "$pid" || true) != "$start" ]]
}

# each_daemon <command...>: runs <command...> <pid> <home> for every kata and Docbank daemon that has a home. A process can exit
# between any two reads of /proc; a failed read skips it.
each_daemon() {
  local dir comm var args home
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
    "$@" "${dir#/proc/}" "$home"
  done
}

stop_in() { [[ $3 != "$1"/* ]] || kill "$2" 2>/dev/null || true; }
stop_stale() { ! stale "$2" || kill "$1" 2>/dev/null || true; }

usage() { printf 'Usage: smoke/daemons.sh own <folder> | stop <folder> | sweep\n' >&2; exit 2; }
[[ -d /proc ]] || exit 0
case ${1:-} in
  own | stop)
    # An empty folder would make stop match every home on the machine.
    folder=${2:-}
    folder=${folder%/}
    [[ -n $folder ]] || usage
    if [[ $1 == own ]]; then
      start=$(start_of "$PPID")
      printf '%s %s\n' "$PPID" "$start" >"$folder/smoke-owner"
    else
      each_daemon stop_in "$folder"
    fi
    ;;
  sweep) each_daemon stop_stale ;;
  *) usage ;;
esac
