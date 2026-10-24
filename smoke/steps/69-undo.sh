# u undoes the newest change the Pane made: a close, a label and a link, newest first, and says so. Own Issues; it cleans up.
ud_dir="$tmp/workspace"
kata --workspace "$ud_dir" create "Smoke undo Issue" --json >"$tmp/undo-issue.json"
kata --workspace "$ud_dir" create "Smoke undo peer" --json >"$tmp/undo-peer.json"
ud_uid=$(jq -r '.issue.uid' "$tmp/undo-issue.json")
ud_short=$(jq -r '.issue.short_id' "$tmp/undo-issue.json")
ud_peer_uid=$(jq -r '.issue.uid' "$tmp/undo-peer.json")
ud_peer=$(jq -r '.issue.short_id' "$tmp/undo-peer.json")
ud_notice="[data-id='main.notice']"
expect_tree "[data-id='main.columns.list.issues.$ud_uid']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.issues.$ud_peer_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$ud_uid']"
expect_tree "[data-id='main.columns.list.issues.$ud_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key enter

# With nothing done yet there is nothing to undo.
ctl key u
expect_tree "$ud_notice" '.ok and .count == 1 and (.nodes[0].text | startswith("nothing to undo"))'

# A label, a link to the peer and a close.
ctl key +
ctl type '"undo-me"'
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
ctl key l
ctl type "\"$ud_peer\""
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
kata --workspace "$ud_dir" show "$ud_uid" --json >"$tmp/undo-linked.json"
jq -e '(.links | length) == 1 and (.labels | map(.label) | index("undo-me"))' "$tmp/undo-linked.json" >/dev/null
ctl key x
expect_tree "[data-id='layer.close.actions.quick']" '.ok and .count == 1'
ctl click "[data-id='layer.close.actions.quick']"
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
kata --workspace "$ud_dir" show "$ud_uid" --json | jq -e '.issue.status == "closed"' >/dev/null

# u reopens it, then removes the link, then removes the label; each leaves kata as it was before the change.
ctl key u
expect_tree "$ud_notice" '.ok and .count == 1 and (.nodes[0].text | startswith("undid close on "))'
kata --workspace "$ud_dir" show "$ud_uid" --json | jq -e '.issue.status == "open"' >/dev/null
ctl key u
expect_tree "$ud_notice" '.ok and .count == 1 and (.nodes[0].text | startswith("undid link.add on "))'
kata --workspace "$ud_dir" show "$ud_uid" --json | jq -e '(.links | length) == 0' >/dev/null
ctl key u
expect_tree "$ud_notice" '.ok and .count == 1 and (.nodes[0].text | startswith("undid label.add on "))'
kata --workspace "$ud_dir" show "$ud_uid" --json | jq -e '(.labels | map(.label) | index("undo-me")) == null' >/dev/null

# A timed assignment is a boundary: u says why instead of undoing the label before it.
ctl key +
ctl type '"again"'
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
ctl key t
ctl type '"1h"'
ctl key enter
expect_tree "[data-id='layer.fields']" '.ok and .count == 0'
ctl key u
expect_tree "$ud_notice" '.ok and .count == 1 and (.nodes[0].text == "nothing to undo: timed assignment cannot be undone")'

for uid_short in "$ud_uid $ud_short" "$ud_peer_uid $ud_peer"; do
  set -- $uid_short
  kata --workspace "$ud_dir" delete "$1" --force --confirm "DELETE smoke#$2" --json >/dev/null
done
expect_tree "[data-id='main.columns.list.issues.$ud_uid']" '.ok and .count == 0'
