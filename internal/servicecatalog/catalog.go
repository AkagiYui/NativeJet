// Package servicecatalog holds the built-in service list shared by the app and helper.
package servicecatalog

import (
	_ "embed"
	"encoding/json"
	"slices"
)

type Entry struct {
	ID, Name, Site, Icon        string
	URL, Kind, Reader, Callback string
	Post, Allow, Deny           string
}

func (e Entry) IconID() string {
	if e.Icon != "" {
		return e.Icon
	}
	return e.ID
}

// The extended directory and region policies are adapted from unlock-check
// ad81c8e7fd7e92530ac7f88c29f7a152017b7343. See the third-party notices.
//
//go:embed services.json
var data []byte

var entries = func() []Entry {
	var catalog []Entry
	if err := json.Unmarshal(data, &catalog); err != nil {
		panic(err)
	}
	return catalog
}()

func All() []Entry { return slices.Clone(entries) }

func Find(id string) (Entry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}
