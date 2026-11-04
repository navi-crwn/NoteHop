package app

import (
	"testing"
)

// A2-S2-18: the reporter's address is hashed with a secret kept by the server,
// so the stored value can't be reversed by trying every IPv4 address.
func TestReportAddressHashIsKeyed(t *testing.T) {
	hash := func() string {
		srv, _, d := hopApp(t, Config{})
		c := newClient(t, srv)
		n := c.create(nil)
		for i := 0; i < 2; i++ {
			if code, _, _ := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "spam"}, nil); code != 201 {
				t.Fatalf("report: %d", code)
			}
		}
		r1, _ := d.Report(1)
		r2, _ := d.Report(2)
		if r1.IPHash != r2.IPHash {
			t.Fatalf("one address, two hashes: %q %q", r1.IPHash, r2.IPHash)
		}
		return r1.IPHash
	}
	a, b := hash(), hash()
	if legacy := hashHex("report/127.0.0.1")[:32]; a == legacy || b == legacy {
		t.Fatal("ip_hash is still the unsalted sha256 of the address")
	}
	if a == b {
		t.Fatal("two installations hash the same address the same way")
	}
}
