ctl key enter
expect_tree "[data-id='main.columns.detail.viewport.content.body']" '.ok and .count == 1 and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("seeded body"))'
for tab in 0 1 2; do
  expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab${tab}']" '.ok and .count == 1'
done
expect_tree "[data-id='main.columns.detail.viewport.content.activity.tabs.tab0']" '.nodes[0].text | startswith("[ Comments (")'
expect_tree "[data-id^='main.columns.detail.viewport.content.properties.'][data-id$='.k']" '.ok and ([.nodes[].text] | (.[0:3] == ["Author:", "Created:", "Updated:"]) and all(endswith(":")) and (index("Revision:") == null) and (index("Project:") == null))'
expect_tree "[data-id='main.columns.detail.viewport.content.heading.status']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.detail.viewport.content.body.heading'],[data-id='main.columns.detail.navigation']" '.ok and .count == 0'
ctl key tab
ctl key shift+tab
ctl key L
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
ctl key L
expect_tree "[data-id='main.columns.detail.viewport.content.body']" '.ok and .count == 1'
ctl key enter
ctl key escape
