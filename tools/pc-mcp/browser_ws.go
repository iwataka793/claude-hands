package main

// ブラウザの道具で使う、小さな WebSocket の受け手（Chrome の DevTools につなぐだけ）。
// 外の部品を増やさないため、手で書いている（つなぐのは PC の中の 127.0.0.1 だけ）。

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type wsConn struct {
	c   net.Conn
	r   *bufio.Reader
	wmu sync.Mutex
}

const wsMaxMessage = 64 << 20

// wsDial は ws://127.0.0.1:port/path へつなぐ。Chrome がカオルに「許可しますか？」を出している間は、返事を待つ。
func wsDial(raw string, wait time.Duration) (*wsConn, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "ws" {
		return nil, fmt.Errorf("つなぎ先がおかしい：%s", raw)
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, fmt.Errorf("PC の中（127.0.0.1）にしかつながない：%s", host)
	}
	c, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	k := base64.StdEncoding.EncodeToString(key)
	path := u.RequestURI()
	req := "GET " + path + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + k + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	_ = c.SetDeadline(time.Now().Add(wait))
	if _, err := io.WriteString(c, req); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReaderSize(c, 1<<16)
	res, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		c.Close()
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, errConsentTimeout
		}
		return nil, fmt.Errorf("Chrome がつなぐのを断りました（%v）", err)
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		c.Close()
		return nil, fmt.Errorf("Chrome がつなぐのを断りました（%s）", res.Status)
	}
	h := sha1.Sum([]byte(k + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if res.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(h[:]) {
		c.Close()
		return nil, errors.New("Chrome の返事の合言葉が合いません")
	}
	_ = c.SetDeadline(time.Time{})
	return &wsConn{c: c, r: br}, nil
}

var errConsentTimeout = errors.New("consent timeout")

func (w *wsConn) Close() error { return w.c.Close() }

func (w *wsConn) writeFrame(op byte, p []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	hdr := []byte{0x80 | op}
	n := len(p)
	switch {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n < 1<<16:
		hdr = append(hdr, 0x80|126, byte(n>>8), byte(n))
	default:
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(n))
		hdr = append(append(hdr, 0x80|127), b...)
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	hdr = append(hdr, mask...)
	body := make([]byte, n)
	for i := range p {
		body[i] = p[i] ^ mask[i%4]
	}
	_ = w.c.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if _, err := w.c.Write(append(hdr, body...)); err != nil {
		return err
	}
	return nil
}

func (w *wsConn) WriteText(p []byte) error { return w.writeFrame(0x1, p) }

// ReadMessage は、文字のメッセージを 1 つ読む（分かれて届いたものはつなぐ。ping には pong を返す）。
func (w *wsConn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		b0, err := w.r.ReadByte()
		if err != nil {
			return nil, err
		}
		b1, err := w.r.ReadByte()
		if err != nil {
			return nil, err
		}
		fin := b0&0x80 != 0
		op := b0 & 0x0f
		n := uint64(b1 & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.r, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.r, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		var mask []byte
		if b1&0x80 != 0 {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(w.r, mask); err != nil {
				return nil, err
			}
		}
		if n > wsMaxMessage || uint64(len(msg))+n > wsMaxMessage {
			return nil, errors.New("Chrome からの返事が大きすぎます")
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.r, p); err != nil {
			return nil, err
		}
		if mask != nil {
			for i := range p {
				p[i] ^= mask[i%4]
			}
		}
		switch op {
		case 0x8: // close
			return nil, io.EOF
		case 0x9: // ping
			_ = w.writeFrame(0xA, p)
			continue
		case 0xA: // pong
			continue
		case 0x0, 0x1, 0x2:
			msg = append(msg, p...)
			if fin {
				return msg, nil
			}
		default:
			return nil, fmt.Errorf("知らない種類のフレーム %d", op)
		}
	}
}

func hostOnly(s string) string {
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}
