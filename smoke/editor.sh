#!/usr/bin/env sh
set -eu
[ -t 0 ]
printf 'Draft editor ready: type the appended text\n'
IFS= read -r text
printf '%s' "$text" >> "$1"
