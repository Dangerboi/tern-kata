package data

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// DocbankSupervision is the plugin's supervision rule for Docbank, as docbanksetup.Env answers it. The Gateway cannot import that
// package (it imports this one), so the program hands it in with WithDocbankSupervision.
type DocbankSupervision interface {
	// ManualPath is the marker docbank.manual.
	ManualPath() string
	// Supervised says whether the plugin keeps a local Docbank running, and when not, why.
	Supervised() (on bool, why string)
	// SetSupervision turns supervision on (the marker is removed) or off (it is created).
	SetSupervision(on bool) error
}

// WithDocbankSupervision sets how the Gateway builds the supervision environment, asked afresh for each read and switch so a
// changed KATA_HOME or config file shows. Without it the Config Page has no Docbank row and the switch fails.
func (g *Gateway) WithDocbankSupervision(build func() DocbankSupervision) *Gateway {
	g.docbank = build
	return g
}

// docbankRow is the Docbank supervision row: the marker's path and whether it exists, and the rule's own answer as the summary.
// It describes the default kata daemon's Docbank, whichever daemon the Pane is switched to.
func (g *Gateway) docbankRow() (model.ConfigFile, bool) {
	if g.docbank == nil {
		return model.ConfigFile{}, false
	}
	env := g.docbank()
	row := model.ConfigFile{Key: model.ConfigDocbank, Title: "docbank.manual (Docbank of the default kata daemon)", Path: env.ManualPath(), Actions: []model.ConfigAction{model.ConfigSwitch}}
	if _, err := os.Stat(row.Path); err == nil {
		row.Status = model.ConfigLoads
	} else if !errors.Is(err, fs.ErrNotExist) {
		row.Status, row.Err = model.ConfigBroken, err.Error()
	}
	on, why := env.Supervised()
	row.Supervised = on
	if on {
		row.Summary = "on: the plugin keeps Docbank running"
	} else {
		row.Summary = "off: " + why
	}
	return row, true
}

func (g *Gateway) setDocbankSupervision(_ context.Context, cmd model.SetDocbankSupervision) model.Msg {
	done := model.DocbankSupervisionSet{On: cmd.On}
	if g.docbank == nil {
		done.Err = errors.New("Docbank supervision is not available here")
		return done
	}
	done.Err = g.docbank().SetSupervision(cmd.On)
	return done
}
