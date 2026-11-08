package model

import "time"

// GeoData reports controller settings and local files independently.
type GeoData struct {
	PID         int       `json:"pid"`
	DatMode     *bool     `json:"datMode,omitempty"`
	SourceError string    `json:"sourceError,omitempty"`
	FileError   string    `json:"fileError,omitempty"`
	Files       []GeoFile `json:"files"`
}

type GeoFile struct {
	Kind       string    `json:"kind"`
	URL        string    `json:"url,omitempty"`
	Name       string    `json:"name,omitempty"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
	Problem    string    `json:"problem,omitempty"`
}
