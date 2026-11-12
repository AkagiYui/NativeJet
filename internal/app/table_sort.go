package app

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/logging"
	"nativejet/internal/model"
)

type tableSortKey struct {
	text    string
	number  uint64
	at      time.Time
	missing bool
}

func sortableTableRows[T any](rows []T, columns []ui.TableColumn, order ui.SortOrder, key func(T, int) tableSortKey) []T {
	column := -1
	for i := range columns {
		columns[i].Sortable = true
		id := columns[i].ID
		if id == "" {
			id = columns[i].Title
		}
		if id == order.Column {
			column = i
		}
	}
	if column < 0 || len(rows) < 2 {
		return rows
	}
	type indexedKey struct {
		index int
		key   tableSortKey
	}
	keys := make([]indexedKey, len(rows))
	for i, row := range rows {
		keys[i] = indexedKey{index: i, key: key(row, column)}
	}
	slices.SortStableFunc(keys, func(a, b indexedKey) int {
		left, right := a.key, b.key
		if left.missing != right.missing {
			if left.missing {
				return 1
			}
			return -1
		}
		result := cmp.Or(cmp.Compare(left.text, right.text), cmp.Compare(left.number, right.number), left.at.Compare(right.at))
		if order.Descending {
			return -result
		}
		return result
	})
	sorted := make([]T, len(rows))
	for i, entry := range keys {
		sorted[i] = rows[entry.index]
	}
	return sorted
}

// Resolve the old selection against the full new order before building the table.
func followTableSelection[T any](state *ui.ListState, rows []T, key func(T) string) {
	if previous := state.Key; previous != nil && *state.Selected >= 0 {
		selected := previous(*state.Selected)
		*state.Selected = slices.IndexFunc(rows, func(row T) bool { return key(row) == selected })
	}
	state.Key = func(row int) any {
		if row < 0 || row >= len(rows) {
			return nil
		}
		return key(rows[row])
	}
}

var kernelReleaseVersion = regexp.MustCompile(`^[vV]?([0-9]+)\.([0-9]+)(?:\.([0-9]+))?(.*)$`)

func kernelSortKey(k model.Kernel, column int, selected string) tableSortKey {
	switch column {
	case 0:
		return tableSortKey{text: k.Name}
	case 1:
		version := kernelVersionLabel(k.Version)
		return tableSortKey{text: kernelVersionOrder(version), missing: version == model.KernelVersionUnknown}
	case 2:
		return tableSortKey{text: k.Arch, missing: k.Arch == ""}
	default:
		return tableSortKey{text: kernelStateLabel(k, selected)}
	}
}

func kernelVersionOrder(version string) string {
	parts := kernelReleaseVersion.FindStringSubmatch(version)
	if parts == nil {
		return version
	}
	var key strings.Builder
	for _, part := range parts[1:4] {
		digits := strings.TrimLeft(part, "0")
		fmt.Fprintf(&key, "%03d%s", len(digits), digits)
	}
	suffix, _, _ := strings.Cut(parts[4], "+")
	if suffix == "" {
		key.WriteByte('1')
	} else {
		key.WriteByte('0')
		key.WriteString(suffix)
	}
	return key.String()
}

func logSortKey(entry model.Log, column int) tableSortKey {
	switch column {
	case 0:
		return tableSortKey{at: entry.Time, missing: entry.Time.IsZero()}
	case 1:
		switch entry.Level {
		case "debug":
			return tableSortKey{number: 0}
		case "info":
			return tableSortKey{number: 1}
		case "warning":
			return tableSortKey{number: 2}
		case "error":
			return tableSortKey{number: 3}
		default:
			return tableSortKey{text: entry.Level, missing: true}
		}
	case 2:
		return tableSortKey{text: logSourceLabel(entry.Source), missing: entry.Source == ""}
	default:
		_, message := logging.Parse(entry.Message)
		return tableSortKey{text: message, missing: message == ""}
	}
}
