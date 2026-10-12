kata --workspace "$tmp/workspace" create "Smoke hierarchy parent" --priority 0 --owner smoke-owner --label smoke-label --json >"$tmp/hierarchy-parent.json"
hierarchy_parent=$(jq -r '.issue.uid' "$tmp/hierarchy-parent.json")
hierarchy_short=$(jq -r '.issue.short_id' "$tmp/hierarchy-parent.json")
kata --workspace "$tmp/workspace" create "Smoke hierarchy child" --parent "$hierarchy_parent" --json >"$tmp/hierarchy-child.json"
hierarchy_child=$(jq -r '.issue.uid' "$tmp/hierarchy-child.json")
kata --workspace "$tmp/workspace" create "Smoke hierarchy grandchild" --parent "$hierarchy_child" --json >"$tmp/hierarchy-grandchild.json"
hierarchy_grandchild=$(jq -r '.issue.uid' "$tmp/hierarchy-grandchild.json")
ctl key q
expect_tree .sf-list '.count == 0'
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id$='.$hierarchy_parent']" '.count == 1'
expect_tree "[data-id$='.$hierarchy_child']" '.count == 0'
expect_tree "[data-id$='.$hierarchy_parent']" "([.nodes[0] | .. | objects | .text? // empty] | join(\" \")) as \$text | (\$text | contains(\"$hierarchy_short\") and contains(\"Smoke hierarchy parent\") and contains(\"open\") and contains(\"P0\") and contains(\"1/1\") and (contains(\"smoke-owner\") | not) and (contains(\"smoke-label\") | not))"
ctl key L
expect_tree "[data-id$='.$hierarchy_parent']" '.count == 1 and (.nodes[0].text | contains("smoke-o…"))'
ctl key L
expect_tree "[data-id$='.$hierarchy_parent']" '.count == 1 and (.nodes[0].text | contains("smoke-o…") | not)'
ctl click "[data-id$='.$hierarchy_parent']"
ctl key Right
expect_tree "[data-id$='.$hierarchy_child']" '.count == 1 and (.nodes[0].text | startswith("└─"))'
expect_tree "[data-id$='.$hierarchy_grandchild']" '.count == 0'
ctl key E
expect_tree "[data-id$='.$hierarchy_grandchild']" '.count == 1 and (.nodes[0].text | startswith("└─"))'
ctl key E
expect_tree "[data-id$='.$hierarchy_child']" '.count == 0'
ctl key v
expect_tree "[data-id$='.$hierarchy_grandchild']" '.count == 1 and (.nodes[0].text | test("[├└]") | not)'
ctl key v
expect_tree "[data-id$='.$hierarchy_child']" '.count == 0'
