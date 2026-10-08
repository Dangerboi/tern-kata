package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// narrowView is what a Pane under model.MinCols shows instead of the app: one
// centred notice, with no footer or layers. The model is untouched, so
// widening the Pane brings the previous screen back.
func narrowView() tern.View {
	grow := func(key string) tern.Element { return ui.Spacer{Common: ui.Common{Key: key, Grow: 1}} }
	notice := ui.Text{Common: ui.Common{Key: "text", Tone: ui.TonePending}, Text: fmt.Sprintf("Widen this Pane to at least %d columns to use kata.", model.MinCols)}
	return tern.View{Main: ui.Nodes(ui.Col{
		Common:   ui.Common{Key: "notice", Grow: 1},
		Align:    ui.AlignCenter,
		Children: []tern.Element{grow("above"), notice, grow("below")},
	})}
}
