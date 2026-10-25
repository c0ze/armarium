//go:build !darwin

package main

// Elsewhere a second launch finds the lock and opens the browser itself.
func listenForReopen(func()) {}
