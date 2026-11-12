package kernel

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"runtime"
	"strings"
	"testing"
)

func universalFixture(t *testing.T, images ...[]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint32(macho.MagicFat))
	_ = binary.Write(&out, binary.BigEndian, uint32(len(images)))
	offset := uint32(4096)
	for _, image := range images {
		f, err := macho.NewFile(bytes.NewReader(image))
		if err != nil {
			t.Fatal(err)
		}
		header := macho.FatArchHeader{Cpu: f.Cpu, SubCpu: f.SubCpu, Offset: offset, Size: uint32(len(image)), Align: 12}
		if err := binary.Write(&out, binary.BigEndian, header); err != nil {
			t.Fatal(err)
		}
		offset = (offset + uint32(len(image)) + 4095) &^ 4095
	}
	for _, image := range images {
		out.Write(make([]byte, (4096-out.Len()%4096)%4096))
		out.Write(image)
	}
	return out.Bytes()
}

func TestUniversalKernelInspectsNativeImage(t *testing.T) {
	other := "amd64"
	if runtime.GOARCH == "amd64" {
		other = "arm64"
	}
	foreign := buildFixture(t, other, "github.com/MetaCubeX/mihomo")
	native := buildFixture(t, runtime.GOARCH, "github.com/MetaCubeX/mihomo")
	// Keep the image structurally valid, but identify its main package as another
	// project. The foreign slice must never supply the native slice's identity.
	unrelated := bytes.ReplaceAll(native, []byte("github.com/MetaCubeX/mihomo"), []byte("github.com/untrusted/otherx"))
	if len(unrelated) != len(native) || bytes.Equal(unrelated, native) {
		t.Fatal("unrelated metadata must preserve executable offsets")
	}
	for _, tc := range []struct {
		name  string
		image []byte
		valid bool
	}{
		{"native slice second", universalFixture(t, foreign, native), true},
		{"foreign metadata cannot identify native slice", universalFixture(t, foreign, unrelated), false},
		{"foreign architecture only", universalFixture(t, foreign), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager(t)
			putCandidate(t, m, Executable, tc.image)
			items, err := m.Scan(context.Background())
			if err != nil || len(items) != 1 || (items[0].Error == "") != tc.valid {
				t.Fatalf("universal identification: %+v, %v", items, err)
			}
			if !tc.valid && !strings.Contains(items[0].Error, "架构") && !strings.Contains(items[0].Error, "构建信息") {
				t.Fatalf("unexpected rejection: %+v", items[0])
			}
			assertNeverExecuted(t, m)
		})
	}
}
