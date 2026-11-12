package lane

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSOCKS5 implements just enough of RFC 1928 for the Proton transport test:
// no-auth greeting, CONNECT with a domain or IPv4 target, then a raw tunnel.
// The target address is recorded so the test can prove the request did not
// leave directly.
func fakeSOCKS5(t *testing.T, target string) (addr string, got func() string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu   sync.Mutex
		seen string
	)
	get := func() string {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				var head [2]byte
				if _, err := io.ReadFull(br, head[:]); err != nil {
					return
				}
				n := int(head[1])
				if _, err := io.CopyN(io.Discard, br, int64(n)); err != nil {
					return
				}
				if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
					return
				}
				var req [4]byte
				if _, err := io.ReadFull(br, req[:]); err != nil {
					return
				}
				var host string
				switch req[3] {
				case 0x01:
					var b [4]byte
					if _, err := io.ReadFull(br, b[:]); err != nil {
						return
					}
					host = net.IP(b[:]).String()
				case 0x03:
					var l [1]byte
					if _, err := io.ReadFull(br, l[:]); err != nil {
						return
					}
					name := make([]byte, int(l[0]))
					if _, err := io.ReadFull(br, name); err != nil {
						return
					}
					host = string(name)
				default:
					return
				}
				var p [2]byte
				if _, err := io.ReadFull(br, p[:]); err != nil {
					return
				}
				port := binary.BigEndian.Uint16(p[:])
				mu.Lock()
				seen = net.JoinHostPort(host, strconv.Itoa(int(port)))
				mu.Unlock()
				up, err := net.DialTimeout("tcp", target, 2*time.Second)
				if err != nil {
					return
				}
				defer up.Close()
				if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				go func() { _, _ = io.Copy(up, br) }()
				_, _ = io.Copy(c, up)
			}(conn)
		}
	}()
	return ln.Addr().String(), get, func() { _ = ln.Close(); <-done }
}

func TestProtonTransportDialsThroughSocks5(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "via-proton")
	}))
	defer target.Close()
	targetAddr := strings.TrimPrefix(target.URL, "http://")

	proxyAddr, seen, closeProxy := fakeSOCKS5(t, targetAddr)
	defer closeProxy()

	tr, err := NewProtonTransport("socks5h://" + proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	resp, err := client.Get("http://example.test/proton")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "via-proton" {
		t.Fatalf("body = %q, want via-proton", body)
	}
	if seen() != "example.test:80" {
		t.Fatalf("SOCKS5 target = %q, want example.test:80 (request bypassed the proxy)", seen())
	}
}

func TestProtonEgressPeerLabelsAndFallsBackToNoOtherPeer(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()
	proxyAddr, _, closeProxy := fakeSOCKS5(t, strings.TrimPrefix(target.URL, "http://"))
	defer closeProxy()

	note := SetEgress(EgressOptions{
		Modes:     []string{"proton"},
		ProtonURL: "socks5h://" + proxyAddr,
		BaseMode:  "direct",
	})
	defer SetEgress(EgressOptions{})
	if note != "" {
		t.Fatalf("SetEgress note = %q, want empty for a valid listener", note)
	}
	if !EgressExplicit() {
		t.Fatal("proton mode did not install the peer selector")
	}
	req, err := http.NewRequest(http.MethodGet, "http://example.test/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := laneHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-OpenKi-Egress"); got != "proton" {
		t.Fatalf("egress label = %q, want proton", got)
	}
}

func TestNormalizeProtonSocks5(t *testing.T) {
	got, err := NormalizeProtonSocks5(" 127.0.0.1:1080 ")
	if err != nil || got != "socks5h://127.0.0.1:1080" {
		t.Fatalf("bare host = %q, err = %v", got, err)
	}
	if got, err := NormalizeProtonSocks5(""); err != nil || got != DefaultProtonSocks5 {
		t.Fatalf("empty = %q, err = %v", got, err)
	}
	for _, raw := range []string{"http://127.0.0.1:8080", "socks5://127.0.0.1", "socks5h://127.0.0.1:1080/path"} {
		if _, err := NormalizeProtonSocks5(raw); err == nil {
			t.Errorf("NormalizeProtonSocks5(%q) accepted an invalid listener", raw)
		}
	}
}
