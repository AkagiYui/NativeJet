package app

import "nativejet/internal/platform"

func serviceAction(action string) (int, error)       { return platform.Service(action) }
func signingRequirement(path string) (string, error) { return platform.SigningRequirement(path) }
