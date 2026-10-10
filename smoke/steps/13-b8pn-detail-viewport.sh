# The detail is one scroll container: with the detail focused the wheel, ↓ and pgdn reach the end of a
# long body, ↑ and pgup come back, the top bar never moves, a key move in the detail leaves the wheel
# working (Tern pins a reveal's target, so the detail reveals only for a tab switch, 19-yd2n), and a different Issue opens at its
# top. Own Project and Issues; it cleans up.
vp_rects() { ctl tree "[data-id='main.columns.detail.viewport'],[data-id='main.columns.detail.viewport.content']" | jq -c '[.nodes[].rect]'; }
# How far the content is scrolled and how much is left below the viewport's bottom edge, in px.
vp_above() { vp_rects | jq -r '.[1][1] - .[0][1] | -floor'; }
vp_below() { vp_rects | jq -r '(.[1][1] + .[1][3]) - (.[0][1] + .[0][3]) | floor'; }
vp_bar() { ctl tree "[data-id='main.topbar']" | jq -c '.nodes[0].rect'; }
vp_wait() { # <label> <jq test on above/below as $a/$b>
  local a b
  for ((attempt = 0; attempt < 40; attempt++)); do
    a=$(vp_above); b=$(vp_below)
    jq -en --argjson a "$a" --argjson b "$b" "$2" >/dev/null && return 0
    sleep 0.1
  done
  printf '%s: scrolled %s px, %s px left below\n' "$1" "$a" "$b" >&2
  return 1
}
vp_wheel() { ctl hover "[data-id='main.columns.detail.viewport']" >/dev/null; for _ in $(seq "$2"); do ctl wheel 0 "$1" >/dev/null; done; }
vp_keys() { for _ in $(seq "$2"); do ctl key "$1" >/dev/null; done; }

vp_dir="$tmp/viewport-workspace"
mkdir -p "$vp_dir" "$tmp/viewport-unbound"
kata --workspace "$vp_dir" init --project viewport-detail --json >/dev/null
vp_project=$(kata --workspace "$vp_dir" projects list --json | jq -er '.projects[] | select(.name == "viewport-detail") | .uid')
vp_body=$(for line in $(seq 40); do printf 'Body line %s\n\n' "$line"; done)
kata --workspace "$vp_dir" create "Viewport long" --body "$vp_body" --json >"$tmp/viewport-long.json"
kata --workspace "$vp_dir" create "Viewport other" --body "$vp_body" --json >"$tmp/viewport-other.json"
vp_long=$(jq -r '.issue.uid' "$tmp/viewport-long.json")
vp_other=$(jq -r '.issue.uid' "$tmp/viewport-other.json")
kata --workspace "$vp_dir" comment "$(jq -r '.issue.short_id' "$tmp/viewport-long.json")" --body "Viewport comment" --json >/dev/null

quit_app
rm -f "$tmp/viewport-frames.jsonl"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata)"\n' "$tmp/viewport-unbound" "$tmp/viewport-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/viewport-launch.ctl"
ctl --file "$tmp/viewport-launch.ctl"
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$vp_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$vp_project']"
expect_tree "[data-id\$='.$vp_long']" '.ok and .count == 1'
ctl click "[data-id\$='.$vp_long']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Viewport long"))'
vp_bar_rect=$(vp_bar)
ctl key enter

# The one container holds the title and the whole body: it starts at its top and is taller than it shows.
vp_wait "the detail starts at its top" '$a == 0 and $b > 300'

# Wheel to the end of the body (the last line is reachable), then back.
vp_wheel 300 12
vp_wait "the wheel reaches the end of the body" '$b <= 1'
[[ $(vp_bar) == "$vp_bar_rect" ]] || { printf 'The top bar moved while the wheel scrolled the detail\n' >&2; return 1; }
vp_wheel -300 12
vp_wait "the wheel returns to the top" '$a == 0'

# ↓ scrolls by a line, pgdn by a page up to the end, ↑ and pgup come back.
vp_keys Down 3
vp_wait "↓ scrolls by a line" '$a > 0 and $a < 120'
vp_keys PageDown 12
vp_wait "pgdn reaches the end of the body" '$b <= 1'
[[ $(vp_bar) == "$vp_bar_rect" ]] || { printf 'The top bar moved while pgdn scrolled the detail\n' >&2; return 1; }
vp_keys Up 3
vp_wait "↑ scrolls up by a line" '$b > 20'
vp_keys PageUp 12
vp_wait "pgup returns to the top" '$a == 0'

# Key moves in the detail (tab, j) leave the wheel working: only a tab switch reveals (the new tab, see 19-yd2n), and the wheel works after it.
vp_keys Tab 3
vp_keys j 2
vp_keys Tab 3
vp_wheel 300 12
vp_wait "the wheel after tab and j" '$b <= 1'
vp_wheel -300 12
vp_wait "the wheel back up after tab and j" '$a == 0'
jq -sc '[.[] | select(.dir == "out" and .verb == "f") | .body.ops[] | select(.[0] == "reveal" and (.[1] | startswith("main.columns.detail")))] as $r | range(0; $r | length) as $i | select(([$r[$i][1], ($r[$i + 1][1] // "")] | any(endswith(".activity.tabs"))) | not) | $r[$i]' "$tmp/viewport-frames.jsonl" >"$tmp/viewport-reveals.json"
[[ ! -s $tmp/viewport-reveals.json ]] || { printf 'Reveal inside the detail for a key: %s\n' "$(<"$tmp/viewport-reveals.json")" >&2; return 1; }

# A different Issue opens at its top.
vp_wheel 300 12
vp_wait "scrolled to the end before another Issue opens" '$b <= 1'
ctl click "[data-id\$='.$vp_other']"
expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" '.ok and (.nodes[0].text | contains("Viewport other"))'
vp_wait "the other Issue opens at its top" '$a == 0'

quit_app
kata projects remove viewport-detail --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
