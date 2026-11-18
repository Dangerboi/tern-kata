package model

// HistoryRowOf phrases one event the way the Events tab does, including the events the tab does not list.
func HistoryRowOf(m Model, event Event) DetailRow { return m.historyRow(event) }
