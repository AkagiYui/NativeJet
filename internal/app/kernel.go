package app

import (
	"slices"

	"nativejet/internal/model"
)

func kernelSourceLabel(k model.Kernel) string {
	if k.Source == model.KernelSourceGitHub {
		return "应用内下载 · MetaCubeX/mihomo"
	}
	return "本地导入 · 来源未验证"
}

func kernelDigestLabel(k model.Kernel) string {
	if k.Source == model.KernelSourceGitHub && k.DigestVerified && k.AssetDigest != "" {
		return "下载时与 GitHub 发布的 SHA-256 匹配"
	}
	if k.Source == model.KernelSourceGitHub {
		return "未验证 · 下载时未取得发布摘要"
	}
	return "未验证"
}

func kernelAuthorizationLabel(id string, status model.Status) string {
	if id != "" && slices.Contains(status.AuthorizedKernels, id) {
		return "此文件指纹已获管理员授权"
	}
	if status.Privileged {
		return "此文件指纹尚未授权"
	}
	return "普通用户模式 · 提权运行需管理员授权"
}
