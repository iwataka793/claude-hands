package main

// 容量の調べ：ドライブの空き・フォルダの大きさ（中を全部たどって足す）・膨らみやすい場所。
// たどるのは Go（PowerShell より速い）。つなぎ目（ジャンクション）はたどらない（二重に数えないため）。
// クラウドにだけある OneDrive などのファイルは「クラウドのみ」として分けて数える（PC の容量は使わない）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type sizeResult struct {
	Path   string
	Local  int64 // PC の容量を使っている分
	Cloud  int64 // クラウドにだけある分
	Files  int64
	Denied int64 // 開けなかったフォルダの数
	Err    string
}

// walkSize は path の中の大きさを足す。bigFiles が nil でなければ大きいファイルも集める。
func walkSize(ctx context.Context, path string, big *bigFiles) sizeResult {
	res := sizeResult{Path: path}
	st, err := os.Lstat(path)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	if !st.IsDir() {
		if isCloudOnly(st) {
			res.Cloud = st.Size()
		} else {
			res.Local = st.Size()
		}
		res.Files = 1
		return res
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if d == nil || d.IsDir() {
				res.Denied++
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != path && isReparse(d) {
				return fs.SkipDir
			}
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if isReparse(d) {
			return nil
		}
		res.Files++
		if isCloudOnly(fi) {
			res.Cloud += fi.Size()
			return nil
		}
		res.Local += fi.Size()
		if big != nil {
			big.add(p, fi.Size(), fi.ModTime())
		}
		return nil
	})
	if ctx.Err() != nil {
		res.Err = "時間切れ（途中まで）"
	}
	return res
}

type bigFile struct {
	Path string
	Size int64
	Mod  time.Time
}

type bigFiles struct {
	mu  sync.Mutex
	n   int
	min int64
	xs  []bigFile
}

func (b *bigFiles) add(p string, s int64, m time.Time) {
	if s < 50<<20 { // 50 MB より小さいものは数えない
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.xs = append(b.xs, bigFile{p, s, m})
	if len(b.xs) > b.n*4 {
		sort.Slice(b.xs, func(i, j int) bool { return b.xs[i].Size > b.xs[j].Size })
		b.xs = b.xs[:b.n]
	}
}

func human(n int64) string {
	switch {
	case n >= 10<<30:
		return fmt.Sprintf("%d GB", n>>30)
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	case n > 0:
		return "1 MB 未満"
	}
	return "0"
}

// 膨らみやすい場所（ある物だけ大きさを出す）
var knownPlaces = []struct{ Name, Pattern string }{
	{"一時ファイル（自分）", `%TEMP%`},
	{"一時ファイル（Windows）", `C:\Windows\Temp`},
	{"ダウンロード", `%USERPROFILE%\Downloads`},
	{"ごみ箱", `C:\$Recycle.Bin`},
	{"Windows Update のダウンロード", `C:\Windows\SoftwareDistribution\Download`},
	{"前の Windows（Windows.old）", `C:\Windows.old`},
	{"休止状態のファイル", `C:\hiberfil.sys`},
	{"仮想メモリのファイル", `C:\pagefile.sys`},
	{"スワップファイル", `C:\swapfile.sys`},
	{"クラッシュの記録（ダンプ）", `%LOCALAPPDATA%\CrashDumps`},
	{"エラー報告（自分）", `%LOCALAPPDATA%\Microsoft\Windows\WER`},
	{"エラー報告（全体）", `C:\ProgramData\Microsoft\Windows\WER`},
	{"iPhone のバックアップ（Apple Devices）", `%USERPROFILE%\Apple\MobileSync\Backup`},
	{"iPhone のバックアップ（iTunes）", `%APPDATA%\Apple Computer\MobileSync\Backup`},
	{"Docker の仮想ディスク", `%LOCALAPPDATA%\Docker\wsl`},
	{"WSL の仮想ディスク", `%LOCALAPPDATA%\Packages\*Canonical*\LocalState`},
	{"WSL（ほか）", `%LOCALAPPDATA%\wsl`},
	{"Chrome のキャッシュ", `%LOCALAPPDATA%\Google\Chrome\User Data\*\Cache`},
	{"Chrome のコードキャッシュ", `%LOCALAPPDATA%\Google\Chrome\User Data\*\Code Cache`},
	{"Edge のキャッシュ", `%LOCALAPPDATA%\Microsoft\Edge\User Data\*\Cache`},
	{"Edge のコードキャッシュ", `%LOCALAPPDATA%\Microsoft\Edge\User Data\*\Code Cache`},
	{"pip のキャッシュ", `%LOCALAPPDATA%\pip\cache`},
	{"npm のキャッシュ", `%LOCALAPPDATA%\npm-cache`},
	{"いろいろなキャッシュ（.cache）", `%USERPROFILE%\.cache`},
	{"Playwright のブラウザ", `%LOCALAPPDATA%\ms-playwright`},
	{"Go のモジュール", `%USERPROFILE%\go\pkg\mod`},
	{"Go のビルドキャッシュ", `%LOCALAPPDATA%\go-build`},
	{"NuGet のパッケージ", `%USERPROFILE%\.nuget\packages`},
	{"インストーラーの控え（Package Cache）", `C:\ProgramData\Package Cache`},
	{"Claude（ストア版）のデータ", `%LOCALAPPDATA%\Packages\Claude_*\LocalCache`},
	{"VS Code のキャッシュ", `%APPDATA%\Code\Cache*`},
	{"OneDrive（PC にある分）", `%USERPROFILE%\OneDrive*`},
}

type diskIn struct {
	Path  string `json:"path"`
	Top   int    `json:"top"`
	Files bool   `json:"files"`
	Known *bool  `json:"known"`
	Secs  int    `json:"seconds"`
}

func (a *Agent) disk(ctx context.Context, raw json.RawMessage) Reply {
	in, err := decode[diskIn](raw)
	if err != nil {
		return errText("引数が読めません：%v", err)
	}
	if in.Top <= 0 {
		in.Top = 20
	}
	if in.Secs <= 0 || in.Secs > 600 {
		in.Secs = 120
	}
	known := in.Path == ""
	if in.Known != nil {
		known = *in.Known
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(in.Secs)*time.Second)
	defer cancel()
	var out []string

	// ドライブ
	if d, err := a.ps.Run(ctx, scriptDrives, nil, 30*time.Second); err == nil && !strings.HasPrefix(strings.TrimSpace(d), "ERROR") {
		out = append(out, "■ ドライブ", d)
	}

	// フォルダの中身の大きさ
	if in.Path != "" {
		p := normPath(in.Path)
		ents, err := os.ReadDir(p)
		if err != nil {
			return errText("%s を開けません：%v", p, err)
		}
		var big *bigFiles
		if in.Files {
			big = &bigFiles{n: in.Top}
		}
		results := make([]sizeResult, len(ents))
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		var done atomic.Int64
		for i, e := range ents {
			if e.Type()&fs.ModeSymlink != 0 || isReparse(e) {
				results[i] = sizeResult{Path: filepath.Join(p, e.Name()), Err: "つなぎ目（数えない）"}
				continue
			}
			wg.Add(1)
			go func(i int, name string) {
				defer wg.Done()
				sem <- struct{}{}
				results[i] = walkSize(ctx, filepath.Join(p, name), big)
				<-sem
				done.Add(1)
			}(i, e.Name())
		}
		wg.Wait()
		sort.Slice(results, func(i, j int) bool { return results[i].Local > results[j].Local })
		var total, cloud, denied int64
		for _, r := range results {
			total += r.Local
			cloud += r.Cloud
			denied += r.Denied
		}
		out = append(out, fmt.Sprintf("■ %s の中（合計 %s）", p, human(total)))
		for i, r := range results {
			if i >= in.Top {
				out = append(out, fmt.Sprintf("  ほか %d 個", len(results)-in.Top))
				break
			}
			name := filepath.Base(r.Path)
			line := fmt.Sprintf("  %8s  %s", human(r.Local), name)
			var notes []string
			if r.Cloud > 0 {
				notes = append(notes, "クラウドのみ "+human(r.Cloud))
			}
			if r.Denied > 0 {
				notes = append(notes, fmt.Sprintf("開けないフォルダ %d", r.Denied))
			}
			if r.Err != "" {
				notes = append(notes, r.Err)
			}
			if len(notes) > 0 {
				line += "（" + strings.Join(notes, "・") + "）"
			}
			out = append(out, line)
		}
		if cloud > 0 {
			out = append(out, "  ※ クラウドのみの分は PC の容量を使っていない")
		}
		if denied > 0 {
			out = append(out, fmt.Sprintf("  ※ 開けなかったフォルダが %d 個（権限）。その分は数えていない", denied))
		}
		if big != nil {
			sort.Slice(big.xs, func(i, j int) bool { return big.xs[i].Size > big.xs[j].Size })
			out = append(out, "■ 大きいファイル（50 MB 以上）")
			for i, f := range big.xs {
				if i >= in.Top {
					break
				}
				out = append(out, fmt.Sprintf("  %8s  %s  %s", human(f.Size), f.Mod.Format("2006-01-02"), f.Path))
			}
		}
	}

	// 膨らみやすい場所
	if known {
		out = append(out, "■ 膨らみやすい場所（あるものだけ）")
		type kr struct {
			name string
			r    sizeResult
		}
		var krs []kr
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for _, k := range knownPlaces {
			matches, _ := filepath.Glob(expandWinEnv(k.Pattern))
			for _, m := range matches {
				wg.Add(1)
				go func(name, m string) {
					defer wg.Done()
					sem <- struct{}{}
					r := walkSize(ctx, m, nil)
					<-sem
					if r.Local > 0 || r.Cloud > 0 {
						mu.Lock()
						krs = append(krs, kr{name, r})
						mu.Unlock()
					}
				}(k.Name, m)
			}
		}
		wg.Wait()
		sort.Slice(krs, func(i, j int) bool { return krs[i].r.Local > krs[j].r.Local })
		for _, k := range krs {
			line := fmt.Sprintf("  %8s  %s  %s", human(k.r.Local), k.name, k.r.Path)
			if k.r.Denied > 0 {
				line += fmt.Sprintf("（開けないフォルダ %d）", k.r.Denied)
			}
			out = append(out, line)
		}
		out = append(out, "  ※ WinSxS など Windows 本体の中は、見かけの大きさと実際が違うので数えていない")
	}
	if ctx.Err() != nil {
		out = append(out, fmt.Sprintf("※ %d 秒で打ち切った。seconds を増やすか、場所を狭める", in.Secs))
	}
	return okText(strings.Join(out, "\n"))
}
