package netbackend

import "testing"

func TestNetworkAndMask(t *testing.T) {
	network, mask, err := NetworkAndMask("10.38.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	if network != "10.38.0.0" || mask != "255.255.0.0" {
		t.Fatalf("got %s %s", network, mask)
	}
	network, mask, err = NetworkAndMask("10.68.236.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if network != "10.68.236.0" || mask != "255.255.255.0" {
		t.Fatalf("got %s %s", network, mask)
	}
	if _, _, err := NetworkAndMask("0.0.0.0/0"); err == nil {
		t.Fatal("expected default route rejection")
	}
}
