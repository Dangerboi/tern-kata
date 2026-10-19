package data

import (
	"errors"
	"fmt"
	"net/http"

	"go.kenn.io/kata/pkg/client"
)

// guardedAttempts bounds how often a Guarded write retries when the revision moves between reading the target and writing it.
const guardedAttempts = 3

func etag(revision int64) string { return fmt.Sprintf("\"rev-%d\"", revision) }

// guardedWrite runs attempt, which reads the target's current revision and writes against it, and runs it again when kata
// refuses the revision as stale; a revision still refused after guardedAttempts reads as a plain conflict. attempt returns kata's raw error;
// the result is the text the Pane shows.
func guardedWrite(attempt func() error) error {
	for n := 1; ; n++ {
		err := attempt()
		if client.StatusCode(err) == http.StatusPreconditionFailed && n < guardedAttempts {
			continue
		}
		if client.StatusCode(err) == http.StatusPreconditionFailed {
			return errors.New("The Recurrence kept changing while saving; nothing was written. Try again.")
		}
		return daemonError(err)
	}
}
