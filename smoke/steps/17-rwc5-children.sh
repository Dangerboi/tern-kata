# A parent's detail shows `Children: N open / M total` row in its properties and its children as a table below the
# body, above Activity: reference, status, title and updated, the closed child muted. A child opens by click, and
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
expect_tree "[data-id='main.columns.collections.project.$ch_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$ch_project']"
expect_tree "[data-id^='main.columns.list.'][data-id\$='.$ch_parent']" '.ok and .count == 1'
ctl click "[data-id^='main.columns.list.'][data-id\$='.$ch_parent']"
ch_content="main.columns.detail.viewport.content"
expect_tree "[data-id='$ch_content.heading.title']" '.ok and (.nodes[0].text | contains("Children parent") and (contains("loading") | not))'

# The properties block counts the children in one line and lists none of them.
expect_tree "[data-id='$ch_content.properties.children.v']" '.ok and .count == 1 and (.nodes[0].text == "2 open / 3 total")'
expect_tree "[data-id^='$ch_content.properties.parent']" '.ok and .count == 0'

# The table sits below the body and above Activity, one row per child with its four cells.
expect_tree "[data-id='$ch_content.children.heading']" '.ok and .count == 1 and (.nodes[0].text == "Children")'
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.jump']" '.ok and .count == 3'
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.status']" '.ok and .count == 3'
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.title']" '.ok and .count == 3'
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.updated']" '.ok and .count == 3 and ([.nodes[].text | select(test(" ago$"))] | length == 3)'
ch_y() { ctl tree "[data-id='$1']" | jq -r ".nodes[0].rect[${2:-1}]"; }
ch_body=$(ch_y "$ch_content.body")
ch_table=$(ch_y "$ch_content.children")
ch_activity=$(ch_y "$ch_content.activity")
awk -v b="$ch_body" -v t="$ch_table" -v a="$ch_activity" 'BEGIN { exit !(b < t && t < a) }' || { printf 'Children table is not between the body and Activity: body y=%s table y=%s activity y=%s\n' "$ch_body" "$ch_table" "$ch_activity" >&2; return 1; }
# The cells line up: every row's status cell starts at the same x.
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.status']" '.ok and ([.nodes[].rect[0]] | unique | length == 1)'
# The reference and status cells are one line high at this width: as high as the one-line Children heading.
ch_line=$(ch_y "$ch_content.children.heading" 3)
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.jump'], [data-id^='$ch_content.children.'][data-id\$='.status']" ".ok and .count == 6 and ([.nodes[].rect[3]] | all(. == $ch_line))"
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.jump']" '.ok and ([.nodes[].rect[0]] | unique | length == 1) and ([.nodes[].rect[2]] | unique | length == 1)'
expect_tree "[data-id^='$ch_content.children.'][data-id\$='.updated']" '.ok and ([.nodes[].rect[0]] | unique | length == 1) and ([.nodes[].rect[2]] | unique | length == 1)'
# The closed child's status and title are muted, the open child's are not.
expect_tree "[data-id='$ch_content.children.${ch_links[0]}'] *" '.ok and ([.nodes[] | select(.class | contains("sf-t-muted")) | .text] | index("closed") != null and index("Children kid 1") != null)'
expect_tree "[data-id='$ch_content.children.${ch_links[1]}'] *" '.ok and ([.nodes[] | select(.class | contains("sf-t-muted")) | .text] | index("open") == null and index("Children kid 2") == null)'

# A click on a child row opens it; esc comes back to the parent.
ctl click "[data-id='$ch_content.children.${ch_links[1]}.title']"
expect_tree "[data-id='$ch_content.heading.title']" '.ok and (.nodes[0].text | contains("Children kid 2"))'
ctl key esc
expect_tree "[data-id='$ch_content.heading.title']" '.ok and (.nodes[0].text | contains("Children parent"))'

# esc leaves the focus in the detail, so j walks the child rows from the first; enter opens the second.
ch_second=$(ctl tree "[data-id^='$ch_content.children.'][data-id\$='.title']" | jq -r '.nodes[1].text')
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
