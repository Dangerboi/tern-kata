arrows_parent=$(kata --workspace "$tmp/workspace" create "Smoke arrows parent" --json | jq -r '.issue.uid')
arrows_refs=()
for child in first second; do
  arrows_refs+=("smoke#$(kata --workspace "$tmp/workspace" create "Smoke arrows $child child" --parent "$arrows_parent" --json | jq -r '.issue.short_id')")
done
ctl key q
expect_tree .sf-list '.count == 0'
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 4'
ctl key Home
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[0]["aria-selected"] == "true"'
ctl key Down
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[1]["aria-selected"] == "true"'
ctl key End
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[-1]["aria-selected"] == "true"'
ctl key Up
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[-2]["aria-selected"] == "true"'
ctl key Home
expect_tree "[data-id^='main.columns.list.issues.01']" '.nodes[0]["aria-selected"] == "true"'

# Collections sidebar: an unbound directory keeps it, and Escape focuses it.
ctl key q
expect_tree .sf-list '.count == 0'
mkdir -p "$tmp/arrows-unbound"
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/arrows-unbound" "$KATA_HOME" "$tmp" >"$tmp/arrows-unbound.ctl"
ctl --file "$tmp/arrows-unbound.ctl"
expect_tree "[data-id^='main.columns.collections.']" '.ok and .count >= 7'
ctl key Escape
ctl key Home
expect_tree "[data-id^='main.columns.collections.']" '.nodes[0]["aria-selected"] == "true"'
ctl key Down
expect_tree "[data-id^='main.columns.collections.']" '.nodes[1]["aria-selected"] == "true"'
ctl key End
expect_tree "[data-id^='main.columns.collections.']" '.nodes[-1]["aria-selected"] == "true"'
ctl key Up
expect_tree "[data-id^='main.columns.collections.']" '.nodes[-2]["aria-selected"] == "true"'
ctl key End
ctl key Up
ctl key Enter
ctl key End
ctl key Enter
expect_tree "[data-id='main.topbar.heading']" '.ok and .nodes[0].text == "Project: smoke"'
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 4'
ctl key q
expect_tree .sf-list '.count == 0'
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 4'

# Detail: j/k move between the child rows and then the Links rows (the arrows scroll the detail), Left backs out like Escape.
# A tab click puts the cursor on the tab's first row, so k from there climbs into the Children table's last row and Home reaches its first.
jump() {
  expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']" '.ok and .count == 1'
  ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab2']"
  for key in "$@"; do ctl key "$key"; done
  ctl key Enter
}
# The title names the shown Issue. The card's own text is capped at 200 characters by Tern's tree, and the parent's first
# child row now falls inside that window, so it cannot tell a parent from a child; a child's detail also names its
# parent, and the tab click below needs the parent's own detail loaded to know its child rows.
detail_title="[data-id='main.columns.detail.viewport.content.heading.title']"
expect_detail() {
  expect_tree "$detail_title" ".ok and (.nodes[0].text | contains(\"Smoke arrows $1 child\") and (contains(\"Smoke arrows $2 child\") | not))"
}
back_to_parent() {
  ctl key Left
  expect_tree "$detail_title" '.ok and (.nodes[0].text | contains("Smoke arrows parent"))'
}
ctl click "[data-id$='.$arrows_parent']"
ctl key Enter
jump
expect_tree "$detail_title" '.ok and (.nodes[0].text | contains(" child"))'
if jq -e '.nodes[0].text | contains("Smoke arrows first child")' "$tmp/assert-tree.json" >/dev/null; then
  ref_a=first ref_b=second
else
  ref_a=second ref_b=first
fi
expect_detail "$ref_a" "$ref_b"
back_to_parent
expect_tree "[data-id^='main.columns.detail.viewport.content.children.'][data-id\$='.title']" '.ok and .count == 2'
cp "$tmp/assert-tree.json" "$tmp/arrows-children.json"
if jq -e '.nodes[0].text | contains("first child")' "$tmp/arrows-children.json" >/dev/null; then
  table_first=first table_last=second
else
  table_first=second table_last=first
fi
jump j
expect_detail "$ref_b" "$ref_a"
back_to_parent
jump k
expect_detail "$table_last" "$table_first"
back_to_parent
jump End
expect_detail "$ref_b" "$ref_a"
back_to_parent
jump Home
expect_detail "$table_first" "$table_last"
back_to_parent
ctl key Left
ctl key Down
expect_tree "[data-id^='main.columns.list.issues.01']" '[.nodes[] | select(.["aria-selected"] == "true") | .text] | length == 1 and (.[0] | contains("Smoke arrows") and contains("child"))'
ctl key Home

# Later steps count the seeded Issues, so remove this step's fixtures.
for ref in "${arrows_refs[@]}" "$arrows_parent"; do
  [[ $ref == smoke#* ]] || ref="smoke#$(kata --workspace "$tmp/workspace" show "$ref" --json | jq -r '.issue.short_id')"
  kata --workspace "$tmp/workspace" delete "$ref" --force --confirm "DELETE $ref"
done
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count == 3'
