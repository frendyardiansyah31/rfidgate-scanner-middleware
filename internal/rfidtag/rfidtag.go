// Package rfidtag holds the single definition of how an RFID tag is normalised
// for comparison.
//
// The legacy gate compared `replace(label,'SN','') = replace(<scan>,'SN','')`,
// because a tag may or may not carry an "SN" prefix on either side. The Koha
// RfidGateApi plugin applies the same rule (item_rfid.rfid_norm), and the local
// mirror stores its keys in this form, so everything that touches a tag key
// (engine, enricher, mirror) must go through Normalize.
package rfidtag

import "strings"

// Normalize removes every "SN" from the tag, exactly like the legacy SQL
// replace(). If that would leave nothing (the tag is only "SN"s), the original
// tag is returned so a lookup key is never empty.
func Normalize(tag string) string {
	n := strings.ReplaceAll(tag, "SN", "")
	if n == "" {
		return tag
	}
	return n
}
