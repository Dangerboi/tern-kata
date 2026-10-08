# Rows carry theme-toned badges and label chips, and the focused top bar title is accent.
# The fixture is deleted afterwards so later steps see the three seeded Issues.
kata --workspace "$tmp/workspace" create "Smoke colour Issue" --priority 0 --label colour-chip --json >"$tmp/colour.json"
colour_uid=$(jq -r '.issue.uid' "$tmp/colour.json")
colour_short=$(jq -r '.issue.short_id' "$tmp/colour.json")
colour_row="[data-id='main.columns.list.issues.$colour_uid']"
expect_tree "$colour_row" '.ok and .count == 1 and (.nodes[0].text | contains("P0 open"))'
expect_tree "$colour_row *" '[.nodes[] | select(.text == "open" or .text == "P0" or .text == "colour-chip") | {(.text): .class}] | add == {"open": "sf-t-accent", "P0": "sf-t-error", "colour-chip": "sf-t-mark"}'
expect_tree "[data-id='main.topbar.heading'] *" '.nodes[0].class == "sf-t-accent sf-t-strong"'

kata --workspace "$tmp/workspace" delete "$colour_uid" --force --confirm "DELETE smoke#$colour_short" --json
expect_tree "$colour_row" '.ok and .count == 0'
