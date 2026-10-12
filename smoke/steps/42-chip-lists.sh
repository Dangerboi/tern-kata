# No single-line input draws Tern's mode chip (the plugin's style sheet hides it) while typing still fills the field,
# and an ordered list in an Issue body draws every marker, two-digit ones included, inside the content's left edge.
kata --workspace "$tmp/workspace" create "Smoke ordered list" --body $'1. first\n2. second\n3. third\n4. fourth\n5. fifth\n6. sixth\n7. seventh\n8. eighth\n9. ninth\n10. tenth' --json >"$tmp/ordered.json"
ordered_uid=$(jq -r '.issue.uid' "$tmp/ordered.json")
ordered_short=$(jq -r '.issue.short_id' "$tmp/ordered.json")
expect_tree "[data-id\$='.$ordered_uid']" '.ok and .count == 1'
ctl click "[data-id\$='.$ordered_uid']"
expect_tree ".md li" '.ok and .count >= 10'
# A marker hangs left of its item by about its width; the list's own start padding must hold "10." inside the content.
expect_tree ".md ol" '.ok and .count == 1 and .nodes[0].rect[0] > 0'
ctl tree ".md ol li" >"$tmp/ordered-li.json"
ctl tree ".md ol" >"$tmp/ordered-ol.json"
jq -e --slurpfile ol "$tmp/ordered-ol.json" '.nodes[0].rect[0] - $ol[0].nodes[0].rect[0] >= 30' "$tmp/ordered-li.json" >/dev/null

ctl key n
expect_tree "[data-id='layer.create.title']" '.count == 1'
ctl type '"Chip title"'
expect_tree "[data-id='layer.create.title']" '.nodes[0].text | contains("Chip title")'
expect_tree ".sf-ed-mode" '.ok and .count == 0'
ctl key esc
ctl key y
expect_tree "[data-id='layer.create']" '.count == 0'

ctl click "[data-id='main.topbar.right.menu']"
expect_tree "[data-id='layer.collections']" '.ok and .count == 1'
ctl type '"inb"'
expect_tree "[data-id='layer.collections.filter']" '.nodes[0].text | contains("inb")'
expect_tree ".sf-ed-mode" '.ok and .count == 0'
ctl key esc
expect_tree "[data-id='layer.collections']" '.count == 0'

ctl key /
ctl type '"zzz"'
expect_tree "[data-id='main.columns.list.search']" '.nodes[0].text | contains("zzz")'
expect_tree ".sf-ed-mode" '.ok and .count == 0'
ctl key esc

kata --workspace "$tmp/workspace" delete "$ordered_uid" --force --confirm "DELETE smoke#$ordered_short" --json
