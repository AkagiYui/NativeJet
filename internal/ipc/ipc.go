// Package ipc is the versioned App ↔ helper protocol. It uses an authenticated
// Unix domain socket on macOS; the domain methods do not depend on transport.
package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"nativejet/internal/model"
)

type Request struct {
	Version int             `json:"version"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type Response struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}
type Client struct {
	Socket      string
	Development bool
	TeamID      string
	Requirement string
}

func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	data, err := json.Marshal(Request{Version: model.ProtocolVersion, Method: method, Params: p})
	if err != nil {
		return err
	}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return c.dial(ctx)
	}}
	defer transport.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://helper/v1", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("后台服务 HTTP %s", resp.Status)
	}
	limit := int64(24 << 20)
	if method == "logs" {
		// 4 MiB of log messages can expand sixfold in JSON. Leave room for entry metadata.
		limit = 26 << 20
	}
	var envelope Response
	if err = json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Error != "" {
		return errors.New(envelope.Error)
	}
	if out != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}
