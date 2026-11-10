package data

import (
	"context"
	"errors"
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
	status.Daemon, status.Remote = c.name, c.kind == model.DaemonRemote
	return model.EmbeddingsStatusLoaded{Status: status}, nil
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
