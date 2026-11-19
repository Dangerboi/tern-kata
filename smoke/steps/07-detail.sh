ctl key enter
expect_tree "[data-id^='main.columns.detail.viewport.content.body']" '.ok and .count == 1 and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("seeded body"))'
for tab in 0 1 2; do
  expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab${tab}']" '.ok and .count == 1'
done
expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']" '.nodes[0].text | test("^Comments( [0-9]+)?$")'
expect_tree "[data-id^='main.columns.detail.viewport.content.properties.facts.'][data-id$='.k']" '.ok and ([.nodes[].text] | (.[0:4] == ["Status", "Priority", "Owner", "Labels"]) and all(endswith(":") | not) and (index("Revision") == null) and (index("Project") == null) and (index("Author") == null))'
# Every value of the left column starts at the same x, in the sans face as in mono.
expect_tree "[data-id^='main.columns.detail.viewport.content.properties.facts.left.'][data-id\$='.v']" '.ok and .count >= 1 and ([.nodes[].rect[0]] | unique | length == 1)'
# The key legend keeps its keys in key spans and its hints in one wrapping row.
expect_tree "[data-id^='dock.hints.grid.']" '.ok and .count > 3 and all(.nodes[]; (.text | test("\u00a0|▕") | not))'
expect_tree "[data-id='main.columns.detail.viewport.content.heading.status']" '.ok and .count == 1'
expect_tree "[data-id^='main.columns.detail.viewport.content.body'][data-id\$='.heading'],[data-id='main.columns.detail.navigation']" '.ok and .count == 0'
ctl key tab
ctl key shift+tab
ctl key L
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
ctl key L
expect_tree "[data-id^='main.columns.detail.viewport.content.body']" '.ok and .count == 1'
ctl key enter
ctl key escape
