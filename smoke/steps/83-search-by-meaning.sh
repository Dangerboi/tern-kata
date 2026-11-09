# ctrl+t in the search box switches the search to by meaning: enter runs kata's own search of the Project and lists what it found.
# The smoke daemon has no embeddings, so kata answers by words and the search line says so. esc out of the query ends the search.
# Own Issues in the seeded Project; it cleans up.
kata --workspace "$tmp/workspace" create "Meaning smoke zebra" --json >"$tmp/meaning-zebra.json"
meaning_zebra=$(jq -r '.issue.uid' "$tmp/meaning-zebra.json")
kata --workspace "$tmp/workspace" create "Meaning smoke other" --json >"$tmp/meaning-other.json"
meaning_other=$(jq -r '.issue.uid' "$tmp/meaning-other.json")
expect_tree "[data-id\$='.$meaning_zebra']" '.ok and .count == 1'
expect_tree "[data-id\$='.$meaning_other']" '.ok and .count == 1'

meaning_line="[data-id='main.columns.list.search-line']"
# meaning_search opens the search, switches it to by meaning and runs a query for the zebra Issue. Headless Tern delivers ctrl+t
# as meta+t, which the key table binds too.
meaning_search() {
  ctl key /
  ctl key ctrl+t
  for letter in z e b r a; do ctl key "$letter"; done
  ctl key enter
}
meaning_search
expect_tree "$meaning_line" '.ok and .count == 1 and (.nodes[0].text | startswith("kata searched by words"))'
expect_tree "[data-id\$='.$meaning_zebra']" '.ok and .count == 1'
expect_tree "[data-id\$='.$meaning_other']" '.ok and .count == 0'

# esc out of the query ends the search and the next one starts by words again.
ctl key escape
ctl key escape
expect_tree "$meaning_line" '.ok and .count == 0'
expect_tree "[data-id\$='.$meaning_zebra']" '.ok and .count == 1'
expect_tree "[data-id\$='.$meaning_other']" '.ok and .count == 1'
ctl key /
for letter in z e b r a; do ctl key "$letter"; done
expect_tree "[data-id\$='.$meaning_zebra']" '.ok and .count == 1'
expect_tree "[data-id\$='.$meaning_other']" '.ok and .count == 0'
expect_tree "$meaning_line" '.ok and .count == 0'
ctl key escape
expect_tree "[data-id\$='.$meaning_other']" '.ok and .count == 1'

for fixture in zebra other; do
  short=$(jq -r '.issue.short_id' "$tmp/meaning-$fixture.json")
  kata --workspace "$tmp/workspace" delete "$short" --force --confirm "DELETE smoke#$short" --json
done
