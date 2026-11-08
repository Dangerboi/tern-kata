package docbanksetup

import "testing"

func TestStatusNamesAnyLoopbackSpellingOfTheAddress(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"running, listening on 127.0.0.1:8080", true},
		{"running, listening on [::1]:8080", true},
		{"running, listening on localhost:8080", true},
		{"running, listening on [::1]:80801", false},
		{"running, listening on [::1]:9090", false},
	} {
		if got := statusNames(tc.status, "127.0.0.1:8080"); got != tc.want {
			t.Errorf("statusNames(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
