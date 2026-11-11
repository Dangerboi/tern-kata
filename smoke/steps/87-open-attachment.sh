# A click on an image Attachment shows it in Tern: `tern open --preview` runs from the Pane, so the tab gets a preview file block on
# the downloaded copy in tern-kata's cache and the focus stays in the Pane. A second Attachment replaces the preview. This step
# gives the smoke daemon a Docbank of its own (a vault in $tmp) and runs last, because the earlier steps expect no Docbank.
export DOCBANK_HOME="$tmp/docbank" DOCBANK_TELEMETRY_ENABLED=0
oa_dir="$tmp/workspace"
quit_app
kata --workspace "$oa_dir" docbank setup --yes >/dev/null
kata --workspace "$oa_dir" create "Smoke open attachment" --json >"$tmp/open-attachment.json"
oa_uid=$(jq -r '.issue.uid' "$tmp/open-attachment.json")
oa_short=$(jq -r '.issue.short_id' "$tmp/open-attachment.json")
# 8x8 PNG and a text file: the kinds Tern draws in its own preview block.
printf 'iVBORw0KGgoAAAANSUhEUgAAAAgAAAAICAIAAABLbSncAAAAEklEQVR4nGP4z8CAFTEMLQkAbMcr8TDBSnYAAAAASUVORK5CYII=' | base64 -d >"$tmp/shot.png"
printf 'a plain log\n' >"$tmp/notes.txt"
kata --workspace "$oa_dir" docbank attachments add "$oa_short" "$tmp/shot.png" "$tmp/notes.txt" >/dev/null
ctl --file "$tmp/launch.ctl"
expect_tree "[data-id='main.columns.list.issues.$oa_uid']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$oa_uid']"
expect_tree "[data-id='main.columns.list.issues.$oa_uid']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl click "[data-id='main.columns.detail.viewport.content.activity.tabs.tab3']"
expect_tree "[data-id^='main.columns.detail.viewport.content.activity.attachment-']" '.ok and .count == 2'
oa_focus=$(ctl state | jq '.focused.id')

# A click on the row of the Attachment named $1 opens it: its row key is its id in hex.
oa_open_row() {
  local id
  id=$(kata --workspace "$oa_dir" docbank attachments ls "$oa_short" --json | jq -r --arg n "$1" '.attachments | to_entries[] | select(.value.name == $n) | .key')
  ctl click "[data-id='main.columns.detail.viewport.content.activity.attachment-$(printf %s "$id" | od -An -tx1 | tr -d ' \n')']"
}
# The tab's one preview block shows file $2 (of kind $1) from the cache, and the focus is where it was.
oa_expect_preview() {
  local deadline=$((SECONDS + wait_limit))
  while (( SECONDS < deadline )); do
    ctl state >"$tmp/oa-state.json"
    if jq -e --arg kind "$1" --arg name "$2" --argjson focus "$oa_focus" '(.file_blocks | length) == 1 and (.file_blocks[0] | .preview == true and .load == "ready" and .kind == $kind and (.path | test("/tern-kata/attachments/.+/" + $name + "$"))) and .focused.id == $focus' "$tmp/oa-state.json" >/dev/null 2>&1; then return 0; fi
    sleep 0.1
  done
  cat "$tmp/oa-state.json" >&2
  printf 'No preview block of %s for %s\n' "$1" "$2" >&2
  return 1
}
oa_open_row shot.png
oa_expect_preview image shot.png
# The Pane inherits run.sh's XDG_CACHE_HOME, so the download lands in $tmp and never in the user's ~/.cache.
jq -e --arg cache "$XDG_CACHE_HOME/" '.file_blocks[0].path | startswith($cache)' "$tmp/oa-state.json" >/dev/null || { echo "Attachment downloaded outside $XDG_CACHE_HOME" >&2; exit 1; }
expect_tree "[data-id='main.notice']" '.ok and .count == 1 and (.nodes[0].text | contains("shot.png"))'
oa_open_row notes.txt
oa_expect_preview code notes.txt

# Leave as found: the Issue is deleted and the daemon has no Docbank again for later steps. The cached copies and the vault are in
# $tmp, which the run removes.
kata --workspace "$oa_dir" delete "$oa_uid" --force --confirm "DELETE smoke#$oa_short" --json >/dev/null
docbank daemon stop >/dev/null 2>&1 || true
