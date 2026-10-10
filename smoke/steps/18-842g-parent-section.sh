# A child whose body starts with a `## Parent` section naming its parent draws no Parent heading in the
# body, and its Parent: property sits below Labels:. Own Project and Issues; it cleans up.
ps_dir="$tmp/parentsec-workspace"
mkdir -p "$ps_dir" "$tmp/parentsec-unbound"
kata --workspace "$ps_dir" init --project parentsec --json >/dev/null
ps_project=$(kata --workspace "$ps_dir" projects list --json | jq -er '.projects[] | select(.name == "parentsec") | .uid')
kata --workspace "$ps_dir" create "Parentsec parent" --body "Parent body" --json >"$tmp/parentsec-parent.json"
ps_parent=$(jq -r '.issue.uid' "$tmp/parentsec-parent.json")
ps_short=$(jq -r '.issue.short_id' "$tmp/parentsec-parent.json")
kata --workspace "$ps_dir" create "Parentsec kid" --force-new --parent "$ps_parent" --label bug --body "$(printf '## Parent\n\n%s\n\n## What to build\n\nthe kid body' "$ps_short")" --json >"$tmp/parentsec-kid.json"
ps_kid=$(jq -r '.issue.uid' "$tmp/parentsec-kid.json")

quit_app
printf 'ready\nrun "(cd %s && env -u KATA_SERVER -u KATA_AUTH_TOKEN -u TMUX -u STY -u ZELLIJ -u TERN_TSP KATA_HOME=%s %s/tern-kata)"\n' "$tmp/parentsec-unbound" "$KATA_HOME" "$tmp" >"$tmp/parentsec-launch.ctl"
ctl --file "$tmp/parentsec-launch.ctl"
ctl click "[data-id='main.columns.collections.projects']"
expect_tree "[data-id='main.columns.collections.project.$ps_project']" '.ok and .count == 1'
ctl click "[data-id='main.columns.collections.project.$ps_project']"
expect_tree "[data-id^='main.columns.list.'][data-id\$='.$ps_parent']" '.ok and .count == 1'
ctl click "[data-id^='main.columns.list.'][data-id\$='.$ps_parent']"
expect_tree "[data-id^='main.columns.list.'][data-id\$='.$ps_kid']" '.ok and .count == 1'
ctl click "[data-id^='main.columns.list.'][data-id\$='.$ps_kid']"
ps_content="main.columns.detail.viewport.content"
expect_tree "[data-id='$ps_content.heading.title']" '.ok and (.nodes[0].text | contains("Parentsec kid") and (contains("loading") | not))'
expect_tree "[data-id='$ps_content.body']" '.ok and .count == 1 and (.nodes[0].text | contains("the kid body"))'
# No heading named Parent anywhere in the body.
expect_tree "[data-id='$ps_content.body'] *" '.ok and ([.nodes[].text // ""] | map(select(. == "Parent")) | length == 0)'
expect_tree "[data-id='$ps_content.properties.parent']" '.ok and .count == 1'
ps_labels=$(ctl tree "[data-id='$ps_content.properties.labels']" | jq -r '.nodes[0].rect[1]')
ps_parent_y=$(ctl tree "[data-id='$ps_content.properties.parent']" | jq -r '.nodes[0].rect[1]')
awk -v a="$ps_labels" -v b="$ps_parent_y" 'BEGIN { exit !(a < b) }' || { printf 'Parent: is not below Labels: (%s, %s)\n' "$ps_labels" "$ps_parent_y" >&2; return 1; }

quit_app
kata projects remove parentsec --force --json >/dev/null
ctl --file "$tmp/launch.ctl"
ctl plugins expect smoke
expect_tree "[data-id='main.columns.list']" '.ok and .count == 1'
