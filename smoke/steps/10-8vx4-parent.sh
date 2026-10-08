# Selecting a parent Issue never blanks the Pane. The detail of a parent (one
# Link row per child) is far taller than the Pane, so the list must stay at the
# top beside it, in view, whether the parent is reached by j/k, by → then a
# child and back, or by a click. The app records every TSP op it sends and Tern
# every event it returns (TERN_TSP_RECORD), so a plugin that ended on a surface
# error would show there too.
kata --workspace "$tmp/workspace" create "Smoke parent spec" --body "Parent with many children" --json >"$tmp/parent-spec.json"
parent_uid=$(jq -r '.issue.uid' "$tmp/parent-spec.json")
parent_children=()
parent_shorts=("$(jq -r '.issue.short_id' "$tmp/parent-spec.json")")
for n in $(seq 1 40); do
  kata --workspace "$tmp/workspace" create "Smoke parent child $n" --force-new --parent "$parent_uid" --json >"$tmp/parent-child.json"
  parent_children+=("$(jq -r '.issue.uid' "$tmp/parent-child.json")")
  parent_shorts+=("$(jq -r '.issue.short_id' "$tmp/parent-child.json")")
done

quit_app
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata"\n' "$tmp/parent-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/parent-launch.ctl"
ctl --file "$tmp/parent-launch.ctl"
expect_tree "[data-id\$='.$parent_uid']" '.ok and .count == 1'
expect_tree "[data-id\$='.${parent_children[0]}']" '.ok and .count == 0'

parent_height=$(ctl stats | jq -r '.viewport[1]')
# The parent's row is the selected one, inside the Pane, and its detail is drawn.
parent_expect_drawn() {
  expect_tree "[data-id\$='.$parent_uid']" ".ok and .count == 1 and .nodes[0][\"aria-selected\"] == \"true\" and .nodes[0].rect[1] >= 0 and (.nodes[0].rect[1] + .nodes[0].rect[3]) <= $parent_height"
  expect_tree "[data-id='main.columns.detail.title']" '.ok and (.nodes[0].text | contains("Smoke parent spec") and (contains("loading") | not))'
  expect_tree "[data-id^='main.columns.detail.sections.section3.'][data-id\$='.jump']" '.ok and .count == 40'
}

# j/k onto the collapsed parent: from row 1 down to it, or up from row 2 when it is the first.
parent_row=$(ctl tree "[data-id^='main.columns.list.issues.']" | jq 'first(.nodes | to_entries[] | select(.value.text | contains("Smoke parent spec")) | .key)')
ctl key Home
if ((parent_row == 0)); then
  ctl key j
  ctl key k
else
  for ((step = 0; step < parent_row; step++)); do ctl key j; done
fi
parent_expect_drawn

# → expands it, j moves to its first child, k goes back up to the expanded parent.
ctl key Right
expect_tree "[data-id\$='.${parent_children[0]}']" '.ok and .count == 1'
parent_expect_drawn
ctl key j
expect_tree "[data-id='main.columns.detail.title']" '.ok and (.nodes[0].text | contains("Smoke parent child 1") and (contains("loading") | not))'
expect_tree "[data-id\$='.${parent_children[0]}']" '.ok and .nodes[0]["aria-selected"] == "true" and .nodes[0].rect[1] >= 0 and (.nodes[0].rect[1] + .nodes[0].rect[3]) <= '"$parent_height"
ctl key k
parent_expect_drawn

# A click on the parent's row gives the same screen: it folds the children away and keeps the parent drawn.
ctl click "[data-id\$='.$parent_uid']"
expect_tree "[data-id\$='.${parent_children[0]}']" '.ok and .count == 0'
parent_expect_drawn
ctl click "[data-id\$='.$parent_uid']"
expect_tree "[data-id\$='.${parent_children[0]}']" '.ok and .count == 1'
parent_expect_drawn

# The top bar stays put with the tall detail focused, at the window's height and a short one:
# only the detail's sections list scrolls, never the Pane.
parent_bar() { ctl tree "[data-id='main.topbar']" | jq -c '.nodes[0].rect'; }
ctl key enter
for parent_pane_height in "$parent_height" 420; do
  ctl resize 1200 "$parent_pane_height"
  expect_tree "[data-id='main.topbar']" '.ok and .count == 1'
  parent_bar_before=$(parent_bar)
  for _ in $(seq 8); do ctl key tab; done
  for _ in $(seq 45); do ctl key j; done
  sleep 0.5
  [[ $(parent_bar) == "$parent_bar_before" ]] || { printf 'Top bar moved with the detail focused at height %s: %s -> %s\n' "$parent_pane_height" "$parent_bar_before" "$(parent_bar)" >&2; return 1; }
done
ctl resize 1200 "$parent_height"
ctl key esc

# The plugin never met a surface error, and the Pane is still its Pane.
expect_tree "[data-id^='main.']" '.ok and .count > 0'
parent_errors=$(jq -c 'select(.dir == "in" and .verb == "e" and .body.ev == "error")' "$tmp/parent-frames.jsonl")
[[ -z $parent_errors ]] || { printf 'Tern sent surface errors:\n%s\n' "$parent_errors" >&2; return 1; }

quit_app
parent_uids=("$parent_uid" "${parent_children[@]}")
for i in "${!parent_uids[@]}"; do
  kata --workspace "$tmp/workspace" delete "${parent_uids[$i]}" --force --confirm "DELETE smoke#${parent_shorts[$i]}" --json >/dev/null
done
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
