package data

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// calendar is the local calendar the dated Collections need: today's date in the user's IANA zone. Env and Zone are injected so
// tests need neither the process environment nor a system zone; Zone is the OS zone source (see osZone).
type calendar struct {
	Now  func() time.Time
	Env  func(string) string
	Zone func() string
}

var systemCalendar = calendar{Now: time.Now, Env: os.Getenv, Zone: osZone}

// Today returns the date and zone name; note is non-empty when the zone is unknown or unloadable and UTC stands in.
func (c calendar) Today() (date, zone, note string) {
	zone = strings.TrimPrefix(c.Env("TZ"), ":")
	if zone == "" {
		zone = c.Zone()
	}
	if zone == "" {
		return c.Now().UTC().Format(time.DateOnly), "UTC", "time zone unknown, using UTC"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return c.Now().UTC().Format(time.DateOnly), "UTC", fmt.Sprintf("time zone %q not found, using UTC", zone)
	}
	return c.Now().In(location).Format(time.DateOnly), zone, ""
}
