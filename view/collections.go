package view

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func Collections(m model.Model) tern.Element {
	return ui.Col{Common: ui.Common{Key: "collections"}}
}
