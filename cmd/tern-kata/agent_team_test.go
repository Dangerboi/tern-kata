package main

import (
	"runtime"
	"testing"
)

func TestAgentSetsTheTeamVariablesOnlyWithAnAddressPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	script := `printf '%s|%s' "${TERN_KATA_COORDINATOR-unset}" "${TERN_KATA_COORDINATING-unset}"`
	inherited := []string{"TERN_KATA_COORDINATOR=stale/omp-1", "TERN_KATA_COORDINATING=1", "TERN_PANE=18494129176579"}
	for name, tc := range map[string]struct {
		flags []string
		want  string
	}{
		"child":                      {[]string{"--address-prefix", "dangerboi/omp", "--coordinator", "dangerboi/omp-7"}, "dangerboi/omp-7|unset"},
		"coordinator":                {[]string{"--address-prefix", "dangerboi/omp", "--coordinating"}, "unset|1"},
		"neither drops inherited":    {[]string{"--address-prefix", "dangerboi/omp"}, "unset|unset"},
		"no prefix leaves inherited": {nil, "stale/omp-1|1"},
	} {
		t.Run(name, func(t *testing.T) {
			args := append(append([]string{"agent", "--as", "claude"}, tc.flags...), "--ref", "demo#ab12", "--", "sh", "-c", script)
			out, status, stderr := runAgent(t, inherited, args...)
			if status != 0 || out != tc.want {
				t.Errorf("status %d, output %q, stderr %q; want %q", status, out, stderr, tc.want)
			}
		})
	}
}

func TestPlanAgentRefusesAValueForCoordinating(t *testing.T) {
	if plan, err := PlanLaunch([]string{"agent", "--coordinating=1", "--", "omp"}, func(string) (string, bool) { return "", false }, "v"); err == nil {
		t.Errorf("--coordinating=1 was accepted: %+v", plan)
	}
}
