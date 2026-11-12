//go:build darwin || linux

package kernel

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"strings"
)

func unpack(compressed []byte, name string) ([]byte, string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, "", err
	}
	defer reader.Close()
	binary, err := io.ReadAll(io.LimitReader(reader, MaxBinary+1))
	if err != nil {
		return nil, "", err
	}
	if len(binary) > MaxBinary {
		return nil, "", errors.New("解压后的内核过大")
	}

	return binary, strings.TrimSuffix(name, ".gz"), nil
}
func (m *Manager) setDownloadOwner(f *os.File) error { return f.Chown(m.UID, m.GID) }

const executableMode = 0555
