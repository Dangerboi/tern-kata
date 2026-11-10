# s on the Config Page's Search by meaning row opens the card, which asks before anything starts: kata uses your Ollama, or downloads
# about N MB to the daemon's machine. The smoke daemon has no embeddings and lets this account change them, so the question shows.
# n closes the card and leaves the row as it was. Nothing is downloaded: y is never pressed.
ctl key K
embeddings_summary="[data-id='main.columns.list.config.embeddings.summary']"
embeddings_card="[data-id='layer.embeddings']"
expect_tree "$embeddings_summary" '.ok and .count == 1 and (.nodes[0].text | startswith("off: kata searches by words"))'
row_before=$(ctl tree "$embeddings_summary" | jq -r '.nodes[0].text')

# s acts only on the Search by meaning row (and the old pinned Docbank form): walk down the rows until it opens the card.
for _ in $(seq 12); do
  ctl key s
  if ctl tree "$embeddings_card" | jq -e '.count == 1' >/dev/null; then break; fi
  ctl key j
done
expect_tree "$embeddings_card" '.ok and .count == 1'
expect_tree "[data-id='layer.embeddings.question']" '.ok and .count == 1 and (.nodes[0].text | startswith("Turn on search by meaning? kata ") and endswith(" y/n") and (contains("kata will use your Ollama; nothing to download.") or test("kata downloads about [0-9]+ MB \\(shared by all Kenn apps\\) to the kata daemon.s machine \\(this machine\\)\\. Search stays by words until the daemon restarts\\.")))'

# n sends nothing: the card goes and the row reads as before.
ctl key n
expect_tree "$embeddings_card" '.ok and .count == 0'
expect_tree "$embeddings_summary" ".ok and .count == 1 and (.nodes[0].text == $(jq -Rn --arg t "$row_before" '$t'))"
ctl key escape
