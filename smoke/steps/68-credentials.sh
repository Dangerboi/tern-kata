# The Credentials Page (C) lists the daemon's API tokens as kata redacts them: state, id, name, actor, scope, created, expires, last
# observed use and revoked, under "N credentials · observed at <time>"; r reads them again and esc returns. Own token; it revokes it.
kata --workspace "$tmp/workspace" tokens create --actor smoke-credentials --name "Smoke credential" --json >/dev/null
cr_id=$(kata tokens list --json | jq -r '[.tokens[] | select(.name == "Smoke credential")][0].id')
ctl key C
expect_tree "[data-id='main.columns.list.credentials.$cr_id.line0']" '.ok and .count == 1 and (.nodes[0].text | contains("state live") and contains("name Smoke credential") and contains("actor smoke-credentials"))'
expect_tree "[data-id='main.columns.list.credentials.$cr_id.line1']" '.ok and (.nodes[0].text | contains("scope global"))'
expect_tree "[data-id='main.columns.list.credentials.$cr_id.line2']" '.ok and (.nodes[0].text | contains("created 20") and contains("expires —"))'
expect_tree "[data-id='main.columns.list.credentials.$cr_id.line3']" '.ok and (.nodes[0].text | contains("revoked —"))'
expect_tree "[data-id='main.columns.list.summary']" '.ok and (.nodes[0].text | contains("credentials · observed at"))'
# A token revoked behind the Pane's back shows as revoked after r.
kata tokens revoke "$cr_id" --json >/dev/null
ctl key r
expect_tree "[data-id='main.columns.list.credentials.$cr_id.line0']" '.ok and (.nodes[0].text | contains("state revoked"))'
expect_tree "[data-id='main.columns.list.credentials.$cr_id.line3']" '.ok and (.nodes[0].text | contains("revoked 20"))'
ctl key escape
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
