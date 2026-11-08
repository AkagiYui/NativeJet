// Package servicetest probes known public endpoints through a configured proxy.
package servicetest

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/publicsuffix"
	"nativejet/internal/model"
	"nativejet/internal/servicecatalog"
)

const batchTimeout = 85 * time.Second

type RouteLookup func(context.Context, netip.AddrPort) []string

type reply struct {
	status   int
	body     []byte
	final    *url.URL
	location *url.URL
	ms       int64
	route    []string
}

type client struct {
	http  *http.Client
	route RouteLookup
}

// Run checks the built-in services in parallel, keeping their display order.
// A nil proxy produces unknown results without making network requests.
func Run(ctx context.Context, proxy *url.URL, route RouteLookup, entries []servicecatalog.Entry) []model.ServiceTestResult {
	results := make([]model.ServiceTestResult, len(entries))
	for i, entry := range entries {
		results[i] = model.ServiceTestResult{ID: entry.ID, Name: entry.Name, Availability: model.ServiceUnknown}
	}
	if proxy == nil {
		return results
	}
	ctx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()
	jobs := make(chan int, len(entries))
	for i := range entries {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for range min(24, len(entries)) {
		wg.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				checkService(ctx, proxy, route, entries[i], &results[i])
			}
		})
	}
	wg.Wait()
	return results
}

func checkService(ctx context.Context, proxy *url.URL, route RouteLookup, entry servicecatalog.Entry, result *model.ServiceTestResult) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// An isolated pool keeps this service's socket identifiable in /connections.
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), DialContext: dialer.DialContext, IdleConnTimeout: 2 * time.Second}
	defer transport.CloseIdleConnections()
	c := &client{http: &http.Client{Transport: transport, CheckRedirect: serviceRedirect}, route: route}
	if probe, ok := nativeProbes[entry.ID]; ok {
		if probe.cookies {
			c.http.Jar, _ = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
		}
		first := c.get(ctx, probe.address, probe.limit, true)
		result.ElapsedMS, result.Route = first.ms, first.route
		if first.status != 0 {
			result.Availability, result.Region = probe.judge(ctx, c, first)
			result.Evidence = "response"
		}
		switch entry.ID {
		case "gemini", "claude":
			result.RegionSupport, result.Availability, result.Evidence = result.Availability, model.ServiceUnknown, "region"
		case "bahamut":
			if first.status != 0 && result.Availability == model.ServiceUnknown {
				trace := c.get(ctx, bahamutHome+"cdn-cgi/trace", 4<<10, false)
				setRegionEvidence(result, entry, traceRegion(trace))
			}
		case "google", "github", "discord":
			if result.Availability == model.ServiceAvailable {
				result.Availability, result.Evidence = model.ServiceReachable, "reach"
			}
		}
		return
	}
	c.checkExtended(ctx, entry, result)
}

// Follow ordinary site redirects, but keep a probe on the service's own domain.
func serviceRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") {
		return http.ErrUseLastResponse
	}
	original, err := publicsuffix.EffectiveTLDPlusOne(via[0].URL.Hostname())
	target, targetErr := publicsuffix.EffectiveTLDPlusOne(req.URL.Hostname())
	if err != nil || targetErr != nil || target != original {
		return http.ErrUseLastResponse
	}
	return nil
}

func (c *client) get(ctx context.Context, address string, limit int64, first bool) reply {
	return c.fetch(ctx, address, "", limit, first)
}

func (c *client) fetch(ctx context.Context, address, post string, limit int64, first bool) reply {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var local atomic.Pointer[net.TCPAddr]
	if first && c.route != nil {
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			addr, _ := info.Conn.LocalAddr().(*net.TCPAddr)
			local.Store(addr)
		}})
	}
	method := http.MethodGet
	var body io.Reader
	if post != "" {
		method, body = http.MethodPost, strings.NewReader(post)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return reply{}
	}
	if post != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return reply{}
	}
	defer resp.Body.Close()
	out := reply{status: resp.StatusCode, final: resp.Request.URL, ms: max(1, time.Since(start).Milliseconds())}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		out.location, _ = resp.Location()
	}
	// Read the chain while the connection is still open. A missing or slow
	// controller never invalidates the service's own response.
	if addr := local.Load(); addr != nil {
		routeCtx, stop := context.WithTimeout(ctx, 400*time.Millisecond)
		out.route = c.route(routeCtx, addr.AddrPort())
		stop()
	}
	if limit > 0 {
		out.body, _ = io.ReadAll(io.LimitReader(resp.Body, limit))
	}
	return out
}
