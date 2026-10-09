# A search that is an Issue id lists that Issue alone, even when another Issue's body mentions the id (kata's full-text
# search would list the mentioning one instead); a search that is not an id stays full text. Own Issues in the seeded
# Project; it cleans up.
kata --workspace "$tmp/workspace" create "Idsearch target" --json >"$tmp/idsearch-target.json"
id_target=$(jq -r '.issue.uid' "$tmp/idsearch-target.json")
id_short=$(jq -r '.issue.short_id' "$tmp/idsearch-target.json")
kata --workspace "$tmp/workspace" create "Idsearch mention" --body "Mentions $id_short in its body" --json >"$tmp/idsearch-mention.json"
id_mention=$(jq -r '.issue.uid' "$tmp/idsearch-mention.json")
expect_tree "[data-id\$='.$id_target']" '.ok and .count == 1'
expect_tree "[data-id\$='.$id_mention']" '.ok and .count == 1'

ctl key /
for ((i = 0; i < ${#id_short}; i++)); do ctl key "${id_short:i:1}"; done
expect_tree "[data-id='main.filters']" ".nodes[0].text | contains(\"search=$id_short\")"
expect_tree "[data-id\$='.$id_target']" '.ok and .count == 1'
expect_tree "[data-id\$='.$id_mention']" '.ok and .count == 0'
# The result behaves as any search result: enter focuses it, a second enter commits it, and the Issue stays alone and selected.
ctl key enter
ctl key enter
expect_tree "[data-id='main.columns.list.search']" '.count == 0'
expect_tree "[data-id\$='.$id_target']" '.ok and .count == 1 and .nodes[0]["aria-selected"] == "true"'
expect_tree "[data-id\$='.$id_mention']" '.ok and .count == 0'

# Words are still full text: both Issues carry "Idsearch".
ctl key c
expect_tree "[data-id='main.filters']" '.ok and .count == 0'
ctl key /
for letter in I d s e a r c h; do ctl key "$letter"; done
expect_tree "[data-id\$='.$id_target']" '.ok and .count == 1'
expect_tree "[data-id\$='.$id_mention']" '.ok and .count == 1'
ctl key enter
ctl key enter
ctl key c
expect_tree "[data-id='main.filters']" '.ok and .count == 0'

for fixture in target mention; do
  short=$(jq -r '.issue.short_id' "$tmp/idsearch-$fixture.json")
  kata --workspace "$tmp/workspace" delete "$short" --force --confirm "DELETE smoke#$short" --json
done
