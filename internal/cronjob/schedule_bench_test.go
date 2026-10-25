package cronjob

import (
	"testing"
	"time"
)

// BenchmarkPerDay is the rate the Jobs Page works out for every enabled cron job that makes an Issue per run, on each draw.
func BenchmarkPerDay(b *testing.B) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, expression := range []string{"* * * * *", "*/5 * * * *", "0 * * * *", "0 9 * * 1-5"} {
		b.Run(expression, func(b *testing.B) {
			s, err := Cron(expression, "Europe/Amsterdam", now.AddDate(0, -1, 0))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				s.PerDay(now)
			}
		})
	}
}
