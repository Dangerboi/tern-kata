# An agent claims an Issue from the CLI while a Draft is open and again while the Close card is open: the claim
# moves the Issue's revision but not its text, so the save and the close both go through. Own Issues; it cleans up.
ctl key q
expect_tree ".sf-list" '.ok and .count == 0'
ctl --file "$tmp/launch.ctl"
kata --workspace "$tmp/workspace" create "Smoke claim Draft Issue" --body "Claim seeded body" --json >"$tmp/claim-draft.json"
kata --workspace "$tmp/workspace" create "Smoke claim close Issue" --body "Claim close body" --json >"$tmp/claim-close.json"
claim_draft=$(jq -r '.issue.uid' "$tmp/claim-draft.json")
claim_close=$(jq -r '.issue.uid' "$tmp/claim-close.json")

expect_tree "[data-id='main.columns.list.issues.$claim_draft']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$claim_draft']"
ctl key enter
ctl key e
expect_tree "[data-id='layer.editor.body']" '.ok and .count == 1'
ctl key End
ctl type '" + saved after a claim"'
kata --workspace "$tmp/workspace" claim "$claim_draft" --if-unowned --json >/dev/null
ctl key Ctrl+o
expect_tree "[data-id='layer.editor']" '.ok and .count == 0'
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ $(kata --workspace "$tmp/workspace" show "$claim_draft" --json | jq -r '.issue.body') == "Claim seeded body + saved after a claim" ]] && break
  sleep 0.1
done
[[ $(kata --workspace "$tmp/workspace" show "$claim_draft" --json | jq -r '.issue.body') == "Claim seeded body + saved after a claim" ]]
ctl key Escape

expect_tree "[data-id='main.columns.list.issues.$claim_close']" '.ok and .count == 1'
ctl click "[data-id='main.columns.list.issues.$claim_close']"
expect_tree "[data-id='main.columns.list.issues.$claim_close']" '.ok and .nodes[0]["aria-selected"] == "true"'
ctl key x
expect_tree "[data-id='layer.close.actions.quick']" '.ok and .count == 1'
kata --workspace "$tmp/workspace" claim "$claim_close" --if-unowned --json >/dev/null
ctl click "[data-id='layer.close.actions.quick']"
expect_tree "[data-id='layer.close']" '.ok and .count == 0'
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ $(kata --workspace "$tmp/workspace" show "$claim_close" --json | jq -r '.issue.status') == closed ]] && break
  sleep 0.1
done
[[ $(kata --workspace "$tmp/workspace" show "$claim_close" --json | jq -r '.issue.status') == closed ]]

for fixture in claim-draft claim-close; do
  uid=$(jq -r '.issue.uid' "$tmp/$fixture.json")
  short=$(jq -r '.issue.short_id' "$tmp/$fixture.json")
  kata --workspace "$tmp/workspace" delete "$uid" --force --confirm "DELETE smoke#$short" --json
done
