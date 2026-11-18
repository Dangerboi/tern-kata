# The Credentials Page (C) lists the daemon's API tokens as kata redacts them: state, name, actor, scope, created, expires, last used
# and revoked, under "count · as of <time>"; r reads them again and esc returns. Own token; it revokes it.
kata --workspace "$tmp/workspace" tokens create --actor smoke-credentials --name "Smoke credential" --json >/dev/null
cr_id=$(kata tokens list --json | jq -r '[.tokens[] | select(.name == "Smoke credential")][0].id')
ctl key C
cred="main.columns.list.credentials.$cr_id.properties"
expect_tree "[data-id='$cred.state.v']" '.ok and .count == 1 and (.nodes[0].text == "live")'
expect_tree "[data-id='$cred.name.v']" '.ok and (.nodes[0].text == "Smoke credential")'
expect_tree "[data-id='$cred.actor.v']" '.ok and (.nodes[0].text == "smoke-credentials")'
expect_tree "[data-id='$cred.scope.v']" '.ok and (.nodes[0].text == "Global")'
expect_tree "[data-id='$cred.created.v']" '.ok and (.nodes[0].text | test("^([0-9]{2}:[0-9]{2}|20[0-9-]+ [0-9]{2}:[0-9]{2})$"))'
expect_tree "[data-id='$cred.expires.v']" '.ok and (.nodes[0].text == "—")'
expect_tree "[data-id='$cred.revoked.v']" '.ok and (.nodes[0].text == "—")'
expect_tree "[data-id='main.columns.list.heading.facts']" '.ok and (.nodes[0].text | contains("as of "))'
# A token revoked behind the Pane's back shows as revoked after r.
kata tokens revoke "$cr_id" --json >/dev/null
ctl key r
expect_tree "[data-id='$cred.state.v']" '.ok and (.nodes[0].text == "revoked")'
expect_tree "[data-id='$cred.revoked.v']" '.ok and (.nodes[0].text != "—")'
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
expect_tree "[data-id='layer.token.file']" '.ok and (.nodes[0].text | startswith("The secret is in ") and contains("'"$pane_token_file"'") and (contains("kata_") | not))'
test -s "$pane_token_file"
[[ "$(stat -c %a "$pane_token_file")" == 600 ]]
pane_token_id=$(kata tokens list --json | jq -r '[.tokens[] | select(.name == "Smoke pane token")][0].id')
ctl key escape
expect_tree "[data-id='main.columns.list.credentials.$pane_token_id.properties.state.v']" '.ok and (.nodes[0].text == "live")'
expect_tree "[data-id='main.columns.list.credentials.$pane_token_id.properties.name.v']" '.ok and (.nodes[0].text == "Smoke pane token")'
# The file now exists, so a second token into it is refused and nothing is made.
ctl key n
ctl key Tab
ctl type '"Smoke refused token"'
for _ in 1 2 3 4 5; do ctl key Tab; done
ctl type "$(jq -Rn --arg path "$pane_token_file" '$path')"
ctl key enter
expect_tree "[data-id='layer.token.foot.error']" '.ok and (.nodes[0].text | contains("already exists"))'
[[ "$(kata tokens list --json | jq '[.tokens[] | select(.name == "Smoke refused token")] | length')" == 0 ]]
ctl key escape
ctl key x
expect_tree "[data-id='layer.revoke.foot.question']" '.ok and .nodes[0].text == "Revoke token '"$pane_token_id"'?"'
expect_tree "[data-id='layer.revoke.consequence']" '.ok and (.nodes[0].text | contains("Smoke pane token") and contains("This cannot be undone."))'
ctl key y
expect_tree "[data-id='layer.revoke']" '.count == 0'
expect_tree "[data-id='main.columns.list.credentials.$pane_token_id.properties.state.v']" '.ok and (.nodes[0].text == "revoked")'
ctl key escape
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
