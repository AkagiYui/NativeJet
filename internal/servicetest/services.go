package servicetest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/text/language"
	"nativejet/internal/model"
	"nativejet/internal/servicecatalog"
)

type verdict struct {
	state  model.ServiceAvailability
	region string
}

var unknown = verdict{state: model.ServiceUnknown}

type judge func(context.Context, *client, reply) (model.ServiceAvailability, string)

func single(check func(reply) verdict) judge {
	return func(_ context.Context, _ *client, r reply) (model.ServiceAvailability, string) {
		v := check(r)
		return v.state, v.region
	}
}

const (
	netflixLicensed1 = "https://www.netflix.com/title/81280792"
	netflixLicensed2 = "https://www.netflix.com/title/70143836"
	netflixOriginal  = "https://www.netflix.com/title/80018499"
	bilibiliPlayback = "https://api.bilibili.com/pgc/player/web/playurl?"
	bahamutHome      = "https://ani.gamer.com.tw/"
)

var nativeProbes = map[string]struct {
	address string
	limit   int64
	judge   judge
	cookies bool
}{
	"google":          {address: "https://www.google.com/generate_204", judge: reachable(http.StatusNoContent)},
	"github":          {address: "https://github.com/", judge: reachable(http.StatusOK)},
	"youtube-premium": {address: "https://www.youtube.com/premium?hl=en", limit: 256 << 10, judge: single(youtube)},
	"chatgpt":         {address: "https://api.openai.com/compliance/cookie_requirements", limit: 16 << 10, judge: chatGPT},
	"claude":          {address: "https://claude.ai/cdn-cgi/trace", limit: 4 << 10, judge: single(claude)},
	"gemini":          {address: "https://gemini.google.com/", limit: 2 << 20, judge: single(gemini)},
	"netflix":         {address: netflixLicensed1, judge: netflix},
	"disney":          {address: "https://www.disneyplus.com/", limit: 128 << 10, judge: single(disney)},
	"primevideo":      {address: "https://www.primevideo.com/", limit: 384 << 10, judge: single(primeVideo)},
	"spotify":         {address: "https://www.spotify.com/premium/", judge: single(spotify)},
	"tiktok":          {address: "https://www.tiktok.com/explore", limit: 256 << 10, judge: single(tiktok)},
	"discord":         {address: "https://discord.com/api/v9/experiments", judge: reachable(http.StatusOK)},
	"bilibili-cn": {
		address: bilibiliPlayback + "avid=82846771&qn=0&type=&otype=json&ep_id=307247&fourk=1&fnver=0&fnval=16&module=bangumi",
		limit:   1 << 20,
		judge:   single(bilibili),
	},
	"bilibili-hmt": {
		address: bilibiliPlayback + "avid=18281381&cid=29892777&qn=0&type=&otype=json&ep_id=183799&fourk=1&fnver=0&fnval=16&module=bangumi",
		limit:   1 << 20,
		judge:   single(bilibili),
	},
	"bahamut": {address: bahamutHome + "ajax/getdeviceid.php", limit: 4 << 10, judge: bahamut, cookies: true},
	"muse":    {address: "https://muse.ai/", limit: 128 << 10, judge: single(muse)},
}

var (
	youtubeRegion = regexp.MustCompile(`"(?:GL|countryCode|country_code)"\s*:\s*"([A-Z]{2})"`)
	geminiRegion  = regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`)
	disneyRegion  = regexp.MustCompile(`"region"\s*:\s*"([A-Z]{2})"`)
	primeRegion   = regexp.MustCompile(`"currentTerritory"\s*:\s*"([A-Z]{2})"`)
	tiktokRegion  = regexp.MustCompile(`"region"\s*:\s*"([A-Z]{2})"`)
	bahamutRegion = regexp.MustCompile(`data-geo="([A-Za-z]{2})"`)
)

func reachable(want int) judge {
	return single(func(r reply) verdict {
		if r.status == want {
			return verdict{state: model.ServiceAvailable}
		}
		return unknown
	})
}

func chatGPT(ctx context.Context, c *client, r reply) (model.ServiceAvailability, string) {
	state := model.ServiceUnknown
	switch {
	case bytes.Contains(bytes.ToLower(r.body), []byte("unsupported_country")):
		state = model.ServiceBlocked
	case r.status == http.StatusOK && !challengePage(r.body):
		state = model.ServiceAvailable
	}
	trace := c.get(ctx, "https://chatgpt.com/cdn-cgi/trace", 4<<10, false)
	return state, traceRegion(trace)
}

func claude(r reply) verdict {
	return aiRegion(traceRegion(r))
}

func gemini(r reply) verdict {
	if r.status != http.StatusOK || challengePage(r.body) {
		return unknown
	}
	region := bodyRegion(geminiRegion, r.body)
	entry, _ := servicecatalog.Find("gemini")
	return verdict{regionSupport(entry, region), region}
}

func aiRegion(code string) verdict {
	switch code {
	case "":
		return unknown
	case "CN", "HK", "MO", "RU", "BY", "IR", "KP", "CU", "SY":
		return verdict{model.ServiceBlocked, code}
	default:
		return verdict{model.ServiceAvailable, code}
	}
}

func netflix(ctx context.Context, c *client, r reply) (model.ServiceAvailability, string) {
	if r.status == http.StatusNotFound {
		r = c.get(ctx, netflixLicensed2, 0, false)
	}
	switch r.status {
	case http.StatusOK:
		return model.ServiceAvailable, pathRegion(r)
	case http.StatusNotFound:
		original := c.get(ctx, netflixOriginal, 0, false)
		if original.status == http.StatusOK {
			return model.ServicePartial, pathRegion(original)
		}
		if original.status == http.StatusNotFound {
			return model.ServiceBlocked, ""
		}
	}
	return model.ServiceUnknown, ""
}

func youtube(r reply) verdict {
	body := bytes.ToLower(r.body)
	region := bodyRegion(youtubeRegion, r.body)
	if bytes.Contains(body, []byte("premium is not available in your country")) ||
		bytes.Contains(body, []byte("premium is not available in your region")) {
		return verdict{model.ServiceBlocked, region}
	}
	if r.status == http.StatusOK && region != "" && !challengePage(r.body) {
		return verdict{model.ServiceAvailable, region}
	}
	return verdict{model.ServiceUnknown, region}
}

func disney(r reply) verdict {
	if strings.Contains(r.final.Path, "/unavailable") {
		return verdict{state: model.ServiceBlocked}
	}
	if r.status == http.StatusOK && !challengePage(r.body) {
		return verdict{model.ServiceAvailable, bodyRegion(disneyRegion, r.body)}
	}
	return unknown
}

func primeVideo(r reply) verdict {
	if bytes.Contains(r.body, []byte(`"isServiceRestricted":true`)) ||
		bytes.Contains(r.body, []byte(`"isServiceRestricted": true`)) {
		return verdict{state: model.ServiceBlocked}
	}
	return availableIn(r, bodyRegion(primeRegion, r.body))
}

func spotify(r reply) verdict {
	return availableIn(r, pathRegion(r))
}

func tiktok(r reply) verdict {
	return availableIn(r, bodyRegion(tiktokRegion, r.body))
}

func bilibili(r reply) verdict {
	var playback struct {
		Code *int `json:"code"`
	}
	if r.status != http.StatusOK || json.Unmarshal(r.body, &playback) != nil || playback.Code == nil {
		return unknown
	}
	switch *playback.Code {
	case 0:
		return verdict{state: model.ServiceAvailable}
	case -10403:
		return verdict{state: model.ServiceBlocked}
	default:
		return unknown
	}
}

func bahamut(ctx context.Context, c *client, r reply) (model.ServiceAvailability, string) {
	var device struct {
		ID string `json:"deviceid"`
	}
	if r.status != http.StatusOK || json.Unmarshal(r.body, &device) != nil || device.ID == "" {
		return model.ServiceUnknown, ""
	}
	query := url.Values{"adID": {"89422"}, "sn": {"37783"}, "device": {device.ID}}
	token := c.get(ctx, bahamutHome+"ajax/token.php?"+query.Encode(), 16<<10, false)
	state := bahamutToken(token)
	if state == model.ServiceUnknown {
		return state, ""
	}
	home := c.get(ctx, bahamutHome, 128<<10, false)
	if home.status == http.StatusOK {
		return state, bodyRegion(bahamutRegion, home.body)
	}
	return state, ""
}

func bahamutToken(r reply) model.ServiceAvailability {
	var token struct {
		AnimeSN json.Number `json:"animeSn"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if r.status == http.StatusUnavailableForLegalReasons {
		return model.ServiceBlocked
	}
	if r.status != http.StatusOK || json.Unmarshal(r.body, &token) != nil {
		return model.ServiceUnknown
	}
	if strings.Contains(token.Error.Message, "地區") || strings.Contains(token.Error.Message, "地区") {
		return model.ServiceBlocked
	}
	if sn, err := token.AnimeSN.Int64(); err == nil && sn > 0 {
		return model.ServiceAvailable
	}
	return model.ServiceUnknown
}

func muse(r reply) verdict {
	if r.final.Hostname() == "auth.muse.ai" || challengePage(r.body) ||
		bytes.Contains(r.body, []byte("static.facebook.com/images/logos")) ||
		bytes.Contains(r.body, []byte("Sorry, something went wrong")) {
		return unknown
	}
	if r.status == http.StatusOK {
		return verdict{state: model.ServiceAvailable}
	}
	if r.status == http.StatusUnavailableForLegalReasons {
		return verdict{state: model.ServiceBlocked}
	}
	return unknown
}

func availableIn(r reply, region string) verdict {
	if r.status == http.StatusOK && region != "" && !challengePage(r.body) {
		return verdict{model.ServiceAvailable, region}
	}
	return verdict{model.ServiceUnknown, region}
}

func challengePage(body []byte) bool {
	return bytes.Contains(body, []byte("cf-chl-")) || bytes.Contains(body, []byte("/cdn-cgi/challenge-platform/"))
}

func traceRegion(r reply) string {
	if r.status != http.StatusOK {
		return ""
	}
	for line := range strings.Lines(string(r.body)) {
		if code, found := strings.CutPrefix(strings.TrimSpace(line), "loc="); found {
			return country(code)
		}
	}
	return ""
}

func bodyRegion(pattern *regexp.Regexp, body []byte) string {
	if match := pattern.FindSubmatch(body); match != nil {
		return country(string(match[1]))
	}
	return ""
}

func pathRegion(r reply) string {
	segment, _, _ := strings.Cut(strings.TrimPrefix(r.final.Path, "/"), "/")
	code, _, _ := strings.Cut(segment, "-")
	if len(code) != 2 {
		return ""
	}
	return country(code)
}

func country(code string) string {
	if region, err := language.ParseRegion(code); err == nil && region.IsCountry() {
		return region.String()
	}
	return ""
}
