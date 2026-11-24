package main

// DevKit などを LiveContainer に直接入れる（Safari の手作業なし）。
// LiveContainer は起動前に LCAppInfo.plist の LCPatchRevision を見て、古ければ実行ファイルに
// パッチを当てて自分の証明書で署名し直す。そこで .app の中身を上書きし、LCPatchRevision を消す。
// 上書きする前の中身は PC に控えとして残す。

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielpaulus/go-ios/ios/afc"
	"github.com/danielpaulus/go-ios/ios/house_arrest"
	"howett.net/plist"
)

const releaseBase = "https://github.com/iwataka793/iphone-dev-kit/releases"

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	c := &http.Client{Timeout: 3 * time.Minute}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s：%d", url, res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

func afcRead(c *afc.Client, p string) ([]byte, error) {
	f, err := c.Open(p, afc.READ_ONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func afcWrite(c *afc.Client, p string, data []byte) error {
	f, err := c.Open(p, afc.WRITE_ONLY_CREATE_TRUNC)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// LCInstall は Releases のタグ（"latest" なら最新）の DevKit.ipa を LiveContainer に入れる。
// 先に LiveContainer を終わらせておくこと（動いている実行ファイルを上書きしない）。
func (p *Phone) LCInstall(ctx context.Context, tag, asset, backupRoot string) (string, error) {
	if asset == "" {
		asset = "DevKit.ipa"
	}
	url := releaseBase + "/download/" + tag + "/" + asset
	if tag == "" || tag == "latest" {
		url = releaseBase + "/latest/download/" + asset
	}
	ipa, err := download(ctx, url)
	if err != nil {
		return "", fmt.Errorf("IPA を取れません：%w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(ipa), int64(len(ipa)))
	if err != nil {
		return "", fmt.Errorf("IPA を開けません：%w", err)
	}
	prefix := ""
	for _, f := range zr.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) >= 3 && parts[0] == "Payload" && strings.HasSuffix(parts[1], ".app") {
			prefix = "Payload/" + parts[1] + "/"
			break
		}
	}
	if prefix == "" {
		return "", fmt.Errorf("IPA の中に .app がありません")
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, prefix) || strings.HasSuffix(f.Name, "/") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return "", err
		}
		files[strings.TrimPrefix(f.Name, prefix)] = b
	}
	var info map[string]any
	if _, err := plist.Unmarshal(files["Info.plist"], &info); err != nil {
		return "", fmt.Errorf("Info.plist を読めません：%w", err)
	}
	bid, _ := info["CFBundleIdentifier"].(string)
	if bid == "" {
		return "", fmt.Errorf("bundle id がありません")
	}

	lc := p.LiveContainer()
	if lc == "" {
		return "", fmt.Errorf("LiveContainer が見つかりません")
	}
	c, err := house_arrest.New(p.device, lc)
	if err != nil {
		return "", fmt.Errorf("LiveContainer の中を開けません：%w", err)
	}
	defer c.Close()
	appDir := "/Documents/Applications/" + bid + ".app"
	lcInfoRaw, err := afcRead(c, appDir+"/LCAppInfo.plist")
	if err != nil {
		return "", fmt.Errorf("%s はまだ LiveContainer に入っていません（最初の 1 回は Safari → LiveContainer で入れる）：%w", bid, err)
	}

	// 控え：今の中身を PC に取っておく
	backup := filepath.Join(backupRoot, time.Now().Format("20060102-150405")+"-"+bid+".app")
	_ = os.MkdirAll(backup, 0o755)
	_ = c.WalkDir(appDir, func(p string, fi afc.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, appDir+"/")
		b, err := afcRead(c, p)
		if err == nil {
			dst := filepath.Join(backup, filepath.FromSlash(rel))
			_ = os.MkdirAll(filepath.Dir(dst), 0o755)
			_ = os.WriteFile(dst, b, 0o644)
		}
		return nil
	})

	var lcInfo map[string]any
	if _, err := plist.Unmarshal(lcInfoRaw, &lcInfo); err != nil {
		return "", fmt.Errorf("LCAppInfo.plist を読めません：%w", err)
	}
	oldRev := lcInfo["LCPatchRevision"]
	delete(lcInfo, "LCPatchRevision")
	newInfo, err := plist.Marshal(lcInfo, plist.BinaryFormat)
	if err != nil {
		return "", err
	}

	dirs := map[string]bool{}
	n := 0
	for rel, b := range files {
		d := path.Dir(rel)
		for d != "." && !dirs[d] {
			dirs[d] = true
			_ = c.MkDir(appDir + "/" + d)
			d = path.Dir(d)
		}
		if err := afcWrite(c, appDir+"/"+rel, b); err != nil {
			return "", fmt.Errorf("%s を書けません：%w（控え：%s）", rel, err, backup)
		}
		n++
	}
	if err := afcWrite(c, appDir+"/LCAppInfo.plist", newInfo); err != nil {
		return "", fmt.Errorf("LCAppInfo.plist を書けません：%w（控え：%s）", err, backup)
	}
	return fmt.Sprintf("%s（%s）を LiveContainer に入れた：ファイル %d 個、LCPatchRevision %v → 削除。控え：%s\n次に LiveContainer を前に出して「実行」を押すと、LiveContainer がパッチと署名をしてから起動する。",
		bid, url, n, oldRev, backup), nil
}
