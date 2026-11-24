package main

// ファイルを見る：一覧・中身（先頭／末尾／正規表現で絞る）・名前で探す・詳しい情報。
// パスワード・セキュリティ・お金にかかわる場所の「中身」は、カオルの許可が要る（名前と大きさは見てよい）。

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

type filesIn struct {
	Action   string `json:"action"` // list / read / find / info
	Path     string `json:"path"`
	Pattern  string `json:"pattern"` // read：行を絞る正規表現　find：名前の形（*.log など）
	Head     int    `json:"head"`
	Tail     int    `json:"tail"`
	Depth    int    `json:"depth"`
	Sort     string `json:"sort"` // list / find：name / new / big
	Max      int    `json:"max"`
	MaxBytes int    `json:"max_bytes"`
}

func (a *Agent) files(ctx context.Context, raw json.RawMessage) Reply {
	in, err := decode[filesIn](raw)
	if err != nil {
		return errText("引数が読めません：%v", err)
	}
	if in.Path == "" {
		return errText("path を指定してください")
	}
	p := normPath(in.Path)
	if in.Max <= 0 {
		in.Max = 200
	}
	switch in.Action {
	case "", "list":
		return listDir(p, in)
	case "read":
		if r := a.needAllowFor(p, false); r != nil {
			return *r
		}
		return a.readFile(ctx, p, in)
	case "find":
		return findFiles(ctx, p, in)
	case "info":
		st, err := os.Stat(p)
		if err != nil {
			return errText("%v", err)
		}
		kind := "ファイル"
		if st.IsDir() {
			kind = "フォルダ"
		}
		extra := ""
		if isCloudOnly(st) {
			extra = "（クラウドのみ）"
		}
		return okTextf("%s：%s%s\n大きさ：%s（%d バイト）\n更新：%s", kind, p, extra, human(st.Size()), st.Size(), st.ModTime().Format("2006-01-02 15:04:05"))
	}
	return errText("action は list / read / find / info")
}

type entry struct {
	name string
	dir  bool
	size int64
	mod  time.Time
}

func sortEntries(es []entry, how string) {
	switch how {
	case "new":
		sort.Slice(es, func(i, j int) bool { return es[i].mod.After(es[j].mod) })
	case "big":
		sort.Slice(es, func(i, j int) bool { return es[i].size > es[j].size })
	default:
		sort.Slice(es, func(i, j int) bool {
			if es[i].dir != es[j].dir {
				return es[i].dir
			}
			return strings.ToLower(es[i].name) < strings.ToLower(es[j].name)
		})
	}
}

func listDir(p string, in filesIn) Reply {
	ds, err := os.ReadDir(p)
	if err != nil {
		return errText("%s を開けません：%v", p, err)
	}
	var es []entry
	for _, d := range ds {
		e := entry{name: d.Name(), dir: d.IsDir()}
		if fi, err := d.Info(); err == nil {
			e.size, e.mod = fi.Size(), fi.ModTime()
		}
		es = append(es, e)
	}
	sortEntries(es, in.Sort)
	out := []string{fmt.Sprintf("%s（%d 個）", p, len(es))}
	for i, e := range es {
		if i >= in.Max {
			out = append(out, fmt.Sprintf("ほか %d 個", len(es)-in.Max))
			break
		}
		if e.dir {
			out = append(out, fmt.Sprintf("  [フォルダ]  %s  %s", e.mod.Format("2006-01-02 15:04"), e.name))
		} else {
			out = append(out, fmt.Sprintf("  %8s  %s  %s", human(e.size), e.mod.Format("2006-01-02 15:04"), e.name))
		}
	}
	return okText(strings.Join(out, "\n"))
}

func findFiles(ctx context.Context, root string, in filesIn) Reply {
	if in.Depth <= 0 {
		in.Depth = 6
	}
	pat := strings.ToLower(in.Pattern)
	if pat == "" {
		pat = "*"
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	base := strings.Count(root, string(os.PathSeparator))
	var es []entry
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && (isReparse(d) || strings.Count(p, string(os.PathSeparator))-base >= in.Depth) {
				return fs.SkipDir
			}
			return nil
		}
		if ok, _ := filepath.Match(pat, strings.ToLower(d.Name())); !ok {
			return nil
		}
		e := entry{name: p}
		if fi, err := d.Info(); err == nil {
			e.size, e.mod = fi.Size(), fi.ModTime()
		}
		es = append(es, e)
		return nil
	})
	if in.Sort == "" {
		in.Sort = "new"
	}
	sortEntries(es, in.Sort)
	out := []string{fmt.Sprintf("%s の中の「%s」（%d 個・深さ %d まで）", root, in.Pattern, len(es), in.Depth)}
	for i, e := range es {
		if i >= in.Max {
			out = append(out, fmt.Sprintf("ほか %d 個", len(es)-in.Max))
			break
		}
		out = append(out, fmt.Sprintf("  %8s  %s  %s", human(e.size), e.mod.Format("2006-01-02 15:04"), e.name))
	}
	if ctx.Err() != nil {
		out = append(out, "※ 60 秒で打ち切った。場所を狭めるか depth を減らす")
	}
	return okText(strings.Join(out, "\n"))
}

func (a *Agent) readFile(ctx context.Context, p string, in filesIn) Reply {
	if in.MaxBytes <= 0 {
		in.MaxBytes = 20 << 20
	}
	st, err := os.Stat(p)
	if err != nil {
		return errText("%v", err)
	}
	if st.IsDir() {
		return errText("%s はフォルダです（action=list を使う）", p)
	}
	f, err := os.Open(p)
	if err != nil {
		return errText("%v", err)
	}
	defer f.Close()
	var b []byte
	// 大きいファイルで末尾だけ欲しいときは、後ろから読む
	if st.Size() > int64(in.MaxBytes) && in.Tail > 0 && in.Head == 0 && in.Pattern == "" {
		_, _ = f.Seek(st.Size()-int64(in.MaxBytes), 0)
	}
	b = make([]byte, min(st.Size(), int64(in.MaxBytes)))
	n, _ := f.Read(b)
	b = b[:n]
	txt, enc := a.decodeText(ctx, b, p)
	if txt == "" && enc == "バイナリ" {
		return errText("%s は文字のファイルではありません（%s）", p, human(st.Size()))
	}
	lines := strings.Split(strings.ReplaceAll(txt, "\r\n", "\n"), "\n")
	head := fmt.Sprintf("%s（%s・%s・%d 行）", p, human(st.Size()), enc, len(lines))
	if st.Size() > int64(in.MaxBytes) {
		head += fmt.Sprintf("※ 大きいので %s だけ読んだ", human(int64(in.MaxBytes)))
	}
	var out []string
	switch {
	case in.Pattern != "":
		re, err := regexp.Compile(in.Pattern)
		if err != nil {
			return errText("正規表現の誤り：%v", err)
		}
		for i, l := range lines {
			if re.MatchString(l) {
				out = append(out, fmt.Sprintf("%6d: %s", i+1, clip(l, 400)))
			}
		}
		if in.Tail > 0 && len(out) > in.Tail {
			out = out[len(out)-in.Tail:]
		} else if len(out) > in.Max {
			out = append(out[:in.Max], fmt.Sprintf("（ほか %d 行）", len(out)-in.Max))
		}
		head += fmt.Sprintf("・「%s」に合う行", in.Pattern)
	case in.Tail > 0:
		s := max(0, len(lines)-in.Tail)
		for i := s; i < len(lines); i++ {
			out = append(out, fmt.Sprintf("%6d: %s", i+1, clip(lines[i], 400)))
		}
	default:
		h := in.Head
		if h <= 0 {
			h = 200
		}
		for i := 0; i < len(lines) && i < h; i++ {
			out = append(out, fmt.Sprintf("%6d: %s", i+1, clip(lines[i], 400)))
		}
		if len(lines) > h {
			out = append(out, fmt.Sprintf("（ほか %d 行。head / tail / pattern で選ぶ）", len(lines)-h))
		}
	}
	return okText(head + "\n" + strings.Join(out, "\n"))
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// decodeText は、UTF-8・UTF-16・Shift_JIS（CP932）を見分けて文字にする。
func (a *Agent) decodeText(ctx context.Context, b []byte, p string) (string, string) {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:]), "UTF-8"
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16le(b[2:]), "UTF-16"
	}
	probe := b
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return "", "バイナリ"
	}
	if utf8.Valid(b) {
		return string(b), "UTF-8"
	}
	// Shift_JIS は Windows に直してもらう（大きすぎるものは先頭 2 MB だけ）
	if len(b) > 2<<20 {
		b = b[:2<<20]
	}
	out, err := a.ps.Run(ctx, `[Text.Encoding]::GetEncoding(932).GetString([Convert]::FromBase64String($A.b))`,
		map[string]string{"b": b64(b)}, 30*time.Second)
	if err == nil {
		return out, "Shift_JIS"
	}
	return strings.ToValidUTF8(string(b), "?"), "不明（読めない字は ?）"
}

func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u))
}
