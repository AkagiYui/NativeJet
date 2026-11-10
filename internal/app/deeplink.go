package app

import (
	"errors"
	"net/url"
	"strings"
)

type subscriptionLink struct {
	name, address string
}

func parseSubscriptionLink(raw string) (subscriptionLink, error) {
	link, err := url.Parse(raw)
	if err != nil || link.Scheme != "clash" || !strings.EqualFold(link.Host, "install-config") ||
		link.User != nil || (link.EscapedPath() != "" && link.EscapedPath() != "/") {
		return subscriptionLink{}, errors.New("仅支持 Clash 的 install-config 订阅导入链接")
	}
	query, err := url.ParseQuery(link.RawQuery)
	if err != nil || len(query["url"]) != 1 {
		return subscriptionLink{}, errors.New("Clash 导入链接须包含一个有效的订阅地址")
	}
	address := query.Get("url")
	subscription, err := parseWebSubscription(address)
	if err != nil {
		return subscriptionLink{}, err
	}
	name := strings.TrimSpace(query.Get("name"))
	if name == "" {
		name = subscription.Hostname()
	}
	return subscriptionLink{name: name, address: address}, nil
}

func parseWebSubscription(address string) (*url.URL, error) {
	subscription, err := url.Parse(address)
	if err != nil || subscription.Scheme != "https" || subscription.Hostname() == "" || subscription.User != nil {
		return nil, errors.New("Clash 链接导入需要有效的 HTTPS 订阅地址")
	}
	return subscription, nil
}

// OpenURL receives a macOS URL event on the main thread once Start has run.
func (a *App) OpenURL(raw string) {
	if a.quitting {
		return
	}
	a.showWindow()
	link, err := parseSubscriptionLink(raw)
	if err != nil {
		a.problem = err.Error()
		a.win.Invalidate()
		return
	}
	a.pendingImports = append(a.pendingImports, link)
	if a.importing != nil {
		return
	}
	a.showPendingImport()
}

func (a *App) showPendingImport() {
	if a.quitting || len(a.pendingImports) == 0 {
		return
	}
	link := a.pendingImports[0]
	a.pendingImports[0] = subscriptionLink{}
	a.pendingImports = a.pendingImports[1:]
	a.openImportForm(&link)
}
