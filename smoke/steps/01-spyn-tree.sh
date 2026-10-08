ctl plugins expect smoke
expect_tree .sf-item '.ok and .count == 3 and ([.nodes[] | .. | objects | .text? // empty | sub("^[a-z0-9]{4} "; "")] | sort) == ["Smoke first Issue", "Smoke second Issue", "Smoke third Issue"]'
expect_tree "[data-id^='main.issues.01']" '.ok and .count == 3'
expect_tree .sf-item '.nodes[0]["aria-selected"] == "true"'
ctl key j
expect_tree .sf-item '.nodes[1]["aria-selected"] == "true"'
ctl key G
expect_tree .sf-item '.nodes[2]["aria-selected"] == "true"'
ctl key k
expect_tree .sf-item '.nodes[1]["aria-selected"] == "true"'
ctl key g
expect_tree .sf-item '.nodes[0]["aria-selected"] == "true"'
