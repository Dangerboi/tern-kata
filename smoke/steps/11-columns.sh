# A header row labels the columns and a parent's row shows kids as open/total.
kata --workspace "$tmp/workspace" create "Smoke columns parent" --json >"$tmp/columns-parent.json"
columns_parent=$(jq -r '.issue.uid' "$tmp/columns-parent.json")
kata --workspace "$tmp/workspace" create "Smoke columns child" --parent "$columns_parent" --json >"$tmp/columns-child.json"
columns_child=$(jq -r '.issue.uid' "$tmp/columns-child.json")
kata --workspace "$tmp/workspace" create "Smoke columns closed child" --parent "$columns_parent" --json >"$tmp/columns-closed.json"
columns_closed=$(jq -r '.issue.uid' "$tmp/columns-closed.json")
kata --workspace "$tmp/workspace" close "$columns_closed" --done --message "Finished the columns fixture and verified it by hand" --commit abc1234def56 --json
expect_tree "[data-id='main.columns.list.header']" '.ok and .count == 1 and (.nodes[0].text | test("#\\s+prio\\s+status\\s+title\\s+kids\\s+(owner\\s+)?updated"))'
expect_tree "[data-id$='.$columns_parent']" '.count == 1 and (.nodes[0].text | contains("1/2") and contains("ago"))'

# The owner cell starts at the owner header's x on every row, whatever glyphs the names have: the fixed-width boxes
# depend on Tern's .sf-item-value span children, which a CSS-text test cannot check.
kata --workspace "$tmp/workspace" create "Smoke columns wide owner" --owner 田中太郎 --json >"$tmp/columns-wide.json"
columns_wide=$(jq -r '.issue.uid' "$tmp/columns-wide.json")
kata --workspace "$tmp/workspace" create "Smoke columns long owner" --owner columns-owner-long --json >"$tmp/columns-long.json"
columns_long=$(jq -r '.issue.uid' "$tmp/columns-long.json")
expect_tree "[data-id\$='.$columns_wide']" '.count == 1'
ctl key L
columns_owner_cells="[data-id^='main.columns.list'] .sf-item[data-role~='tern-kata.row'] .sf-item-value > span:nth-child(2)"
expect_tree "$columns_owner_cells" '.ok and .count >= 4 and ([.nodes[].rect[0] | floor] | unique | length) == 1'
ctl key L
kata --workspace "$tmp/workspace" delete "$columns_wide" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/columns-wide.json")" --json
kata --workspace "$tmp/workspace" delete "$columns_long" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/columns-long.json")" --json

kata --workspace "$tmp/workspace" delete "$columns_child" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/columns-child.json")" --json
kata --workspace "$tmp/workspace" delete "$columns_closed" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/columns-closed.json")" --json
kata --workspace "$tmp/workspace" delete "$columns_parent" --force --confirm "DELETE smoke#$(jq -r '.issue.short_id' "$tmp/columns-parent.json")" --json
expect_tree "[data-id$='.$columns_parent']" '.count == 0'
