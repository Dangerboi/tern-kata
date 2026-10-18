# A comment made with `kata comment --reply` shows in the Comments tab as a reply naming the comment it answers, and the
# comment answered shows its reply count. Own Project and Issue; it cleans up.
cr_dir="$tmp/replies-workspace"
mkdir -p "$cr_dir" "$tmp/replies-unbound"
kata --workspace "$cr_dir" init --project replies --json >/dev/null
cr_project=$(kata --workspace "$cr_dir" projects list --json | jq -er '.projects[] | select(.name == "replies") | .uid')
kata --workspace "$cr_dir" create "Replies main" --json >"$tmp/replies-main.json"
cr_main=$(jq -r '.issue.uid' "$tmp/replies-main.json")
cr_short=$(jq -r '.issue.short_id' "$tmp/replies-main.json")
cr_first=$(kata --workspace "$cr_dir" comment "$cr_short" -m "the claim" --json | jq -r '.comment.uid')
kata --workspace "$cr_dir" comment "$cr_short" -m "an answer" --reply "$cr_first" --json >/dev/null

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/replies-unbound" "$KATA_HOME" "$tmp" >"$tmp/replies-launch.ctl"
ctl --file "$tmp/replies-launch.ctl"
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$cr_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$cr_project']"
expect_tree "[data-id\$='.$cr_main']" '.ok and .count == 1'
ctl click "[data-id\$='.$cr_main']"
ctl key enter
expect_tree "[data-id='main.columns.detail.viewport.content.activity.$cr_first']" '.ok and (.nodes[0].text | contains("1 reply"))'
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.']" '.ok and ([.nodes[] | select(.text | contains("replies to dangerboi'"'"'s comment"))] | length >= 1)'

quit_app
kata projects remove replies --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
