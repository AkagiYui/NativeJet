package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ReadMemory closes the streaming endpoint as soon as it receives an RSS sample.
func (s *Supervisor) ReadMemory(ctx context.Context) (int64, error) {
	resp, err := s.openController(ctx, http.MethodGet, "/memory", nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("内核 API %s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 512), 4<<10)
	for sc.Scan() {
		var frame struct {
			Inuse int64 `json:"inuse"`
		}
		// Mihomo emits a zero on the first frame for chart initialization.
		if json.Unmarshal(sc.Bytes(), &frame) == nil && frame.Inuse > 0 {
			return frame.Inuse, nil
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("内核未返回内存采样")
}
