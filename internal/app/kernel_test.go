package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"nativejet/internal/model"
)

func TestKernelViewSeparatesOriginDigestAndAuthorization(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.page, a.connected = "内核", true
	a.snapshot.Status = model.Status{Privileged: true, KernelAuthorized: true, AuthorizedKernels: []string{"another-file"}}
	a.snapshot.Kernels = []model.Kernel{{ID: strings.Repeat("a", 64), Name: "mihomo", Path: "/inbox/mihomo", Source: model.KernelSourceLocal}}
	tester := ui.NewTester(a.view, 1100, 1000)
	assertText := func(want ...string) {
		t.Helper()
		for _, text := range want {
			if !tester.HasText(text) {
				t.Errorf("kernel details missing %q: %v", text, tester.Texts())
			}
		}
	}
	assertText(model.KernelVersionUnknown, "本地导入 · 来源未验证", "未验证", "此文件指纹尚未授权", "文件指纹（SHA-256）")
	k := &a.snapshot.Kernels[0]
	k.Source, k.SourceURL = model.KernelSourceGitHub, "https://github.com/MetaCubeX/mihomo/releases/download/v1.2.3/mihomo.gz"
	k.DigestVerified, k.AssetDigest = true, "sha256:"+strings.Repeat("b", 64)
	k.Version = "Mihomo v1.2.3（文件名）"
	a.snapshot.Status.AuthorizedKernels = []string{k.ID}
	tester.Frame()
	assertText("应用内下载 · MetaCubeX/mihomo", "下载时与 GitHub 发布的 SHA-256 匹配", "此文件指纹已获管理员授权", k.Version)
	k.DigestVerified = false
	tester.Frame()
	assertText("未验证 · 下载时未取得发布摘要")
}
