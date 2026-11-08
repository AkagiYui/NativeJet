package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"nativejet/internal/model"
)

var geoResources = [...]struct {
	kind   string
	apiKey string
	names  []string
}{
	{kind: "mmdb", apiKey: "mmdb", names: []string{"country.mmdb", "geoip.db", "geoip.metadb"}},
	{kind: "geoip", apiKey: "geo-ip", names: []string{"geoip.dat"}},
	{kind: "geosite", apiKey: "geo-site", names: []string{"geosite.dat"}},
	{kind: "asn", apiKey: "asn", names: []string{"asn.mmdb"}},
}

func (m *Manager) geoStatus(ctx context.Context) (model.GeoData, error) {
	var out model.GeoData
	var settings struct {
		Mode *bool             `json:"geodata-mode"`
		URLs map[string]string `json:"geox-url"`
	}
	if record := m.core.Record(); record != nil && m.core.Running() {
		out.PID = record.PID
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		data, err := m.core.Call(probe, http.MethodGet, "/configs", nil)
		cancel()
		if err == nil {
			err = json.Unmarshal(data, &settings)
		}
		if current := m.core.Record(); current == nil || *current != *record {
			return model.GeoData{}, errors.New("内核状态已变化，正在等待下一次读取")
		}
		if err != nil {
			out.SourceError = err.Error()
		} else {
			out.DatMode = settings.Mode
		}
	} else {
		out.SourceError = "内核未运行，启动后可读取生效的下载源。"
	}

	entries, err := os.ReadDir(m.runtime)
	if err != nil {
		out.FileError = err.Error()
	}
	for _, resource := range geoResources {
		file := geoFileStatus(entries, resource.names)
		file.Kind = resource.kind
		if out.SourceError == "" {
			file.URL = settings.URLs[resource.apiKey]
			if file.URL == "" {
				file.URL = settings.URLs[resource.kind]
			}
		}
		out.Files = append(out.Files, file)
	}
	return out, nil
}

func geoFileStatus(entries []os.DirEntry, names []string) model.GeoFile {
	// Mihomo checks aliases case-insensitively and takes the first matching
	// non-directory entry in the sorted ReadDir result.
	for _, entry := range entries {
		if entry.IsDir() || !slices.Contains(names, strings.ToLower(entry.Name())) {
			continue
		}
		file := model.GeoFile{Name: entry.Name()}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			file.Problem = "无法读取文件属性"
		} else if !info.Mode().IsRegular() {
			file.Problem = "非常规文件"
		} else {
			file.Size = info.Size()
			file.ModifiedAt = info.ModTime()
		}
		return file
	}
	return model.GeoFile{}
}

func (m *Manager) updateGeo(ctx context.Context, pid int) error {
	if !m.op.TryLock() {
		return errors.New("后台服务正在处理另一项操作，请稍后重试")
	}
	defer m.op.Unlock()
	if record := m.core.Record(); record == nil || !m.core.Running() {
		return errors.New("内核未运行")
	} else if record.PID != pid {
		return errors.New("内核已切换，请刷新 GeoData 状态后重试")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	_, err := m.core.Call(ctx, http.MethodPost, "/configs/geo", nil)
	return err
}
