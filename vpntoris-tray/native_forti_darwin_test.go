//go:build darwin

package main

import "testing"

func TestParseInterfaceCounters(t *testing.T) {
	output := "Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll\nutun7 1380 <Link#24> 12 0 4096 8 0 2048 0\n"
	received, sent, err := parseInterfaceCounters(output, "utun7")
	if err != nil || received != 4096 || sent != 2048 {
		t.Fatalf("unexpected counters: %d %d %v", received, sent, err)
	}
}
