package xray

import (
	"strconv"
	"testing"
)

func TestUserRecordIPDedup(t *testing.T) {
	u := &User{ID: 1}
	u.RecordIP("1.1.1.1")
	u.RecordIP("1.1.1.1")
	u.RecordIP("2.2.2.2")
	_, _, ips := u.snapshotAndReset()
	if len(ips) != 2 {
		t.Fatalf("want 2 distinct IPs, got %d (%v)", len(ips), ips)
	}
	if ips[0] != "1.1.1.1" || ips[1] != "2.2.2.2" {
		t.Fatalf("FIFO order broken: %v", ips)
	}
}

func TestUserRecordIPCap(t *testing.T) {
	u := &User{ID: 2}
	// Insert maxRecentIPsPerUser+3, expect oldest 3 dropped.
	total := maxRecentIPsPerUser + 3
	for i := 0; i < total; i++ {
		u.RecordIP("ip-" + strconv.Itoa(i))
	}
	_, _, ips := u.snapshotAndReset()
	if len(ips) != maxRecentIPsPerUser {
		t.Fatalf("want %d, got %d", maxRecentIPsPerUser, len(ips))
	}
	// Oldest survivor is index 3, last is total-1.
	if ips[0] != "ip-3" {
		t.Fatalf("want oldest survivor ip-3, got %s", ips[0])
	}
	if ips[len(ips)-1] != "ip-"+strconv.Itoa(total-1) {
		t.Fatalf("want newest ip-%d, got %s", total-1, ips[len(ips)-1])
	}
}

func TestUserSnapshotAndResetClearsIPs(t *testing.T) {
	u := &User{ID: 3}
	u.RecordIP("1.1.1.1")
	u.snapshotAndReset()
	_, _, ips := u.snapshotAndReset()
	if len(ips) != 0 {
		t.Fatalf("expected IPs cleared after first snapshot, got %v", ips)
	}
}

func TestUserRecordIPEmptyIgnored(t *testing.T) {
	u := &User{ID: 4}
	u.RecordIP("")
	_, _, ips := u.snapshotAndReset()
	if len(ips) != 0 {
		t.Fatalf("empty IP should be ignored, got %v", ips)
	}
}
