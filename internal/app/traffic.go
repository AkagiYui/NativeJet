package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

type trafficLabels struct {
	up, down string
}

func (a *App) trafficLabels() trafficLabels {
	if !a.coreRunning() || a.trafficClient != a.client || !a.traffic.Available {
		return trafficLabels{up: "—", down: "—"}
	}
	return trafficLabels{up: trafficRate(a.traffic.Up), down: trafficRate(a.traffic.Down)}
}

func trafficRate(bytesPerSecond int64) string {
	if bytesPerSecond == 0 {
		return "0 B/s"
	}
	return bytesString(bytesPerSecond) + "/s"
}

// Both surfaces share a live reading. Only a visible window needs the history.
func (a *App) pollTraffic(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var client *ipc.Client
		var pid int
		var history bool
		mygo.RunOnMain(func() {
			if a.quitting || a.installing || a.connecting != nil || !a.connected {
				return
			}
			history = a.win.IsVisible()
			if history || a.coreRunning() {
				client, pid = a.client, a.snapshot.Status.PID
			}
		})
		if client == nil {
			continue
		}
		var snapshot model.TrafficSnapshot
		probe, cancel := context.WithTimeout(ctx, time.Second)
		err := client.Call(probe, "traffic.snapshot", map[string]bool{"history": history}, &snapshot)
		cancel()
		if err != nil {
			snapshot = model.TrafficSnapshot{}
		}
		mygo.RunOnMain(func() {
			if a.quitting || a.installing || a.connecting != nil || client != a.client ||
				!a.connected || pid != a.snapshot.Status.PID {
				return
			}
			changed := a.trafficClient != client || a.traffic != snapshot.Traffic
			if a.trafficClient != client {
				a.trafficHistory = nil
			}
			showHistory := history && (len(a.trafficHistory) != 0 || len(snapshot.History) != 0)
			if history && err == nil {
				a.trafficHistory = snapshot.History
			}
			a.trafficClient, a.traffic = client, snapshot.Traffic
			if changed {
				a.updateTray()
			}
			if a.win.IsVisible() && (changed || showHistory) {
				a.win.Invalidate()
			}
		})
	}
}
