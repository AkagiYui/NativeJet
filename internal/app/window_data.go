package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

const inactivePageInterval = 10 * time.Second

type windowQuery struct{ method, section string }

type windowRead struct {
	started time.Time
	pending bool
}

type windowReads struct {
	client *ipc.Client
	reads  map[windowQuery]windowRead
}

func (a *App) resetWindowData() {
	a.windowReads = nil
	a.traffic = model.Traffic{}
	a.trafficHistory = nil
	a.acceptConnections(nil, nil, time.Time{})
	a.groups, a.proxiesError = nil, nil
	clear(a.metrics[:])
	a.geoData = nil
	a.resetServiceTests()
	a.resetProxyTests()
}

func (a *App) refreshWindowData(preload bool) {
	if !a.canReadWindowData() {
		return
	}
	if !a.paused {
		var logs []model.Log
		a.readWindowData(windowQuery{method: "logs"}, 0, map[string]uint64{"after": a.logAfter}, &logs, func(err error) {
			if err == nil && !a.paused {
				a.appendLogs(logs)
			}
		})
	}
	interval := func(active bool) time.Duration {
		if preload || active {
			return 0
		}
		return inactivePageInterval
	}
	if a.snapshot.Status.GeoData {
		result := &geoResult{client: a.client}
		a.readWindowData(windowQuery{method: "geo.status"}, interval(a.page == geoDataPage), nil, &result.data, func(err error) {
			if err != nil || result.data.PID == geoPID(a.snapshot.Status) {
				result.err = err
				a.geoData = result
			}
		})
	}
	if a.coreRunning() {
		var connections model.Connections
		a.readWindowData(windowQuery{method: "connections"}, interval(a.page == "连接"), nil, &connections, func(err error) {
			a.acceptConnections(&connections, err, time.Now())
		})
		var groups []model.Proxy
		a.readWindowData(windowQuery{method: "proxies"}, interval(a.page == proxiesPage), nil, &groups, func(err error) {
			a.groups, a.proxiesError = groups, err
		})
		for tab, section := range metricSections {
			result := &metricsResult{}
			active := a.page == metricsPage && tab == a.metricsTab
			a.readWindowData(windowQuery{"metrics", section}, interval(active), map[string]string{"section": section}, &result.data, func(err error) {
				if err != nil || result.data.PID == a.snapshot.Status.PID {
					result.err = err
					a.metrics[tab] = result
				}
			})
		}
	}
	// Precompute the saved configuration. An open preview may also show edits.
	if a.page == "配置" && a.tab == 2 || !a.dirty() {
		a.refreshPreview(false)
	}
}

func (a *App) canReadWindowData() bool {
	return !a.quitting && a.connected && !a.installing && a.connecting == nil && a.windowVisible()
}

// Each request publishes independently so a slow controller endpoint cannot
// hold up the other pages or the helper snapshot.
func (a *App) readWindowData(query windowQuery, interval time.Duration, params, out any, accept func(error)) {
	client := a.client
	if a.windowReads == nil || a.windowReads.client != client {
		a.windowReads = &windowReads{client: client, reads: make(map[windowQuery]windowRead)}
	}
	reads := a.windowReads
	last := reads.reads[query]
	if last.pending || time.Since(last.started) < interval {
		return
	}
	reads.reads[query] = windowRead{started: time.Now(), pending: true}
	timeout := 5 * time.Second
	if query.method == "config.preview" {
		timeout = 2 * time.Minute
	}
	go func() {
		ctx, cancel := context.WithTimeout(a.windowContext, timeout)
		defer cancel()
		err := client.Call(ctx, query.method, params, out)
		mygo.RunOnMain(func() {
			read := reads.reads[query]
			read.pending = false
			reads.reads[query] = read
			if a.quitting || !a.connected || a.installing || a.connecting != nil || a.windowReads != reads || a.client != client {
				return
			}
			accept(err)
			if a.windowVisible() {
				a.win.Invalidate()
			}
		})
	}()
}
