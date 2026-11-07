package app

import "nativejet/internal/platform"

func signingRequirement(path string) (string, error) { return platform.SigningRequirement(path) }
