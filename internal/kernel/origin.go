package kernel

import (
	"encoding/json"
	"net/url"
	"strings"

	"nativejet/internal/model"
)

type downloadReceipt struct {
	SHA256         string `json:"sha256"`
	Version        string `json:"version"`
	AssetID        int64  `json:"assetId"`
	URL            string `json:"url"`
	AssetDigest    string `json:"assetDigest,omitempty"`
	ArchiveSHA256  string `json:"archiveSha256"`
	DigestVerified bool   `json:"digestVerified"`
}

func officialDownloadURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil &&
		strings.HasPrefix(u.Path, "/MetaCubeX/mihomo/releases/download/")
}

func (m *Manager) saveReceipt(receipt downloadReceipt) error {
	root, err := managedRoot(m.Installed, true)
	if err != nil {
		return err
	}
	defer root.Close()
	receipts, err := managedChild(root, "receipts", true)
	if err != nil {
		return err
	}
	defer receipts.Close()
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return atomicWrite(receipts, receipt.SHA256+".json", data, 0600)
}

func (m *Manager) applyReceipt(item *model.Kernel) {
	root, err := managedRoot(m.Installed, false)
	if err != nil {
		return
	}
	defer root.Close()
	receipts, err := managedChild(root, "receipts", false)
	if err != nil {
		return
	}
	defer receipts.Close()
	data, _, err := readRegular(receipts, item.ID+".json", 64<<10)
	if err != nil {
		return
	}
	var receipt downloadReceipt
	if json.Unmarshal(data, &receipt) != nil || receipt.SHA256 != item.ID || receipt.AssetID <= 0 ||
		!officialDownloadURL(receipt.URL) || !validID(receipt.ArchiveSHA256) {
		return
	}
	item.Source = model.KernelSourceGitHub
	item.SourceURL = receipt.URL
	item.AssetDigest = receipt.AssetDigest
	item.DigestVerified = receipt.DigestVerified && receipt.AssetDigest == "sha256:"+receipt.ArchiveSHA256
	// A release tag is useful even for builds linked with Main.Version=(devel).
	if item.Version == UnknownVersion && receipt.Version != "" {
		item.Version = "Mihomo " + receipt.Version + "（下载记录）"
	}
}
