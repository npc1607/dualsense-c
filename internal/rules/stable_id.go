package rules

import (
	"crypto/md5"
	"encoding/binary"
)

// StableRuleID derives a rule's 16-bit telemetry id from its name.
//
// The id is the first two bytes of md5(name) read as a little-endian uint16.
// Deriving the id from the name (instead of the rule's position in the
// compiled slice) keeps it stable across Engine.Reload calls, which re-sort
// rules by priority, and across daemon restarts. Clients can therefore key
// selection state, history buffers and recordings by rule_id without having
// to resynchronise after a profile reload.
//
// The empty name still hashes deterministically (md5 of ""); callers should
// prefer named rules so that a profile reorder cannot change their identity.
func StableRuleID(name string) uint16 {
	sum := md5.Sum([]byte(name))
	return binary.LittleEndian.Uint16(sum[0:2])
}
