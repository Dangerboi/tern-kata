ctl key enter
expect_tree "[data-id='main.columns.detail.viewport.content.section2.body']" '.ok and .count == 1 and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("seeded body"))'
for section in 1 3 4 5; do
  expect_tree "[data-id='main.columns.detail.viewport.content.section${section}.heading']" '.ok and .count == 1'
done
expect_tree "[data-id='main.columns.detail.viewport.content.heading.status']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.detail.viewport.content.section2.heading'],[data-id='main.columns.detail.navigation']" '.ok and .count == 0'
ctl key tab
ctl key shift+tab
ctl key L
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
ctl key L
expect_tree "[data-id='main.columns.detail.viewport.content.section2.body']" '.ok and .count == 1'
ctl key enter
ctl key escape
