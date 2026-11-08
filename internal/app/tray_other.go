//go:build darwin || windows

package app

func desktopTrayAvailable() bool { return true }
