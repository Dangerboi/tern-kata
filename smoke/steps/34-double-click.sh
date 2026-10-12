# A double click on an Issue row opens it as selecting the row and pressing enter does: side by side the keys
# move to the detail, in the single-column tier the detail replaces the list, on a leaf, a parent (its fold
# ends where it began) and a nested child, and esc backs out as after enter. A single click keeps its meaning.
# Tern sends a double click as two single clicks then the row's dblclick action, checked from the
# TERN_TSP_RECORD trace. Own Project and Issues; it cleans up.
dbl_title() { expect_tree "[data-id='main.columns.detail.viewport.content.heading.title']" ".ok and (.nodes[0].text | contains(\"$1\"))"; }
dbl_selected() { expect_tree "[data-id\$='.$1']" '.ok and .nodes[0]["aria-selected"] == "true"'; }
# The input events since the line count in $1, as Tern delivered them.
dbl_events() {
  tail -n "+$(($1 + 1))" "$tmp/dbl-frames.jsonl" | jq -r 'select(.dir == "in" and .verb == "e") | .body | if .ev == "action" then .act else .ev end | select(. != "ack")' | paste -sd' ' -
}
# Double clicks the row $1 and requires the events Tern sent to be $2.
dbl_double_click() {
  local mark
  mark=$(wc -l <"$tmp/dbl-frames.jsonl")
  ctl dblclick "[data-id\$='.$1']"
  for ((attempt = 0; attempt < 100; attempt++)); do
    [[ $(dbl_events "$mark") != "$2" ]] || return 0
    sleep 0.1
  done
  printf 'Tern sent "%s" for a double click, want "%s"\n' "$(dbl_events "$mark")" "$2" >&2
  return 1
}

dbl_dir="$tmp/dblclick-workspace"
mkdir -p "$dbl_dir"
kata --workspace "$dbl_dir" init --project dblclick --json >/dev/null
kata --workspace "$dbl_dir" create "Dbl parent" --json >"$tmp/dbl-parent.json"
dbl_parent=$(jq -r '.issue.uid' "$tmp/dbl-parent.json")
kata --workspace "$dbl_dir" create "Dbl child" --parent "$dbl_parent" --json >"$tmp/dbl-child.json"
dbl_child=$(jq -r '.issue.uid' "$tmp/dbl-child.json")
kata --workspace "$dbl_dir" create "Dbl leaf" --json >"$tmp/dbl-leaf.json"
dbl_leaf=$(jq -r '.issue.uid' "$tmp/dbl-leaf.json")
read -r dbl_full dbl_height < <(ctl stats | jq -r '.viewport[0:2] | @tsv')

quit_app
rm -f "$tmp/dbl-frames.jsonl"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP TERN_TSP_RECORD=%s KATA_HOME=%s %s/tern-kata)"\n' "$dbl_dir" "$tmp/dbl-frames.jsonl" "$KATA_HOME" "$tmp" >"$tmp/dbl-launch.ctl"
ctl --file "$tmp/dbl-launch.ctl"
ctl resize 1200 "$dbl_height"
expect_tree "[data-id\$='.$dbl_leaf']" '.ok and .count == 1'
expect_tree "[data-id\$='.$dbl_parent']" '.ok and .count == 1'
expect_tree "[data-id\$='.$dbl_child']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'
(( $(ctl state | jq -r '.panes[0].cols') >= 140 ))
hint_expect_mode list

# A single click keeps its meaning: it selects the leaf and the keys stay in the list.
ctl click "[data-id\$='.$dbl_leaf']"
dbl_selected "$dbl_leaf"
dbl_title "Dbl leaf"
hint_expect_mode list

# Side by side, a double click on the collapsed parent opens it and leaves it collapsed. Tern's two single
# clicks fold and unfold it first, so the children are never left shown.
dbl_double_click "$dbl_parent" "toggle-expansion toggle-expansion open-row"
hint_expect_mode detail
dbl_selected "$dbl_parent"
dbl_title "Dbl parent"
expect_tree "[data-id\$='.$dbl_child']" '.ok and .count == 0'
ctl key esc
hint_expect_mode list
dbl_selected "$dbl_parent"

# A leaf opens the same way: its two single clicks select, then the keys move to the detail.
dbl_double_click "$dbl_leaf" "select select open-row"
hint_expect_mode detail
dbl_selected "$dbl_leaf"
dbl_title "Dbl leaf"
ctl key esc
hint_expect_mode list
dbl_selected "$dbl_leaf"

# A single click on the parent still folds it, and a double click on the expanded parent leaves it expanded.
ctl click "[data-id\$='.$dbl_parent']"
expect_tree "[data-id\$='.$dbl_child']" '.ok and .count == 1'
hint_expect_mode list
dbl_double_click "$dbl_child" "select select open-row"
hint_expect_mode detail
dbl_selected "$dbl_child"
dbl_title "Dbl child"
ctl key esc
hint_expect_mode list
dbl_double_click "$dbl_parent" "toggle-expansion toggle-expansion open-row"
hint_expect_mode detail
dbl_title "Dbl parent"
expect_tree "[data-id\$='.$dbl_child']" '.ok and .count == 1'
ctl key esc
hint_expect_mode list

# Double click from the detail: the keys were in the detail on another Issue and move to the double clicked one.
ctl key enter
hint_expect_mode detail
dbl_double_click "$dbl_leaf" "select select open-row"
hint_expect_mode detail
dbl_selected "$dbl_leaf"
dbl_title "Dbl leaf"
ctl key esc
hint_expect_mode list

# In the single-column tier the double click selects the row and the detail replaces the list.
ctl resize 1000 "$dbl_height"
(( $(ctl state | jq -r '.panes[0].cols') < 140 ))
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
dbl_double_click "$dbl_parent" "toggle-expansion toggle-expansion open-row"
expect_tree "[data-id='main.columns.list']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'
dbl_title "Dbl parent"
hint_expect_mode detail
ctl key esc
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
expect_tree "[data-id\$='.$dbl_child']" '.ok and .count == 1'
dbl_double_click "$dbl_leaf" "select select open-row"
expect_tree "[data-id='main.columns.list']" '.ok and .count == 0'
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'
dbl_title "Dbl leaf"
ctl key esc
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
dbl_selected "$dbl_leaf"
dbl_double_click "$dbl_child" "select select open-row"
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 1'
dbl_title "Dbl child"
ctl key esc
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'

# Tern never reported a surface error for the new action.
dbl_errors=$(jq -c 'select(.dir == "in" and .verb == "e" and .body.ev == "error")' "$tmp/dbl-frames.jsonl")
[[ -z $dbl_errors ]] || { printf 'Tern sent surface errors:\n%s\n' "$dbl_errors" >&2; return 1; }

quit_app
kata projects remove dblclick --force --json >/dev/null
ctl resize "$dbl_full" "$dbl_height"
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
