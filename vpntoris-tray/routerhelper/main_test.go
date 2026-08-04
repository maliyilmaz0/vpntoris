//go:build darwin

package main

import (
	"reflect"
	"testing"
)

func TestTun2SocksArgumentsUseLongFlags(t *testing.T) {
	expected := []string{"--device", "utun", "--proxy", "socks5://127.0.0.1:49152"}
	if actual := tun2SocksArguments("utun", 49152); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("unexpected tun2socks arguments: %#v", actual)
	}
}
