package update

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSHA256AcceptsSidecarForms(t *testing.T) {
	digest := strings.Repeat("aB", 32)
	want := strings.ToLower(digest)
	for _, body := range []string{
		digest,
		digest + "  openki.exe\n",
		"sha256:" + digest,
		"SHA256 (openki.exe) = " + digest + "\n",
	} {
		got, err := parseSHA256([]byte(body))
		if err != nil {
			t.Errorf("parseSHA256(%q): %v", body, err)
			continue
		}
		if got != want {
			t.Errorf("parseSHA256(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestParseSHA256RejectsMissingOrAmbiguousDigests(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, body := range []string{
		"",
		"openki.exe\n",
		digest + "\n" + strings.Repeat("b", 64) + "\n",
		strings.Repeat("a", 63),
	} {
		if _, err := parseSHA256([]byte(body)); err == nil {
			t.Errorf("parseSHA256(%q) accepted a bad checksum file", body)
		}
	}
}

func TestChecksumURLRequiresHTTPSAndAppendsSibling(t *testing.T) {
	got, err := checksumURL("https://github.com/o/r/releases/download/v1/openki.exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/o/r/releases/download/v1/openki.exe.sha256" {
		t.Fatalf("checksumURL = %q", got)
	}
	if _, err := checksumURL("http://example.test/openki.exe"); err == nil {
		t.Fatal("plain HTTP checksum URL was accepted")
	}
}

func TestFetchChecksumReadsTheSiblingAsset(t *testing.T) {
	payload := []byte("new openki binary")
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openki.exe.sha256" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(want + "  openki.exe\n"))
	}))
	defer srv.Close()
	got, err := fetchChecksum(srv.Client(), srv.URL+"/openki.exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("fetchChecksum = %q, want %q", got, want)
	}
}

func TestFetchChecksumRefusesMissingSidecar(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	if _, err := fetchChecksum(srv.Client(), srv.URL+"/openki.exe"); err == nil {
		t.Fatal("missing checksum sidecar was accepted")
	}
}
