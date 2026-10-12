package data

import (
	_ "embed"
	"strings"
)

// windowsZones is CLDR's windowsZones table, territory 001 rows only: "Windows key<TAB>IANA" lines, "#" lines are comments
// (the first records the CLDR release). Refreshing it is a data change.
//
//go:embed windows_zones.txt
var windowsZones string

// windowsIANAZone maps a Windows time zone key name to its IANA zone; ok is false for an unknown key.
func windowsIANAZone(key string) (zone string, ok bool) {
	for line := range strings.Lines(windowsZones) {
		if line = strings.TrimRight(line, "\r\n"); line == "" || line[0] == '#' {
			continue
		}
		if name, iana, found := strings.Cut(line, "\t"); found && name == key {
			return iana, true
		}
	}
	return "", false
}
