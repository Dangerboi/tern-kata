# A parent's detail shows `Children: N open / M total` row in its properties and its children as a table below the
# body, above Activity: reference, status, title, owner and updated, the closed child muted. A child opens by click, and
# by j/enter from the keys in the detail; esc comes back. Own Project and Issues; it cleans up.
ch_dir="$tmp/children-workspace"
mkdir -p "$ch_dir" "$tmp/children-unbound"
kata --workspace "$ch_dir" init --project children --json >/dev/null
ch_project=$(kata --workspace "$ch_dir" projects list --json | jq -er '.projects[] | select(.name == "children") | .uid')
kata --workspace "$ch_dir" create "Children parent" --body "Children parent body" --json >"$tmp/children-parent.json"
ch_parent=$(jq -r '.issue.uid' "$tmp/children-parent.json")
# A row of the table is keyed by its Link's id, so the ids below are the Links' (found through each child's own Link).
ch_shorts=()
ch_links=()
for n in 1 2 3; do
  kata --workspace "$ch_dir" create "Children kid $n" --force-new --parent "$ch_parent" --json >"$tmp/children-kid.json"
  ch_shorts+=("$(jq -r '.issue.short_id' "$tmp/children-kid.json")")
  ch_links+=("$(kata --workspace "$ch_dir" show "${ch_shorts[-1]}" --json | jq -r '.links[] | select(.type == "parent") | .id')")
done
kata --workspace "$ch_dir" close "${ch_shorts[0]}" --done --message "Closed by the children smoke step so one row is muted" --commit abc1234def56 --json >/dev/null

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/children-unbound" "$KATA_HOME" "$tmp" >"$tmp/children-launch.ctl"
ctl --file "$tmp/children-launch.ctl"
ctl click "[data-id='main.columns.collections.back']"
expect_tree "[data-id='main.columns.collections.projects']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$ch_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$ch_project']"
expect_tree "[data-id^='main.columns.list.'][data-id\$='.$ch_parent']" '.ok and .count == 1'
ctl click "[data-id^='main.columns.list.'][data-id\$='.$ch_parent']"
ch_content="main.columns.detail.viewport.content"
expect_tree "[data-id='$ch_content.heading.title']" '.ok and (.nodes[0].text | contains("Children parent") and (contains("loading") | not))'

# The properties block counts the children in one line and lists none of them.
expect_tree "[data-id='$ch_content.properties.children.v']" '.ok and .count == 1 and (.nodes[0].text == "2 open / 3 total")'
expect_tree "[data-id^='$ch_content.properties.parent']" '.ok and .count == 0'

# The table sits below the body and above Activity: Tern's own table, a header row, then one row per child with its five cells.
ch_table="$ch_content.children.table"
ch_rows="[data-id='$ch_table'] .sf-tbl-body .sf-tbl-row"
ch_heads="[data-id='$ch_table'] .sf-tbl > .sf-tbl-row.head > .sf-tbl-h"
expect_tree "[data-id='$ch_content.children.heading']" '.ok and .count == 1 and (.nodes[0].text == "Children")'
expect_tree "$ch_heads" '.ok and ([.nodes[].text] == ["ticket", "status", "title", "owner", "updated"])'
expect_tree "$ch_rows" '.ok and .count == 3'
expect_tree "$ch_rows > .sf-tbl-c" '.ok and .count == 15'
expect_tree "$ch_rows > .sf-tbl-c:nth-child(5)" '.ok and .count == 3 and ([.nodes[].text | select(test(" ago$"))] | length == 3)'
ch_y() { ctl tree "[data-id='$1']" | jq -r ".nodes[0].rect[${2:-1}]"; }
ch_body=$(ch_y "$ch_content.body")
ch_heading=$(ch_y "$ch_content.children.heading")
ch_activity=$(ch_y "$ch_content.activity")
awk -v b="$ch_body" -v t="$ch_heading" -v a="$ch_activity" 'BEGIN { exit !(b < t && t < a) }' || { printf 'Children table is not between the body and Activity: body y=%s children y=%s activity y=%s\n' "$ch_body" "$ch_heading" "$ch_activity" >&2; return 1; }
# The header cells sit over their columns: each header starts at the x of its column's cells and is as wide.
for ch_col in 1 2 3 4 5; do
  expect_tree "$ch_heads:nth-child($ch_col), $ch_rows > .sf-tbl-c:nth-child($ch_col)" '.ok and .count == 4 and ([.nodes[].rect[0]] | unique | length == 1) and ([.nodes[].rect[2]] | unique | length == 1)'
done
# The closed child's status and title are muted, the open child's are not (the dot has no text).
expect_tree "$ch_rows:nth-child(1) .sf-t-muted" '.ok and ([.nodes[].text] | index("closed") != null and index("Children kid 1") != null)'
expect_tree "$ch_rows:nth-child(2) .sf-t-muted, $ch_rows:nth-child(3) .sf-t-muted" '.ok and ([.nodes[].text] | index("open") == null and index("Children kid 2") == null and index("Children kid 3") == null)'

# A click on a child row opens it; esc comes back to the parent.
ch_second=$(ctl tree "$ch_rows > .sf-tbl-c:nth-child(3)" | jq -r '.nodes[1].text')
ctl click "$ch_rows:nth-child(2) > .sf-tbl-c:nth-child(3)"
expect_tree "[data-id='$ch_content.heading.title']" ".ok and (.nodes[0].text | contains(\"$ch_second\"))"
ctl key esc
expect_tree "[data-id='$ch_content.heading.title']" '.ok and (.nodes[0].text | contains("Children parent"))'

# esc leaves the focus in the detail, so j walks the child rows from the first; enter opens the second.
ctl key j
ctl key enter
expect_tree "[data-id='$ch_content.heading.title']" ".ok and (.nodes[0].text | contains(\"$ch_second\"))"
ctl key esc
expect_tree "[data-id='$ch_content.heading.title']" '.ok and (.nodes[0].text | contains("Children parent"))'

quit_app
kata projects remove children --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
