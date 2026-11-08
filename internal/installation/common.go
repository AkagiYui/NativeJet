package installation

import "regexp"

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
