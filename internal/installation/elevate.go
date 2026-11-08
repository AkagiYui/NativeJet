package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"nativejet/internal/storage"
)

func (p *Prepared) ApproveKernel(id string) error {
	if id != "" && !digestPattern.MatchString(id) {
		return errors.New("无效内核标识")
	}
	path := filepath.Join(p.Directory, "payload.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var payload Payload
	if err = json.Unmarshal(data, &payload); err != nil {
		return err
	}
	payload.ApproveKernel = id
	data, err = json.Marshal(payload)
	if err != nil {
		return err
	}
	if err = storage.Atomic(path, data, 0600); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	p.Digest = hex.EncodeToString(sum[:])
	return nil
}
