# The Attention Page (6) lists the requests kata holds for the Pane's Inbox address, across Projects. A request seeded with
# `kata notify --to` shows as a row, and Enter opens its Issue on the Issues Page. The Pane runs as its own actor so the address is known.
quit_app
at_dir="$tmp/workspace"
kata --workspace "$at_dir" create "Smoke attention Issue" --body "Issue with a request" --json >"$tmp/attention-issue.json"
at_issue=$(jq -r '.issue.uid' "$tmp/attention-issue.json")
at_short=$(jq -r '.issue.short_id' "$tmp/attention-issue.json")
kata --as smoke-asker --workspace "$at_dir" notify "$at_short" --to smoke-attention --message "Please look at this Issue" --json >/dev/null
printf 'ready\nrun "env -u KATA_SERVER -u KATA_AUTH_TOKEN -u KATA_INBOX_USER -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata --as smoke-attention"\n' "$KATA_HOME" "$tmp" >"$tmp/attention-launch.ctl"
ctl --file "$tmp/attention-launch.ctl"
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
ctl key 6
expect_tree "[data-id='main.columns.list.attention.$at_issue']" '.ok and .count == 1 and (.nodes[0].text | contains("Smoke attention Issue") and contains("smoke-asker") and contains("request") and contains("Please look at this Issue"))'
expect_tree "[data-id='main.columns.list.attention']" '.ok and .count == 1'
expect_tree "[data-id='main.columns.list.issues']" '.count == 0'
ctl key enter
expect_tree "[data-id='main.columns.list.issues']" '.count == 1'
expect_tree "[data-id^='main.columns.detail']" '.ok and ([.nodes[].text] | any(contains("Smoke attention Issue")))'
ctl key 6
expect_tree "[data-id='main.columns.list.attention.$at_issue']" '.ok and .count == 1'
ctl key 1
quit_app
kata --as smoke-asker --workspace "$at_dir" notify "$at_short" --to smoke-attention --clear --json >/dev/null
ctl --file "$tmp/launch.ctl"
