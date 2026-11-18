# The detail title's Issue id reads `#<short_id>` in muted mono whatever its Lane, then the title in strong text (the tree trims the space between them), with no separator between them.
ctl key enter
title_id="[data-id='main.columns.detail.viewport.content.heading.title']"
expect_tree "$title_id *" '.ok and .nodes[0].class == "sf-t-muted sf-t-mono" and (.nodes[0].text | test("^#[0-9a-z]{4}$")) and (.nodes[1].class == "sf-t-strong") and (.nodes[1].text | startswith("- ") | not)'
title_id_coloured() {
  ctl dump "$title_id *" >"$tmp/title-id-dump.json"
  jq -e '.elements | length >= 2 and .[0].color != .[1].color' "$tmp/title-id-dump.json" >/dev/null
}
wait_for title_id_coloured
ctl key escape
