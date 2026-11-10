# The Config Page (K) has a Search by meaning row after the Docbank row, for the daemon the Pane is connected to. The smoke daemon
# has no embeddings, so the row reads that kata searches by words. The row is read-only: the step reads it and leaves.
ctl key K
expect_tree "[data-id='main.columns.list.config.embeddings.title']" '.ok and .count == 1 and (.nodes[0].text | startswith("Search by meaning (kata daemon:"))'
expect_tree "[data-id='main.columns.list.config.embeddings.summary']" '.ok and .count == 1 and (.nodes[0].text | startswith("off: kata searches by words"))'
ctl key escape
