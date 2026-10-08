ctl key enter
expect_tree "[data-id='main.columns.detail.sections.section2.body']" '.ok and .count == 1 and ([.nodes[] | .. | objects | .text? // empty] | join(" ") | contains("seeded body"))'
for section in 1 2 3 4 5; do
  expect_tree "[data-id='main.columns.detail.sections.section${section}.heading']" '.ok and .count == 1'
done
ctl key tab
ctl key shift+tab
ctl key L
expect_tree "[data-id='main.columns.detail']" '.ok and .count == 0'
ctl key L
expect_tree "[data-id='main.columns.detail.sections.section2.body']" '.ok and .count == 1'
ctl key enter
ctl key escape
