package worker

import (
	"crypto/rand"
	"crypto/sha256"
	"time"

	"github.com/oklog/ulid/v2"
)

// RunUID is a Run's uid for one occurrence of a job (ADR-0002): a ULID of the occurrence time's milliseconds and the
// first 10 bytes of SHA-256("tern-kata/run/v1\0" + job uid + "\0" + occurrence key), so a rebuilt schedule finds the
// same Run. The occurrence key is OccurrenceKey(t) for a scheduled one and "manual:<run uid>" for a manual run.
func RunUID(jobUID, occurrenceKey string, occurrence time.Time) string {
	sum := sha256.Sum256([]byte("tern-kata/run/v1\x00" + jobUID + "\x00" + occurrenceKey))
	var id ulid.ULID
	_ = id.SetTime(ulid.Timestamp(occurrence))
	copy(id[6:], sum[:10])
	return id.String()
}

// OccurrenceKey is the UTC RFC 3339 instant with nanoseconds that names a scheduled occurrence.
func OccurrenceKey(occurrence time.Time) string {
	return occurrence.UTC().Format(time.RFC3339Nano)
}

// newManualUID is a random Run uid for a run requested by hand, kept once the journal is saved.
func newManualUID(now time.Time) string {
	return ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()
}
