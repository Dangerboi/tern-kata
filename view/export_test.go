package view

import "time"

// SetClock fixes the time relative "updated" cells are measured from.
func SetClock(f func() time.Time) { now = f }
