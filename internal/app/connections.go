package app

import (
	"cmp"
	"math"
	"net"
	"path/filepath"
	"strings"
	"time"

	"nativejet/internal/model"
)

const (
	connectionHostColumn = iota
	connectionProcessColumn
	connectionChainColumn
	connectionDownloadColumn
	connectionUploadColumn
	connectionDownloadSpeedColumn
	connectionUploadSpeedColumn
	connectionRuleColumn
	connectionStartColumn
	connectionSourceColumn
	connectionDestinationColumn
	connectionTypeColumn
)

type connectionSpeed struct {
	upload, download int64
}

const maxClosedConnections = 500

func (a *App) acceptConnections(next *model.Connections, err error, at time.Time) {
	elapsed := at.Sub(a.connectionSampleAt)
	a.connectionSpeeds = nil
	if err == nil && next != nil {
		sampleSpeeds := a.connectionsError == nil && a.connections != nil &&
			!a.connectionSampleAt.IsZero() && elapsed > 0
		a.compareConnections(next.Connections, sampleSpeeds, elapsed)
	}
	a.connections, a.connectionsError, a.connectionSampleAt = next, err, at
}

func (a *App) compareConnections(next []model.Connection, sampleSpeeds bool, elapsed time.Duration) {
	// Keep the last successful snapshot through transient errors and core restarts.
	previous := a.lastActiveConnections
	a.lastActiveConnections = next
	if len(previous) == 0 {
		return
	}
	byID := make(map[string]int, len(previous))
	for i, conn := range previous {
		if conn.ID != "" {
			byID[conn.ID] = i
		}
	}
	if sampleSpeeds {
		a.connectionSpeeds = make(map[string]connectionSpeed)
	}
	for _, conn := range next {
		i, ok := byID[conn.ID]
		if !ok {
			continue
		}
		delete(byID, conn.ID)
		if !sampleSpeeds {
			continue
		}
		old := previous[i]
		if conn.Start != old.Start || old.Upload < 0 || old.Download < 0 ||
			conn.Upload < old.Upload || conn.Download < old.Download {
			continue
		}
		a.connectionSpeeds[conn.ID] = connectionSpeed{
			upload:   connectionBytesPerSecond(conn.Upload-old.Upload, elapsed),
			download: connectionBytesPerSecond(conn.Download-old.Download, elapsed),
		}
	}
	if len(byID) == 0 {
		return
	}

	closed := make([]model.Connection, 0, min(maxClosedConnections, len(byID)+len(a.closedConnections)))
	for _, conn := range previous {
		if _, missing := byID[conn.ID]; missing {
			closed = append(closed, conn)
			if len(closed) == maxClosedConnections {
				break
			}
		}
	}
	keep := min(len(a.closedConnections), maxClosedConnections-len(closed))
	a.closedConnections = append(closed, a.closedConnections[:keep]...)
}

func connectionBytesPerSecond(bytes int64, elapsed time.Duration) int64 {
	rate := math.Round(float64(bytes) / elapsed.Seconds())
	if rate >= math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(rate)
}

func (a *App) connectionSortKey(conn model.Connection, column int) tableSortKey {
	var text string
	switch column {
	case connectionHostColumn:
		text = connectionHost(conn)
	case connectionProcessColumn:
		text = connectionProcess(conn)
	case connectionChainColumn:
		text = strings.Join(conn.Chains, " → ")
	case connectionDownloadColumn:
		return tableSortKey{number: uint64(max(0, conn.Download))}
	case connectionUploadColumn:
		return tableSortKey{number: uint64(max(0, conn.Upload))}
	case connectionDownloadSpeedColumn:
		speed, ok := a.connectionSpeeds[conn.ID]
		return tableSortKey{number: uint64(speed.download), missing: !ok || a.connectionTab == 1}
	case connectionUploadSpeedColumn:
		speed, ok := a.connectionSpeeds[conn.ID]
		return tableSortKey{number: uint64(speed.upload), missing: !ok || a.connectionTab == 1}
	case connectionRuleColumn:
		text = connectionRule(conn)
	case connectionStartColumn:
		at, err := time.Parse(time.RFC3339Nano, conn.Start)
		return tableSortKey{at: at, missing: err != nil || at.IsZero()}
	case connectionSourceColumn:
		text = connectionAddress(conn.Metadata.SourceIP, conn.Metadata.SourcePort)
	case connectionDestinationColumn:
		text = connectionDestination(conn)
	case connectionTypeColumn:
		text = connectionType(conn)
	}
	return tableSortKey{text: text, missing: text == ""}
}

func (a *App) connectionCellText(conn model.Connection, column int) string {
	key := a.connectionSortKey(conn, column)
	if key.missing {
		return "—"
	}
	switch column {
	case connectionDownloadColumn, connectionUploadColumn:
		return bytesString(int64(key.number))
	case connectionDownloadSpeedColumn, connectionUploadSpeedColumn:
		return trafficRate(int64(key.number))
	case connectionStartColumn:
		return key.at.Local().Format(time.DateTime)
	default:
		return key.text
	}
}

func connectionAddress(host, port string) string {
	if host == "" || port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func connectionHost(conn model.Connection) string {
	m := conn.Metadata
	return connectionAddress(cmp.Or(m.Host, m.DestinationIP, m.RemoteDestination), m.DestinationPort)
}

func connectionDestination(conn model.Connection) string {
	m := conn.Metadata
	return connectionAddress(cmp.Or(m.DestinationIP, m.RemoteDestination), m.DestinationPort)
}

func connectionProcess(conn model.Connection) string {
	if conn.Metadata.Process != "" {
		return conn.Metadata.Process
	}
	if conn.Metadata.ProcessPath != "" {
		return filepath.Base(conn.Metadata.ProcessPath)
	}
	return ""
}

func connectionRule(conn model.Connection) string {
	if conn.Rule == "" || conn.RulePayload == "" {
		return cmp.Or(conn.Rule, conn.RulePayload)
	}
	return conn.Rule + " · " + conn.RulePayload
}

func connectionType(conn model.Connection) string {
	kind, network := conn.Metadata.Type, strings.ToUpper(conn.Metadata.Network)
	if kind == "" || network == "" {
		return cmp.Or(kind, network)
	}
	return kind + " (" + network + ")"
}

func connectionMatches(conn model.Connection, query string) bool {
	m := conn.Metadata
	fields := []string{
		connectionHost(conn), connectionDestination(conn), connectionAddress(m.SourceIP, m.SourcePort),
		connectionProcess(conn), m.ProcessPath, connectionRule(conn), connectionType(conn),
		strings.Join(conn.Chains, " "),
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, " ")), query)
}
