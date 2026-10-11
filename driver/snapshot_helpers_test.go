package driver_test

import "github.com/Dangerboi/tern-kata/model"

// loadFirst is the Project arriving and the snapshot its read asks for.
func loadFirst(m model.Model, loaded model.ProjectLoaded, snapshot model.SnapshotLoaded) model.Model {
	m, cmds := model.Update(m, loaded)
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			snapshot.Request, snapshot.Query = read.Request, read.Query
			m, _ = model.Update(m, snapshot)
			return m
		}
	}
	panic("the Project did not ask for a snapshot")
}
