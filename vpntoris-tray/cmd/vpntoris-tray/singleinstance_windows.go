//go:build windows

package main

func acquireSingleInstance() (func(), error) {
	return func() {}, nil
}
