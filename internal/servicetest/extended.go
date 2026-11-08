package servicetest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"nativejet/internal/model"
	"nativejet/internal/servicecatalog"
)

func (c *client) checkExtended(ctx context.Context, entry servicecatalog.Entry, result *model.ServiceTestResult) {
	address := entry.URL
	if entry.Reader == "mgtv" {
		address = strings.Replace(address, "{tk2}", url.QueryEscape(mangoToken()), 1)
	}
	var limit int64
	switch entry.Kind {
	case "region":
		limit = 4 << 10
	case "api", "jsonp":
		limit = 512 << 10
	}
	r := c.fetch(ctx, address, entry.Post, limit, true)
	result.ElapsedMS, result.Route = r.ms, r.route
	if r.status == 0 {
		return
	}
	result.Detail = fmt.Sprintf("HTTP %d", r.status)
	switch entry.Kind {
	case "reach":
		result.Availability, result.Evidence = model.ServiceReachable, "reach"
	case "region":
		setRegionEvidence(result, entry, traceRegion(r))
	case "redirect":
		result.Availability, result.Evidence = redirectVerdict(entry, r), "redirect"
	case "api", "jsonp":
		result.Availability, result.Region = readServiceResponse(entry, r)
		result.Evidence = "response"
	}
}

func regionSupport(entry servicecatalog.Entry, cc string) model.ServiceAvailability {
	if cc == "" || (entry.Allow == "" && entry.Deny == "") {
		return model.ServiceUnknown
	}
	if (entry.Allow != "" && !slices.Contains(strings.Fields(entry.Allow), cc)) ||
		slices.Contains(strings.Fields(entry.Deny), cc) {
		return model.ServiceBlocked
	}
	return model.ServiceAvailable
}

func setRegionEvidence(result *model.ServiceTestResult, entry servicecatalog.Entry, cc string) {
	if cc != "" {
		result.Region, result.RegionSupport, result.Evidence = cc, regionSupport(entry, cc), "region"
	}
}

func redirectVerdict(entry servicecatalog.Entry, r reply) model.ServiceAvailability {
	if r.final == nil {
		return model.ServiceUnknown
	}
	for _, target := range []*url.URL{r.final, r.location} {
		if target == nil {
			continue
		}
		path := strings.TrimSuffix(strings.ToLower(target.Path), "/")
		if strings.HasPrefix(path, "/unavailable") || strings.HasPrefix(path, "/restricted") ||
			strings.HasPrefix(path, "/ott/no-service") ||
			(entry.ID == "paramount" && (path == "/intl" || strings.HasPrefix(path, "/intl/"))) ||
			(entry.ID == "tubi" && target.Hostname() == "gdpr.tubi.tv") {
			return model.ServiceBlocked
		}
	}
	if r.status == http.StatusOK && (r.final.Path == "" || r.final.Path == "/" ||
		(entry.ID == "viu" && strings.HasPrefix(r.final.Path, "/ott/")) ||
		(entry.ID == "pandora" && strings.HasPrefix(r.final.Path, "/station"))) {
		return model.ServiceAvailable
	}
	return model.ServiceUnknown
}

func mangoToken() string {
	var device [16]byte
	_, _ = rand.Read(device[:])
	device[6] = device[6]&0x0f | 0x40
	device[8] = device[8]&0x3f | 0x80
	value := fmt.Sprintf("did=%x-%x-%x-%x-%x|pno=1030|ver=0.3.0301|clit=%d",
		device[:4], device[4:6], device[6:8], device[8:10], device[10:], time.Now().Unix())
	encoded := []byte(base64.URLEncoding.EncodeToString([]byte(value)))
	slices.Reverse(encoded)
	return string(encoded)
}

var (
	fodFlag         = regexp.MustCompile(`(?i)<flag\s+type="(true|false)"`)
	telasaOK        = regexp.MustCompile(`"type"\s*:\s*"OK"`)
	catchplayRegion = regexp.MustCompile(`(?i)location\s+([a-z]{2})\b`)
	daznAllowed     = regexp.MustCompile(`"isAllowed"\s*:\s*(true|false)`)
	daznRegion      = regexp.MustCompile(`(?i)"GeolocatedCountry"\s*:\s*"([a-z]{2})"`)
)

func readServiceResponse(entry servicecatalog.Entry, r reply) (model.ServiceAvailability, string) {
	body := strings.TrimSpace(string(r.body))
	if entry.Callback != "" {
		// JSONP is a response envelope. Parse its argument as data.
		var ok bool
		body = strings.TrimSpace(strings.TrimSuffix(body, ";"))
		body, ok = strings.CutPrefix(body, entry.Callback+"(")
		if !ok {
			return model.ServiceUnknown, ""
		}
		body, ok = strings.CutSuffix(body, ")")
		if !ok {
			return model.ServiceUnknown, ""
		}
	}
	lower := strings.ToLower(body)
	const yes, no, unknown = model.ServiceAvailable, model.ServiceBlocked, model.ServiceUnknown
	switch entry.Reader {
	case "dazn":
		if r.status == 403 && strings.Contains(lower, "security policy has been breached") {
			return no, ""
		}
		if m := daznAllowed.FindStringSubmatch(body); m != nil && json.Valid([]byte(body)) {
			cc := bodyRegion(daznRegion, r.body)
			if m[1] == "true" {
				return yes, cc
			}
			return no, cc
		}
	case "dmm":
		if strings.Contains(body, "FOREIGN") {
			return no, ""
		}
		if strings.Contains(body, "UNAUTHORIZED") {
			return yes, "JP"
		}
	case "shudder":
		if strings.Contains(lower, "not available") || strings.Contains(lower, "not yet available") {
			return no, ""
		}
		if r.status == 200 && strings.Contains(lower, "movies") && !challengePage(r.body) {
			return yes, ""
		}
	case "bein":
		if r.status == 451 {
			return no, ""
		}
		if r.status == 500 {
			return yes, ""
		}
	case "zdf":
		if r.status == 403 {
			return no, ""
		}
		if r.status == 404 {
			return yes, "DE"
		}
	case "fod":
		if m := fodFlag.FindStringSubmatch(body); m != nil {
			if strings.EqualFold(m[1], "true") {
				return yes, "JP"
			}
			return no, ""
		}
	case "itv":
		if r.status == 403 && strings.Contains(lower, "geoblock") {
			return no, ""
		}
		if r.status == 404 {
			return yes, "GB"
		}
	case "telasa":
		if strings.Contains(body, "IPLocationNotAllowed") {
			return no, ""
		}
		if telasaOK.MatchString(body) {
			return yes, "JP"
		}
	case "aetv":
		if strings.Contains(body, "GeoLocationBlocked") {
			return no, ""
		}
	default:
		return readServiceJSON(entry.Reader, body)
	}
	return unknown, ""
}

func readServiceJSON(reader, body string) (model.ServiceAvailability, string) {
	const yes, no, unknown = model.ServiceAvailable, model.ServiceBlocked, model.ServiceUnknown
	var obj map[string]any
	if json.Unmarshal([]byte(body), &obj) != nil || obj == nil {
		return unknown, ""
	}
	readBool := func(m map[string]any, key, cc string) (model.ServiceAvailability, string) {
		if allowed, ok := m[key].(bool); ok {
			if allowed {
				return yes, country(cc)
			}
			return no, country(cc)
		}
		return unknown, ""
	}
	switch reader {
	case "kktv":
		m := jsonObject(obj, "data")
		return readBool(m, "is_allowed", jsonString(m, "country"))
	case "philo":
		cc := country(jsonString(obj, "country"))
		if s := jsonString(obj, "status"); s == "SUCCESS" {
			return yes, cc
		} else if s != "" && cc != "" && cc != "US" {
			return no, cc
		}
	case "tvb":
		return readBool(obj, "allow_in_this_country", jsonString(obj, "country"))
	case "sbs":
		if cc := country(jsonString(jsonObject(jsonObject(obj, "get"), "response"), "country_code")); cc != "" {
			if cc == "AU" {
				return yes, cc
			}
			return no, cc
		}
	case "nbc":
		if restricted, ok := obj["restricted"].(bool); ok {
			cc := country(jsonString(jsonObject(obj, "requestInfo"), "countryCode"))
			if restricted {
				return no, cc
			}
			return yes, cc
		}
	case "mgtv":
		if obj["code"] == float64(200) && obj["data"] != nil {
			return yes, "CN"
		}
		message := strings.ToLower(jsonString(obj, "msg"))
		for _, term := range []string{"地区", "区域", "海外", "版权", "region", "country"} {
			if strings.Contains(message, term) {
				return no, ""
			}
		}
	case "qq":
		switch obj["em"] {
		case float64(0):
			return yes, "CN"
		case float64(80):
			return no, ""
		}
	case "ten", "molotov":
		key, cc := "allow", "AU"
		if reader == "molotov" {
			key, cc = "is_france", "FR"
		}
		if allowed, ok := obj[key].(bool); ok {
			if allowed {
				return yes, cc
			}
			return no, ""
		}
	case "nhk":
		if code := jsonString(obj, "country_code"); code != "" {
			if code == "JP" {
				return yes, "JP"
			}
			return no, country(code)
		}
	case "sling":
		cc := ""
		if strings.EqualFold(jsonString(obj, "country"), "USA") {
			cc = "US"
		}
		if allowed, ok := obj["playback_allowed"].(bool); ok {
			if allowed {
				return yes, cc
			}
			return no, cc
		}
		if denied, ok := obj["playback_disallowed"].(bool); ok {
			if denied {
				return no, cc
			}
			return yes, cc
		}
	case "catchplay":
		switch fmt.Sprint(obj["code"]) {
		case "100016":
			return no, bodyRegion(catchplayRegion, []byte(jsonString(obj, "message")))
		case "0":
			return yes, country(jsonString(jsonObject(obj, "data"), "isoCode"))
		}
	case "bbc":
		if obj["result"] == "geolocation" {
			return no, ""
		}
		if _, ok := obj["media"].([]any); ok {
			return yes, "GB"
		}
	}
	return unknown, ""
}

func jsonObject(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}

func jsonString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}
