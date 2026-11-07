package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nativejet/internal/platform"
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

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func (p *Prepared) Script(remove bool) (string, error) {
	data, err := os.ReadFile(filepath.Join(p.Directory, "payload.json"))
	if err != nil {
		return "", err
	}
	var payload Payload
	if err = json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	if !digestPattern.MatchString(p.Digest) || !digestPattern.MatchString(payload.HelperSHA256) {
		return "", errors.New("安装摘要无效")
	}
	args := []string{"--install-service", "--payload", p.Directory, "--digest", p.Digest}
	if remove {
		args = append(args, "--remove-service")
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return fmt.Sprintf(`set -eu
NJ_INSTALL_STAGE=$(/usr/bin/mktemp -d /private/tmp/nativejet-installer.XXXXXXXX)
trap '/bin/rm -rf "$NJ_INSTALL_STAGE"' EXIT
NJ_INSTALL_TOOL="$NJ_INSTALL_STAGE/nativejet-helper"
/bin/cp %s "$NJ_INSTALL_TOOL"
NJ_INSTALL_HASH=$(/usr/bin/shasum -a 256 "$NJ_INSTALL_TOOL")
if [ "${NJ_INSTALL_HASH%%%% *}" != %s ]; then
  echo '{"error":"授权后安装器已改变，安装已取消"}'
  exit 1
fi
/bin/chmod 0700 "$NJ_INSTALL_TOOL"
"$NJ_INSTALL_TOOL" %s
`, shellQuote(p.Helper), shellQuote(payload.HelperSHA256), strings.Join(quoted, " ")), nil
}

func (p *Prepared) Run(remove bool) error {
	script, err := p.Script(remove)
	if err != nil {
		return err
	}
	output, err := platform.RunAuthorizedScript(script)
	if err != nil {
		return err
	}
	var result Result
	if err = json.Unmarshal([]byte(output), &result); err != nil {
		return fmt.Errorf("安装器未返回有效结果: %s", strings.TrimSpace(output))
	}
	if result.Error != "" {
		return errors.New(result.Error)
	}
	return nil
}
