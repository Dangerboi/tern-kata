# The Attention Page (6) lists the requests kata holds for the Pane's Inbox address, across Projects. A request seeded with
# `kata notify --to` shows as a row, Enter opens its Issue on the Issues Page, and `c` clears it. The Page bar count follows the request while the Pane is open. The Pane runs as its own actor so the address is known.
quit_app
at_dir="$tmp/workspace"
kata --workspace "$at_dir" create "Smoke attention Issue" --body "Issue with a request" --json >"$tmp/attention-issue.json"
at_issue=$(jq -r '.issue.uid' "$tmp/attention-issue.json")
at_short=$(jq -r '.issue.short_id' "$tmp/attention-issue.json")
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u KATA_INBOX_USER -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata --as smoke-attention"\n' "$KATA_HOME" "$tmp" >"$tmp/attention-launch.ctl"
ctl --file "$tmp/attention-launch.ctl"
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
at_bar="[data-id='main.topbar.left.pages.attention']"
expect_tree "$at_bar" '.ok and .count == 1 and (.nodes[0].text | contains("Attention") and (contains("Attention 1") | not))'
kata --as smoke-asker --workspace "$at_dir" notify "$at_short" --to smoke-attention --message "Please look at this Issue" --json >/dev/null
expect_tree "$at_bar" '.ok and (.nodes[0].text | contains("Attention 1"))'
ctl key 6
expect_tree "[data-id='main.columns.list.attention.$at_issue']" '.ok and .count == 1 and (.nodes[0].text | contains("Smoke attention Issue") and contains("smoke-asker") and contains("request") and contains("Please look at this Issue"))'
expect_tree "[data-id='main.columns.list.attention']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.issues']" '.count == 0'
ctl key enter
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke attention Issue")))'
ctl key 6
expect_tree "[data-id='main.columns.list.attention.$at_issue']" '.ok and .count == 1'
kata --workspace "$at_dir" inbox --for smoke-attention --all | grep -q "Smoke attention Issue"
ctl key c
expect_tree "[data-id='main.columns.list.attention.$at_issue']" '.ok and .count == 0'
if kata --workspace "$at_dir" inbox --for smoke-attention --all | grep -q "Smoke attention Issue"; then
  printf 'The cleared request is still in kata inbox\n' >&2
  return 1
fi
expect_tree "$at_bar" '.ok and (.nodes[0].text | contains("Attention") and (contains("Attention 1") | not))'
ctl key 1
quit_app
ctl --file "$tmp/launch.ctl"
