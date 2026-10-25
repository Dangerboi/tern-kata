package data

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

const (
	// maxErrorBody bounds the body read from a failed stream request.
	maxErrorBody = 64 << 10
	// maxEventLine and maxEventData bound one line and one event's data; kata's events are small envelopes.
	maxEventLine = 1 << 20
	maxEventData = 1 << 20
)

func (c *Client) ReadEvents(ctx context.Context, cmd model.Subscribe, send func(model.Msg) bool) error {
	response, err := c.StreamEventsRaw(ctx, &g.StreamEventsRequestOptions{Query: &g.StreamEventsQuery{AfterID: &cmd.AfterID}})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
		if err != nil {
			return err
		}
		// The raw stream response is the one place no decoded error reaches us.
		var envelope g.ErrorEnvelope
		if json.Unmarshal(body, &envelope) != nil {
			return fmt.Errorf("kata event stream: HTTP %d", response.StatusCode)
		}
		envelope.Status = int64(response.StatusCode)
		return daemonError(envelope)
	}
	if !send(model.StreamOpened{Generation: cmd.Generation}) {
		return ctx.Err()
	}
	reader := bufio.NewReader(response.Body)
	var id, eventType string
	var data strings.Builder
	for {
		line, err := readStreamLine(reader)
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
			if data.Len()+len(value)+1 > maxEventData {
				return fmt.Errorf("kata event stream: one event carries more than %d bytes of data; the stream was dropped", maxEventData)
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
}

// readStreamLine reads one line of the stream, newline included, and fails once it is longer than maxEventLine: a daemon that
// never ends a line would otherwise grow the buffer without bound.
func readStreamLine(reader *bufio.Reader) (string, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(line)+len(chunk) > maxEventLine {
			return "", fmt.Errorf("kata event stream: a line is longer than %d bytes; the stream was dropped", maxEventLine)
		}
		line = append(line, chunk...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return string(line), err
	}
}
