# The Config Page (K) has a Search by meaning row after the Docbank row, for the daemon the Pane is connected to. The smoke daemon
# has no embeddings, so the row reads that kata searches by words. s on the row opens the card, which asks before anything starts:
# kata uses your Ollama, or downloads about N MB to the daemon's machine. The smoke daemon has no embeddings and lets this account
# change them, so the question shows. n closes the card and leaves the row as it was. Nothing is downloaded: y is never pressed.
ctl key K
embeddings_summary="[data-id='main.columns.list.config.embeddings.summary']"
embeddings_card="[data-id='layer.embeddings']"
# The title is the name only: the row has no status words after it (a file row says "· loads" or "· missing"), and no path.
expect_tree "[data-id='main.columns.list.config.embeddings.title']" '.ok and .count == 1 and (.nodes[0].text | test("^Search by meaning \\(kata daemon: [^)]+\\)$"))'
expect_tree "[data-id='main.columns.list.config.embeddings.path']" '.ok and .count == 0'
expect_tree "$embeddings_summary" '.ok and .count == 1 and (.nodes[0].text | startswith("off: kata searches by words"))'
row_before=$(ctl tree "$embeddings_summary" | jq -r '.nodes[0].text')

# The row's turn-on button is s on that row: a click moves the cursor there and asks, as the key does.
embeddings_turn_on="[data-id='main.columns.list.config.embeddings.turn-on']"
expect_tree "$embeddings_turn_on" '.ok and .count == 1 and (.nodes[0].text | contains("turn on search by meaning"))'
ctl click "$embeddings_turn_on"
expect_tree "$embeddings_card" '.ok and .count == 1'
expect_tree "[data-id='layer.embeddings.question']" '.ok and .count == 1 and (.nodes[0].text | startswith("Turn on search by meaning? kata ") and endswith(" y/n") and (contains("kata will use your Ollama; nothing to download.") or contains("kata will use its built-in model, already on the kata daemon'"'"'s machine; nothing to download.") or test("kata downloads about [0-9]+ MB \\(shared by all Kenn apps\\) to the kata daemon.s machine \\(this machine\\)\\. Search stays by words until the daemon restarts\\.")))'

# n sends nothing: the card goes and the row reads as before.
ctl key n
expect_tree "$embeddings_card" '.ok and .count == 0'
expect_tree "$embeddings_summary" ".ok and .count == 1 and (.nodes[0].text == $(jq -Rn --arg t "$row_before" '$t'))"
ctl key escape
