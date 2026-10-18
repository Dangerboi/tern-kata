# R on a comment in the Comments tab opens the comment Draft as a reply to it; the kind key switches to confirm and saving
# posts it, so `kata show` lists the reply with its kind and the comment it answers. Own Project and Issue; it cleans up.
rp_dir="$tmp/reply-post-workspace"
mkdir -p "$rp_dir" "$tmp/reply-post-unbound"
kata --workspace "$rp_dir" init --project replypost --json >/dev/null
rp_project=$(kata --workspace "$rp_dir" projects list --json | jq -er '.projects[] | select(.name == "replypost") | .uid')
kata --workspace "$rp_dir" create "Reply post main" --json >"$tmp/reply-post-main.json"
rp_main=$(jq -r '.issue.uid' "$tmp/reply-post-main.json")
rp_short=$(jq -r '.issue.short_id' "$tmp/reply-post-main.json")
rp_first=$(kata --workspace "$rp_dir" comment "$rp_short" -m "the claim" --json | jq -r '.comment.uid')

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/reply-post-unbound" "$KATA_HOME" "$tmp" >"$tmp/reply-post-launch.ctl"
ctl --file "$tmp/reply-post-launch.ctl"
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$rp_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$rp_project']"
expect_tree "[data-id\$='.$rp_main']" '.ok and .count == 1'
ctl click "[data-id\$='.$rp_main']"
ctl key enter
expect_tree "[data-id='main.columns.detail.viewport.content.activity.$rp_first']" '.ok and .count == 1'
ctl key R
expect_tree "[data-id='layer.editor']" '.ok and .count == 1'
expect_tree "[data-id='layer.editor']" '.nodes[0].text | startswith("Reply to dangerboi'"'"'s comment")'
ctl key Ctrl+r
expect_tree "[data-id='layer.editor']" '.nodes[0].text | startswith("Confirm dangerboi'"'"'s comment")'
ctl type '"It reproduces on a clean checkout every time."'
ctl key Ctrl+o
expect_tree "[data-id='layer.editor']" '.ok and .count == 0'
kata --workspace "$rp_dir" show "$rp_short" --json >"$tmp/reply-post-shown.json"
jq -e --arg first "$rp_first" '.comments | map(select(.reply_kind == "confirm" and .reply.uid == $first and (.body | startswith("It reproduces")))) | length == 1' "$tmp/reply-post-shown.json" >/dev/null

quit_app
kata projects remove replypost --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
