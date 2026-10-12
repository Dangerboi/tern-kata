package data

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) ReadEvents(ctx context.Context, cmd model.Subscribe, send func(model.Msg) bool) error {
	response, err := c.StreamEventsRaw(ctx, &g.StreamEventsRequestOptions{Query: &g.StreamEventsQuery{AfterID: &cmd.AfterID}})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		return ReadError(body, fmt.Errorf("kata event stream: HTTP %d", response.StatusCode))
	}
	if !send(model.StreamOpened{Generation: cmd.Generation}) {
		return ctx.Err()
	}
	reader := bufio.NewReader(response.Body)
	var id, eventType string
	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if data.Len() > 0 {
				var envelope g.EventEnvelope
				if err := json.Unmarshal([]byte(data.String()), &envelope); err != nil {
					return fmt.Errorf("kata event stream data: %w", err)
				}
				cursor := envelope.EventID
				if id != "" {
					cursor, err = strconv.ParseInt(id, 10, 64)
					if err != nil {
						return fmt.Errorf("kata event stream cursor: %w", err)
					}
				}
				if eventType == "" {
					eventType = envelope.Type
				}
				if !send(model.StreamEvent{Generation: cmd.Generation, ID: cursor, Type: eventType}) {
					return ctx.Err()
				}
				if eventType == "sync.reset_required" {
					return nil
				}
			}
			id, eventType = "", ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			id = value
		case "event":
			eventType = value
		case "data":
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
}
