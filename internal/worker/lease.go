package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The Project lease (ADR-0003): the `tern_kata.lease` key of the Project's metadata, written with a conditional write
// against the Project's revision. Another machine's live lease means this worker admits nothing for the Project.
const (
	leaseKey = "tern_kata.lease"
	// LeaseRenew is how often the owner renews; LeaseExpiry is how long an unrenewed lease lives.
	LeaseRenew  = 3 * time.Minute
	LeaseExpiry = 10 * time.Minute
)

type leaseValue struct {
	Version    int       `json:"version"`
	Owner      string    `json:"owner"`
	Generation int64     `json:"generation"`
	Heartbeat  time.Time `json:"heartbeat"`
}

// LeaseState is what a Project's lease allows this worker. Admission needs Held.
type LeaseState struct {
	Held       bool
	Generation int64
	// Owner is the executor holding a lease this worker may not take.
	Owner string
	// Unknown is set when the lease could not be read or written; the worker admits nothing and retries.
	Unknown bool
	Reason  string
}

// Why is the sentence `worker status` and a refused `run` show for a Project that admits nothing.
func (s LeaseState) Why() string {
	switch {
	case s.Held:
		return ""
	case s.Owner != "":
		return fmt.Sprintf("another worker (%s) owns this Project", s.Owner)
	case s.Reason != "":
		return s.Reason
	}
	return "the Project's lease has not been checked yet"
}

func leaseOf(p Project) (v leaseValue, present bool, err error) {
	raw, ok := p.Metadata[leaseKey]
	if !ok || raw == nil {
		return leaseValue{}, false, nil
	}
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &v); err != nil || v.Version == 0 { // an empty Owner is a lease its owner released
		return leaseValue{}, true, fmt.Errorf("the Project's %s metadata is not a lease", leaseKey)
	}
	return v, true, nil
}

// claimLease reads the Project's lease and takes or renews it when the rules allow: no lease, an expired one, or its
// own. takeover writes the next generation over another executor's live lease (ADR-0003 `--takeover`). A lost
// conditional write is read again and retried, as any writer of the metadata may have got in between.
func claimLease(ctx context.Context, k *Kata, clock Clock, executor, actor string, projectID int64, takeover bool) LeaseState {
	for attempt := 0; attempt < 3; attempt++ {
		p, err := k.Project(ctx, projectID)
		if err != nil {
			return LeaseState{Unknown: true, Reason: "the Project's lease could not be read: " + err.Error()}
		}
		cur, present, err := leaseOf(p)
		if err != nil {
			return LeaseState{Unknown: true, Reason: err.Error()}
		}
		now := clock.Now().UTC()
		own := present && cur.Owner == executor
		live := present && cur.Owner != "" && now.Sub(cur.Heartbeat) < LeaseExpiry
		if present && !own && live && !takeover {
			return LeaseState{Owner: cur.Owner, Generation: cur.Generation}
		}
		gen := cur.Generation
		switch {
		case !present:
			gen = 1
		case !own:
			gen++
		}
		value := leaseValue{Version: 1, Owner: executor, Generation: gen, Heartbeat: now}
		_, err = k.PatchProjectMetadata(ctx, projectID, p.Revision, actor, map[string]any{leaseKey: value})
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		if err != nil {
			return LeaseState{Unknown: true, Reason: "the Project's lease could not be written: " + err.Error()}
		}
		return LeaseState{Held: true, Generation: gen}
	}
	return LeaseState{Unknown: true, Reason: "the Project's metadata kept changing while taking its lease"}
}

// readLease is the lease as it stands, without writing, for `worker doctor`.
func readLease(ctx context.Context, k *Kata, clock Clock, projectID int64) (owner string, generation int64, live bool, err error) {
	p, err := k.Project(ctx, projectID)
	if err != nil {
		return "", 0, false, err
	}
	v, present, err := leaseOf(p)
	if err != nil || !present {
		return "", 0, false, err
	}
	return v.Owner, v.Generation, clock.Now().Sub(v.Heartbeat) < LeaseExpiry, nil
}

// releaseLease clears the owner of the Project's lease when it is this executor's, keeping the generation, so the next
// worker takes the next one at once (ADR-0003 "Retire"). A lease that is another executor's, or absent, is left alone.
func releaseLease(ctx context.Context, k *Kata, clock Clock, executor, actor string, projectID int64) error {
	for range 3 {
		p, err := k.Project(ctx, projectID)
		if err != nil {
			return err
		}
		cur, present, err := leaseOf(p)
		if err != nil || !present || cur.Owner != executor {
			return err
		}
		value := leaseValue{Version: 1, Generation: cur.Generation, Heartbeat: clock.Now().UTC()}
		_, err = k.PatchProjectMetadata(ctx, projectID, p.Revision, actor, map[string]any{leaseKey: value})
		if errors.Is(err, ErrRevisionConflict) {
			continue
		}
		return err
	}
	return errors.New("the Project's metadata kept changing while releasing its lease")
}
