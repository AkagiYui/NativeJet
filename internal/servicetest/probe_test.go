package servicetest

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"

	"nativejet/internal/model"
	"nativejet/internal/servicecatalog"
)

func TestProbeUsesProxyAndDoesNotRequireRoute(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/test" {
			t.Errorf("unexpected probe path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, "loc=SG\n")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	sources := make(chan netip.AddrPort, 2)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != target.Host {
			t.Errorf("unexpected proxy request: %s %s", r.Method, r.Host)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sources <- netip.MustParseAddrPort(r.RemoteAddr)
		remote, err := net.Dial("tcp", target.Host)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer remote.Close()
		local, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack CONNECT: %v", err)
			return
		}
		defer local.Close()
		_, _ = io.WriteString(local, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(remote, rw)
			_ = remote.(*net.TCPConn).CloseWrite()
			close(done)
		}()
		_, _ = io.Copy(local, remote)
		_ = local.Close()
		<-done
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	transport := upstream.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	transport.DisableKeepAlives = true
	defer transport.CloseIdleConnections()
	c := &client{http: &http.Client{Transport: transport}}
	chain := []string{"AI", "Singapore"}
	c.route = func(_ context.Context, source netip.AddrPort) []string {
		select {
		case got := <-sources:
			if got != source {
				t.Errorf("lookup used %s, proxy saw %s", source, got)
			}
		default:
			t.Error("route lookup ran without a proxy connection")
		}
		return chain
	}
	first := c.get(t.Context(), upstream.URL+"/test", 4096, true)
	if traceRegion(first) != "SG" || first.ms < 1 || !slices.Equal(first.route, chain) {
		t.Fatalf("probe through the proxy lost response or route: %+v", first)
	}
	c.route = nil
	second := c.get(t.Context(), upstream.URL+"/test", 4096, true)
	if traceRegion(second) != "SG" || second.ms < 1 || len(second.route) != 0 || len(sources) != 1 {
		t.Fatalf("probe without a controller lost its response: %+v", second)
	}
}

func TestBatchKeepsFailedProxyResultsUnknown(t *testing.T) {
	var attempts atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	results := Run(t.Context(), proxyURL, nil, servicecatalog.All())
	if len(results) != 159 || attempts.Load() != 159 {
		t.Fatalf("got %d results from %d proxy attempts", len(results), attempts.Load())
	}
	for _, result := range results {
		if result.Availability != model.ServiceUnknown || result.ElapsedMS != 0 || result.Region != "" || len(result.Route) != 0 {
			t.Errorf("proxy failure was reported as a service result: %+v", result)
		}
	}
}

func TestRegionalChecksDistinguishRestrictionsFromUnknown(t *testing.T) {
	if got := bilibili(reply{status: 200, body: []byte(`{"message":"busy"}`)}); got.state != model.ServiceUnknown {
		t.Fatalf("missing playback code was interpreted as %s", got.state)
	}
	if got := bilibili(reply{status: 200, body: []byte(`{"code":-10403}`)}); got.state != model.ServiceBlocked {
		t.Fatalf("explicit playback restriction was interpreted as %s", got.state)
	}
	if got := bahamutToken(reply{status: 403, body: []byte(`<html>Access denied</html>`)}); got != model.ServiceUnknown {
		t.Fatalf("ambiguous access denial was interpreted as %s", got)
	}
	if got := bahamutToken(reply{status: 200, body: []byte(`{"animeSn":"37783"}`)}); got != model.ServiceAvailable {
		t.Fatalf("valid playback token was interpreted as %s", got)
	}
	if got := muse(reply{status: 200, final: &url.URL{Host: "auth.muse.ai"}}); got.state != model.ServiceUnknown {
		t.Fatalf("authentication challenge was interpreted as %s", got.state)
	}
}

func TestExtendedServiceUsesOnlyItsOwnEvidence(t *testing.T) {
	geminiEntry, _ := servicecatalog.Find("gemini")
	result := model.ServiceTestResult{Availability: model.ServiceUnknown}
	setRegionEvidence(&result, geminiEntry, "HK")
	if result.Availability != model.ServiceUnknown || result.RegionSupport != model.ServiceAvailable || result.Evidence != "region" {
		t.Fatalf("Gemini's HK region check was reported as %+v", result)
	}
	dazn, _ := servicecatalog.Find("dazn")
	if state, cc := readServiceResponse(dazn, reply{status: 200, body: []byte(`{"Region":{"isAllowed":true,"GeolocatedCountry":"JP"}}`)}); state != model.ServiceAvailable || cc != "JP" {
		t.Fatalf("DAZN's nested result was reported as %s %s", state, cc)
	}
	bbc, _ := servicecatalog.Find("bbc")
	if state, _ := readServiceResponse(bbc, reply{status: 200, body: []byte(`unknownCallback({"media":[]});`)}); state != model.ServiceUnknown {
		t.Fatalf("unexpected JSONP callback was reported as %s", state)
	}
}
