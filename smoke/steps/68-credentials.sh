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
# n opens the Token card: it makes a token whose secret goes to a new owner-only file and is never drawn; the card names the file and
# the token's id, and the list shows the token after it. The card refuses a file that exists, and x then y revokes the token.
pane_token_file="$tmp/pane-token.secret"
ctl key n
expect_tree "[data-id='layer.token.file']" '.count == 1'
ctl key Tab
ctl type '"Smoke pane token"'
for _ in 1 2 3 4 5; do ctl key Tab; done
ctl type "$(jq -Rn --arg path "$pane_token_file" '$path')"
ctl key enter
expect_tree "[data-id='layer.token.done']" '.ok and (.nodes[0].text | contains("Created token"))'
expect_tree "[data-id='layer.token.file']" '.ok and (.nodes[0].text | contains("'"$pane_token_file"'") and (contains("kata_") | not))'
test -s "$pane_token_file"
[[ "$(stat -c %a "$pane_token_file")" == 600 ]]
pane_token_id=$(kata tokens list --json | jq -r '[.tokens[] | select(.name == "Smoke pane token")][0].id')
ctl key escape
expect_tree "[data-id='main.columns.list.credentials.$pane_token_id.line0']" '.ok and (.nodes[0].text | contains("state live") and contains("name Smoke pane token"))'
# The file now exists, so a second token into it is refused and nothing is made.
ctl key n
ctl key Tab
ctl type '"Smoke refused token"'
for _ in 1 2 3 4 5; do ctl key Tab; done
ctl type "$(jq -Rn --arg path "$pane_token_file" '$path')"
ctl key enter
expect_tree "[data-id='layer.token.error']" '.ok and (.nodes[0].text | contains("already exists"))'
[[ "$(kata tokens list --json | jq '[.tokens[] | select(.name == "Smoke refused token")] | length')" == 0 ]]
ctl key escape
ctl key x
expect_tree "[data-id='layer.revoke.question']" '.ok and (.nodes[0].text | contains("Revoke token '"$pane_token_id"' (Smoke pane token") and contains("This cannot be undone."))'
ctl key y
expect_tree "[data-id='layer.revoke']" '.count == 0'
expect_tree "[data-id='main.columns.list.credentials.$pane_token_id.line0']" '.ok and (.nodes[0].text | contains("state revoked"))'
ctl key escape
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
