package data

import (
	"errors"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func (g *Gateway) readViews() model.ViewsLoaded {
	store, err := g.ViewsStore()
	if err != nil {
		return model.ViewsLoaded{Err: err}
	}
	return readViewsOf(store)
}

func readViewsOf(store ViewsStore) model.ViewsLoaded {
	read, err := store.Read()
	if err != nil {
		return model.ViewsLoaded{Err: err}
	}
	return model.ViewsLoaded{Views: read.Views, Problems: read.Problems, Token: read.Token}
}

func (g *Gateway) writeView(cmd model.SaveView) model.ViewSaved {
	store, err := g.ViewsStore()
	if err != nil {
		return model.ViewSaved{Sent: cmd, Err: err}
	}
	token, ok := cmd.Token.(guardedfile.Token)
	if !ok {
		return model.ViewSaved{Sent: cmd, Err: errors.New("views.toml was not read")}
	}
	if cmd.Self == "" {
		err = store.Add(token, cmd.View)
	} else {
		err = store.Replace(token, cmd.Self, cmd.View)
	}
	return model.ViewSaved{Sent: cmd, Changed: errors.Is(err, ErrViewsChanged), Err: err, Loaded: readViewsOf(store)}
}

func (g *Gateway) removeView(cmd model.DeleteView) model.ViewDeleted {
	store, err := g.ViewsStore()
	if err != nil {
		return model.ViewDeleted{Sent: cmd, Err: err}
	}
	token, ok := cmd.Token.(guardedfile.Token)
	if !ok {
		return model.ViewDeleted{Sent: cmd, Err: errors.New("views.toml was not read")}
	}
	err = store.Delete(token, cmd.Name)
	return model.ViewDeleted{Sent: cmd, Changed: errors.Is(err, ErrViewsChanged), Err: err, Loaded: readViewsOf(store)}
}
