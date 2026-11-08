package data

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type calendar struct {
	Now  func() time.Time
	Env  func(string) string
	Zone func() string
}

var systemCalendar = calendar{Now: time.Now, Env: os.Getenv, Zone: osZone}

// Today returns the date and zone name; note is non-empty when the zone is unknown or unloadable and UTC stands in.
func (c calendar) Today() (date, zone, note string) {
	zone = strings.TrimPrefix(c.Env("TZ"), ":")
	if strings.HasPrefix(zone, "/") {
		// An absolute TZ is a zone file, which LoadLocation cannot load: its name is what follows zoneinfo/, else the system zone.
		zone = zoneFromPath(zone)
	}
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

func zoneFromPath(path string) string {
	_, zone, _ := strings.Cut(path, "zoneinfo/")
	return zone
}
