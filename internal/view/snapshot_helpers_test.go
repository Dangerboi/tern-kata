package view_test

import "github.com/Dangerboi/tern-kata/internal/model"

// snap is a snapshot answering the read in flight: view tests send it through step, which fills in the request and query.
type snap struct{ model.SnapshotLoaded }

// step is model.Update, except that a snap answers the snapshot read in flight.
func step(m model.Model, msg model.Msg) (model.Model, []model.Cmd) {
	if s, ok := msg.(snap); ok {
		s.Request, s.Query = m.RefreshRequest, m.SnapshotQuery()
		msg = s.SnapshotLoaded
	}
	return model.Update(m, msg)
}

// loadFirst is the Project arriving and the snapshot its read asks for; it returns what applying the snapshot returned.
func loadFirst(m model.Model, loaded model.ProjectLoaded, snapshot model.SnapshotLoaded) (model.Model, []model.Cmd) {
	m, cmds := model.Update(m, loaded)
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			snapshot.Request, snapshot.Query = read.Request, read.Query
			return model.Update(m, snapshot)
		}
	}
	panic("the Project did not ask for a snapshot")
}
