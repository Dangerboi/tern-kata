mkdir -p "$tmp/fallback-bin"
cat >"$tmp/fallback-bin/kata" <<'SH'
#!/bin/sh
printf '%s\n' "$$" "$@" >"$FALLBACK_RESULT"
SH
chmod +x "$tmp/fallback-bin/kata"
for mode in plain disabled tmux screen zellij; do
  overrides=()
  case "$mode" in
    disabled) overrides=(TERN_TSP=0) ;;
    tmux) overrides=(TMUX=smoke) ;;
    screen) overrides=(STY=smoke) ;;
    zellij) overrides=(ZELLIJ=smoke) ;;
  esac
  env -u TERN_TSP -u TMUX -u STY -u ZELLIJ "${overrides[@]}" PATH="$tmp/fallback-bin:$PATH" FALLBACK_RESULT="$tmp/fallback-result" "$tmp/tern-kata" --as "smoke actor" --project smoke &
  child=$!
  wait "$child"
  expected=$(printf '%s\n' "$child" tui --as "smoke actor" --project smoke)
  [[ $(cat "$tmp/fallback-result") == "$expected" ]]
done
