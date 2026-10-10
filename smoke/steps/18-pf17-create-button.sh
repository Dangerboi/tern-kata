# The Create card ends in buttons that click their keys: Cancel closes the empty card, the primary button creates the Issue.
ctl key n
expect_tree "[data-id='layer.create.title']" '.count == 1'
expect_tree "[data-id='layer.create.actions.save']" '.count == 1 and (.nodes[0].text == "Create Issue (ctrl+o)")'
expect_tree "[data-id='layer.create.actions.cancel']" '.count == 1 and (.nodes[0].text == "Cancel (esc)")'
ctl click "[data-id='layer.create.actions.cancel']"
expect_tree "[data-id='layer.create']" '.count == 0'
ctl key n
expect_tree "[data-id='layer.create.title']" '.count == 1'
ctl type '"Smoke button-created Issue"'
ctl click "[data-id='layer.create.actions.save']"
expect_tree "[data-id='layer.create']" '.count == 0'
expect_tree "[data-id^='main.columns.list.issues.']" '[.nodes[] | select(.["aria-selected"] == "true") | .. | objects | .text? // empty] | any(contains("Smoke button-created Issue"))'
kata --workspace "$tmp/workspace" list --json >"$tmp/create-button-fixtures.json"
uid=$(jq -er '.issues[] | select(.title == "Smoke button-created Issue") | .uid' "$tmp/create-button-fixtures.json")
short=$(jq -er '.issues[] | select(.title == "Smoke button-created Issue") | .short_id' "$tmp/create-button-fixtures.json")
kata --workspace "$tmp/workspace" delete "$uid" --force --confirm "DELETE smoke#$short" --json
expect_tree "[data-id\$='.$uid']" '.count == 0'
