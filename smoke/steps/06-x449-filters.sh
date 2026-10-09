kata --workspace "$tmp/workspace" create "Search target Alpha" --owner x449-owner --label x449-label --json >"$tmp/search-alpha.json"
search_alpha=$(jq -r '.issue.uid' "$tmp/search-alpha.json")
kata --workspace "$tmp/workspace" create "Search target Beta" --owner x449-owner --label x449-label --json >"$tmp/search-beta.json"
search_beta=$(jq -r '.issue.uid' "$tmp/search-beta.json")
kata --workspace "$tmp/workspace" create "Search target Gamma" --owner x449-owner --label x449-label --parent "$search_alpha" --json >"$tmp/search-gamma.json"
search_gamma=$(jq -r '.issue.uid' "$tmp/search-gamma.json")
expect_tree "[data-id$='.$search_beta']" '.count == 1'

ctl key /
for letter in S e a r c h space t a r g e t; do ctl key "$letter"; done
expect_tree "[data-id='main.filters']" '.nodes[0].text | contains("search=Search target")'
expect_tree "[data-id$='.$search_alpha']" '.count == 1 and .nodes[0]["aria-selected"] == "true"'
ctl key Down
expect_tree "[data-id$='.$search_beta']" '.count == 1 and .nodes[0]["aria-selected"] == "true"'
ctl key enter
expect_tree "[data-id='main.columns.list.search']" '.count == 0'

# Query Escape restores the committed query, not the temporary text.
ctl key /
ctl key z
expect_tree "[data-id='main.filters']" '.nodes[0].text | contains("search=Search targetz")'
ctl key escape
expect_tree "[data-id='main.filters']" '.nodes[0].text | endswith("search=Search target")'
expect_tree "[data-id$='.$search_beta']" '.count == 1 and .nodes[0]["aria-selected"] == "true"'

ctl key s
ctl plugins expect status=open
ctl key s
ctl plugins expect status=closed
expect_tree "[data-id='main.columns.list.issues']" '.nodes[0].text | contains("No Issues match these filters")'
expect_tree "[data-id^='main.columns.list.issues.01']" '.count == 0'
ctl key s
expect_tree "[data-id='main.filters']" '.nodes[0].text | contains("status=") | not'
expect_tree "[data-id$='.$search_alpha']" '.count == 1'

ctl key f
expect_tree "[data-id='layer.filters']" '.count == 1 and (.nodes[0].text | contains("Filter smoke"))'
ctl key Right
ctl key tab
ctl key Right
ctl key tab
for letter in x 4 4 9 - o w n e r; do ctl key "$letter"; done
ctl key tab
for letter in x 4 4 9 - l a b e l; do ctl key "$letter"; done
ctl key tab
ctl key Right
ctl click "[data-id='layer.filters.actions.apply']"
expect_tree "[data-id='layer.filters']" '.count == 0'
expect_tree "[data-id='main.filters']" '.nodes[0].text | contains("project=smoke") and contains("status=open") and contains("owner=x449-owner") and contains("label=x449-label") and contains("relationship=parent")'
expect_tree "[data-id$='.$search_gamma']" '.count == 1'
expect_tree "[data-id$='.$search_alpha']" '.count == 0'

# Every filter axis survives a real durable-event refresh.
kata --workspace "$tmp/workspace" create "Search target Delta" --owner x449-owner --label x449-label --parent "$search_alpha" --json >"$tmp/search-delta.json"
search_delta=$(jq -r '.issue.uid' "$tmp/search-delta.json")
expect_tree "[data-id$='.$search_delta']" '.count == 1'
expect_tree "[data-id='main.filters']" '.nodes[0].text | contains("owner=x449-owner") and contains("label=x449-label") and contains("relationship=parent") and contains("search=Search target")'

ctl key c
expect_tree "[data-id='main.topbar.heading']" '.nodes[0].text == "Project: smoke"'
expect_tree "[data-id='main.filters']" '.ok and .count == 0'
expect_tree "[data-id$='.$hierarchy_parent']" '.count == 1'
ctl key f
expect_tree "[data-id='layer.filters']" '.count == 1 and (.nodes[0].text | contains("Filter smoke"))'
ctl click "[data-id='layer.filters.actions.cancel']"
expect_tree "[data-id='layer.filters']" '.count == 0'

# Keep later slices within the native list viewport, without retaining search fixtures.
for fixture in gamma delta beta alpha; do
  uid=$(jq -r '.issue.uid' "$tmp/search-$fixture.json")
  short=$(jq -r '.issue.short_id' "$tmp/search-$fixture.json")
  kata --workspace "$tmp/workspace" delete "$uid" --force --confirm "DELETE smoke#$short" --json
done
ctl key g
expect_tree "[data-id='main.columns.list.issues']" '.nodes[0].text | contains("Smoke first Issue")'
