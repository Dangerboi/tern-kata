package model

// WithSnapshotRead is m with a snapshot read out when none was ever asked for, for tests that build a Model by hand and answer a
// read it never made.
func WithSnapshotRead(m Model) Model {
	if !m.snapshotReads.Asked() {
		m.snapshotReads.Ask()
	}
	return m
}
