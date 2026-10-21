# The detail title's Issue id reads `#<short_id>` in its Lane's colour (muted for an Issue on no Lane, as here), the rest of the title in the text colour.
ctl key enter
title_id="[data-id='main.columns.detail.viewport.content.heading.title']"
expect_tree "$title_id *" '.ok and .nodes[0].class == "sf-t-muted" and (.nodes[0].text | test("^#[0-9a-z]{4}$")) and (.nodes[1].text | contains("- "))'
for ((attempt = 0; attempt < 100; attempt++)); do
  ctl dump "$title_id *" >"$tmp/title-id-dump.json"
  if jq -e '.elements | length >= 2 and .[0].color != .[1].color' "$tmp/title-id-dump.json" >/dev/null; then break; fi
  sleep 0.1
done
jq -e '.elements | length >= 2 and .[0].color != .[1].color' "$tmp/title-id-dump.json" >/dev/null
ctl key escape
