//go:build windows

package data

import "golang.org/x/sys/windows/registry"

// osZone maps the system time zone key name from the registry to an IANA zone; "" when it can't be read or isn't known.
func osZone() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\TimeZoneInformation`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	key, _, err := k.GetStringValue("TimeZoneKeyName")
	if err != nil {
		return ""
	}
	zone, _ := windowsIANAZone(key)
	return zone
}
