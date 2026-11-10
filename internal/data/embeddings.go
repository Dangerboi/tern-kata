package data

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"os"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// EmbeddingsStatus reads kata's search by meaning setup for the connected daemon. An older daemon without the endpoint answers 404,
// which comes back as kata's error line like any other failure.
func (c *Client) EmbeddingsStatus(ctx context.Context, _ model.LoadEmbeddingsStatus) (model.EmbeddingsStatusLoaded, error) {
	response, err := c.GetLocalEmbeddingsWithResponse(ctx)
	if err != nil {
		return model.EmbeddingsStatusLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.EmbeddingsStatusLoaded{}, errors.New("kata answered without a search by meaning status")
	}
	status := embeddingsStatus(*response.JSON200)
	status.Daemon, status.Remote, status.Host = c.name, c.kind == model.DaemonRemote, c.remoteHost()
	return model.EmbeddingsStatusLoaded{Status: status}, nil
}

// remoteHost is the remote daemon's host[:port] as the Pane reaches it: the catalog entry's URL, KATA_SERVER or the workspace's
// pinned URL. It is empty for a local daemon and when no URL is known.
func (c *Client) remoteHost() string {
	if c.kind != model.DaemonRemote {
		return ""
	}
	u, err := url.Parse(cmp.Or(c.entry.URL, os.Getenv("KATA_SERVER"), c.pinURL))
	if err != nil {
		return ""
	}
	return u.Host
}

// TurnOnEmbeddings starts kata's local setup job on the connected daemon and returns the status kata answers with (202). kata
// refuses with 403 when this account lacks daemon administration and with 409 while a job runs or embeddings are already
// configured; either comes back as kata's own reason.
func (c *Client) TurnOnEmbeddings(ctx context.Context, cmd model.TurnOnEmbeddings) (model.EmbeddingsTurnedOn, error) {
	response, err := c.TurnOnLocalEmbeddingsWithResponse(ctx)
	if err != nil {
		return model.EmbeddingsTurnedOn{}, daemonError(err)
	}
	if response.JSON202 == nil {
		return model.EmbeddingsTurnedOn{}, errors.New("kata answered without a search by meaning status")
	}
	status := embeddingsStatus(*response.JSON202)
	status.Daemon, status.Remote, status.Host = c.name, c.kind == model.DaemonRemote, c.remoteHost()
	return model.EmbeddingsTurnedOn{Status: status, Connection: cmd.Connection}, nil
}

// embeddingsStatus maps kata's status. A job or indexing kata does not report stays nil.
func embeddingsStatus(s g.LocalEmbeddingsStatus) model.EmbeddingsStatus {
	status := model.EmbeddingsStatus{
		Provider: model.EmbeddingsProvider(s.Provider), DownloadBytes: s.DownloadBytes, OllamaAvailable: s.OllamaAvailable, CanChange: s.CanChange,
	}
	if j := s.Job; j != nil {
		status.Job = &model.EmbeddingsJob{State: model.EmbeddingsJobState(j.State), Downloaded: j.Downloaded, Total: j.Total}
		if j.ErrorData != nil {
			status.Job.Error = *j.ErrorData
		}
	}
	if i := s.Indexing; i != nil {
		status.Indexing = &model.EmbeddingsIndexing{Done: i.Done, Total: i.Total, Failed: i.Failed}
		if i.EtaSeconds != nil {
			status.Indexing.ETA = time.Duration(*i.EtaSeconds) * time.Second
		}
	}
	return status
}
