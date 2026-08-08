//go:build darwin

package main

import "testing"

func TestParseDarwinInterfaceCounters(t *testing.T) {
	output := "Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll\n" +
		"utun12    1500   <Link#42>                       10     0       1000       20     0       2000     0\n"
	received, sent, err := parseDarwinInterfaceCounters(output, "utun12")
	if err != nil {
		t.Fatal(err)
	}
	if received != 1000 || sent != 2000 {
		t.Fatalf("received=%d sent=%d", received, sent)
	}
}
func TestParseDarwinInterfaceCountersRejectsOtherInterface(t *testing.T) {
	output := "utun11    1500   <Link#41>                       10     0       1000       20     0       2000     0\n"
	if _, _, err := parseDarwinInterfaceCounters(output, "utun12"); err == nil {
		t.Fatal("expected missing interface counters error")
	}
}
