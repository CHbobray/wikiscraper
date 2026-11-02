//go:build !windows

package main

// ownsConsoleWindow is always false outside Windows: macOS and Linux programs
// run inside a terminal the user opened, which stays open after they exit.
func ownsConsoleWindow() bool {
	return false
}
