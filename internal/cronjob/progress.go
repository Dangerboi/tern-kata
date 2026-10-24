package cronjob

import "time"

// ProgressKeyPrefix starts the metadata key a Native job's delivery state lives under on its parent Issue: the prefix, then
// the job's uid.
const ProgressKeyPrefix = "cron.progress."

// Progress is a Native job's delivery state, kept by kata's daemon. Occurrences are delivered strictly after Anchor, the job's
// creation; Through is how far delivery has got, the daemon's clock at its last pass, so it stops advancing while the job is
// paused.
type Progress struct{ Anchor, Through time.Time }

// ReadProgress reads the metadata value kept under ProgressKeyPrefix. Only the anchor and through members are read, which
// kata may extend; false when the value is not an object or either member is missing or not an RFC 3339 instant.
func ReadProgress(value any) (Progress, bool) {
	fields, ok := value.(map[string]any)
	if !ok {
		return Progress{}, false
	}
	anchor, ok := instant(fields["anchor"])
	if !ok {
		return Progress{}, false
	}
	through, ok := instant(fields["through"])
	if !ok {
		return Progress{}, false
	}
	return Progress{Anchor: anchor, Through: through}, true
}

func instant(v any) (time.Time, bool) {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}
