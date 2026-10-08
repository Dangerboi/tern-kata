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

# Collections sidebar: Escape focuses it on the bound Project.
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
ctl key Enter
ctl plugins expect smoke
expect_tree "[data-id^='main.columns.list.issues.01']" '.ok and .count >= 4'

# Detail: arrows move between references, Left backs out like Escape.
jump() {
  ctl key Tab
  ctl key Tab
  ctl key Tab
  for key in "$@"; do ctl key "$key"; done
  ctl key Enter
}
expect_detail() {
  expect_tree "[data-id='main.columns.detail']" "[.nodes[] | .. | objects | .text? // empty] | join(\" \") | contains(\"Smoke arrows $1 child\") and (contains(\"Smoke arrows $2 child\") | not)"
}
back_to_parent() {
  ctl key Left
  expect_tree "[data-id='main.columns.detail']" '[.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Smoke arrows parent")'
}
ctl click "[data-id$='.$arrows_parent']"
ctl key Enter
jump
expect_tree "[data-id='main.columns.detail']" '[.nodes[] | .. | objects | .text? // empty] | join(" ") | contains(" child")'
ctl tree "[data-id='main.columns.detail']" >"$tmp/arrows-detail.json"
if jq -e '[.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("Smoke arrows first child")' "$tmp/arrows-detail.json" >/dev/null; then
  ref_a=first ref_b=second
else
  ref_a=second ref_b=first
fi
expect_detail "$ref_a" "$ref_b"
back_to_parent
jump Down
expect_detail "$ref_b" "$ref_a"
back_to_parent
jump Up
expect_detail "$ref_a" "$ref_b"
back_to_parent
jump End
expect_detail "$ref_b" "$ref_a"
back_to_parent
jump Home
expect_detail "$ref_a" "$ref_b"
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
