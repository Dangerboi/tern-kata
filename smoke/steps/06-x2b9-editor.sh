kata --workspace "$tmp/workspace" create "Smoke editor Issue" --body "Editor seeded body" --priority 0 --json >"$tmp/editor-issue.json"
editor_uid=$(jq -r '.issue.uid' "$tmp/editor-issue.json")
expect_tree "[data-id='main.columns.list.issues.$editor_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$editor_uid']"
ctl key enter
ctl plugins expect '"Editor seeded body"'
ctl key e
expect_tree "[data-id='layer.editor.body']" '.ok and .count == 1'
ctl key End
ctl type '" + native Draft"'
expect_tree "[data-id='layer.editor.body']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("native Draft"))'
ctl key Ctrl+o
ctl plugins expect '"Editor seeded body + native Draft"'
expect_tree "[data-id='layer.editor']" '.ok and .count == 0'

ctl key c
expect_tree "[data-id='layer.editor.body']" '.ok and .count == 1'
ctl type '"Smoke native comment"'
expect_tree "[data-id='layer.editor.body']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("Smoke native comment"))'
ctl key Ctrl+o
expect_tree "[data-id='layer.editor']" '.ok and .count == 0'
ctl click "[data-id='main.columns.detail.section4.heading']"
ctl plugins expect '"Smoke native comment"'

# A real subprocess requires ownership of the tty, not just a suspended surface.
ctl key e
expect_tree "[data-id='layer.editor.body']" '.ok and .count == 1'
ctl key Ctrl+e
ctl expect '"Draft editor ready"'
ctl type '" + external Draft"'
ctl key Enter
expect_tree "[data-id='layer.editor.body']" '.ok and .count == 1'
expect_tree "[data-id='layer.editor.body']" '[.nodes[] | .. | objects | .text? // empty] | any(contains("external Draft"))'
ctl key Ctrl+o
ctl plugins expect '"Editor seeded body + native Draft + external Draft"'
ctl key Escape
