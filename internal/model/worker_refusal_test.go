package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The run-now refusal ends "Cannot run "x" now: <reason>." in one sentence, whatever state the worker banner is in.
func TestWorkerBannerRefusalReadsAsOneSentenceClause(t *testing.T) {
	for _, tc := range []struct {
		name   string
		banner model.WorkerBanner
		want   string
	}{
		{"starting", model.WorkerBanner{Badge: "Starting", Pending: true, Detail: "the plugin starts one within 10 seconds"}, "the worker is starting"},
		{"unread", model.WorkerBanner{Failed: true, Detail: "Cannot read the worker: disk on fire."}, "the worker could not be read: disk on fire"},
		{"none", model.WorkerBanner{Badge: "No worker", Detail: "none set up for kata"}, "no worker: none set up for kata"},
		{"not admitting", model.WorkerBanner{Badge: "Worker omp", Detail: "admits nothing: paused"}, "the worker admits nothing: paused"},
	} {
		if got := tc.banner.Refusal(); got != tc.want {
			t.Errorf("%s: Refusal() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
