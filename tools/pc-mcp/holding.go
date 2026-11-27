package main

// 守りの場所（%ProgramData%\claude-hands）と取り置き（holding）。
// 作るのはカオルの pc-setup.bat（UAC を通って hands-protect.ps1）。
// 取り置きには、常駐役（＝Claude の普通の権限）は「入れる」ことだけできる。消す・書き換える・取り出すはできない
// （Windows の権限で止める）。戻すのはカオル（エクスプローラーで UAC の「続行」）。
// 30 日たったものは、毎日 1 回の管理者のタスク（hands-daily.ps1）が消す。

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	holdDays  = 30
	gib       = int64(1) << 30
	holdCapGB = 10
)

func handsDir() string {
	if v := env("PC_HANDS_DIR", ""); v != "" {
		return v
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "claude-hands")
	}
	return filepath.Join(stateDir(), "hands")
}

func holdingDir() string { return filepath.Join(handsDir(), "holding") }

type holdItem struct {
	name  string
	when  time.Time
	from  string
	bytes int64
}

type holdInfo struct {
	ready bool
	bytes int64
	items []holdItem // 新しい順
}

func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != root && isReparse(d) {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func scanHolding() holdInfo {
	var hi holdInfo
	ents, err := os.ReadDir(holdingDir())
	if err != nil {
		return hi
	}
	hi.ready = true
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(holdingDir(), e.Name())
		it := holdItem{name: e.Name(), bytes: dirSize(dir)}
		if fi, err := e.Info(); err == nil {
			it.when = fi.ModTime()
		}
		if len(e.Name()) >= 15 {
			if t, err := time.ParseInLocation("20060102-150405", e.Name()[:15], time.Local); err == nil {
				it.when = t
			}
		}
		if f, err := os.Open(filepath.Join(dir, "元の場所.txt")); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				l := strings.TrimPrefix(sc.Text(), "\ufeff")
				if strings.HasPrefix(l, "元の場所：") {
					it.from = strings.TrimPrefix(l, "元の場所：")
					break
				}
			}
			f.Close()
		}
		hi.bytes += it.bytes
		hi.items = append(hi.items, it)
	}
	sort.Slice(hi.items, func(i, j int) bool { return hi.items[i].when.After(hi.items[j].when) })
	return hi
}

// holdCap：C の空きで取り置きの上限を決める（ASK-DESIGN.md の「容量の決まり」）。
// 空きが分からないとき（-1）は 10 GB。
func holdCap(free int64) (int64, string) {
	switch {
	case free < 0:
		return holdCapGB * gib, ""
	case free < 50*gib:
		return 0, "C の空きが 50 GB を切ったので、取り置きを止めています"
	case free < 100*gib:
		return 5 * gib, "C の空きが 100 GB を切ったので、上限を 5 GB にしています"
	}
	return holdCapGB * gib, ""
}

func gb(n int64) string { return fmt.Sprintf("%.1f GB", float64(n)/float64(gib)) }

// guardLines：pc_status に出す「保険と容量」の行
func guardLines() []string {
	var ls []string
	free := cFree()
	if free >= 0 {
		rule := "100 GB 以上を保つ決まり：OK"
		switch {
		case free < 50*gib:
			rule = "50 GB を切った：新しい取り置きを止めている。カオルに聞く"
		case free < 100*gib:
			rule = "100 GB を切った：取り置きの上限を 5 GB に"
		}
		ls = append(ls, fmt.Sprintf("C の空き：%d GB（%s）", free/gib, rule))
	}
	hi := scanHolding()
	if !hi.ready {
		ls = append(ls, "取り置き：まだ作っていない（カオルに pc-setup.bat を動かしてもらう。UAC あり）")
	} else {
		cap, _ := holdCap(free)
		ls = append(ls, fmt.Sprintf("取り置き：%d 件・%s／上限 %s（%d 日で消える。戻すのはカオル）：%s", len(hi.items), gb(hi.bytes), gb(cap), holdDays, holdingDir()))
		for i, it := range hi.items {
			if i >= 3 {
				break
			}
			ls = append(ls, fmt.Sprintf("  %s %s ← %s", it.when.Format("01-02 15:04"), gb(it.bytes), it.from))
		}
	}
	if b, err := os.ReadFile(filepath.Join(handsDir(), "status", "daily.txt")); err == nil {
		for _, l := range strings.Split(strings.ReplaceAll(strings.TrimPrefix(string(b), "\ufeff"), "\r\n", "\n"), "\n") {
			if strings.HasPrefix(l, "時刻：") || strings.HasPrefix(l, "最後の復元ポイント：") || strings.HasPrefix(l, "期限で消した") {
				ls = append(ls, "毎日の点検 "+l)
			}
		}
	}
	if l := lastLine(filepath.Join(homeDir(), "devkit", "backup", "copy-onedrive.log"), "done "); l != "" {
		ls = append(ls, "OneDrive への写し：最後 "+strings.TrimPrefix(l, "done "))
	}
	return ls
}

// cFree：C の空き（試験では PC_HANDS_FREE_GB で決める）
func cFree() int64 {
	if v := env("PC_HANDS_FREE_GB", ""); v != "" {
		var n int64
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n * gib
		}
	}
	return diskFree(systemDrive())
}

func homeDir() string {
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func systemDrive() string {
	if d := os.Getenv("SystemDrive"); d != "" {
		return d + `\`
	}
	return "/"
}

// lastLine：ファイルの最後の方から、prefix で始まるいちばん新しい行
func lastLine(path, prefix string) string {
	b := readTail(path, 64<<10)
	ls := strings.Split(strings.ReplaceAll(b, "\r\n", "\n"), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimPrefix(ls[i], "\ufeff"), prefix) {
			return strings.TrimPrefix(ls[i], "\ufeff")
		}
	}
	return ""
}

// readTail：ファイルの最後 n バイト（大きいファイルを全部読まない）
func readTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && len(buf) == 0 {
		return ""
	}
	s := string(buf)
	if off > 0 { // 途中から読んだので、最初の切れた行を捨てる
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	return s
}
