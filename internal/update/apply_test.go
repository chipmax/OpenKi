package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func fakePE(tag byte) []byte {
	b := bytes.Repeat([]byte{tag}, minExeSize+1)
	copy(b, "MZ")
	return b
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type manifestServer struct {
	exe            []byte
	sidecar        []byte
	manifest       []byte
	sig            []byte
	manifestStatus int
}

func (m *manifestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/openki.exe":
		_, _ = w.Write(m.exe)
	case "/openki.exe.sha256":
		if m.sidecar == nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(m.sidecar)
	case "/openki.exe.manifest":
		if m.manifestStatus != 0 {
			w.WriteHeader(m.manifestStatus)
			return
		}
		_, _ = w.Write(m.manifest)
	case "/openki.exe.manifest.sig":
		_, _ = w.Write(m.sig)
	default:
		http.NotFound(w, r)
	}
}

func signedManifest(t *testing.T, pub *string, m Manifest) (manifest, sig []byte) {
	t.Helper()
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	pubKey, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	*pub = base64.StdEncoding.EncodeToString(pubKey)
	return manifest, ed25519.Sign(priv, manifest)
}

func withReleaseKey(t *testing.T, key string) {
	t.Helper()
	old := ReleasePublicKey
	ReleasePublicKey = key
	t.Cleanup(func() { ReleasePublicKey = old })
}

func TestApplyToSwapsVerifiedBinary(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "openki.exe")
	old := fakePE('O')
	if err := os.WriteFile(exePath, old, 0o755); err != nil {
		t.Fatal(err)
	}
	newExe := fakePE('N')
	d := digestOf(newExe)
	var pub string
	manifest, sig := signedManifest(t, &pub, Manifest{
		Version: "2.0.1",
		Asset:   "openki.exe",
		SHA256:  d,
		Size:    int64(len(newExe)),
	})
	withReleaseKey(t, pub)
	srv := httptest.NewTLSServer(&manifestServer{
		exe:      newExe,
		sidecar:  []byte(d + "  openki.exe\n"),
		manifest: manifest,
		sig:      sig,
	})
	defer srv.Close()
	ok, err := applyTo(exePath, srv.URL+"/openki.exe", "2.0.1", srv.Client())
	if err != nil || !ok {
		t.Fatalf("applyTo = %v, %v", ok, err)
	}
	got, err := os.ReadFile(exePath)
	if err != nil || !bytes.Equal(got, newExe) {
		t.Fatalf("exe was not replaced: %v", err)
	}
	bak, err := os.ReadFile(exePath + ".bak")
	if err != nil || !bytes.Equal(bak, old) {
		t.Fatalf("backup = %v, %v", bak, err)
	}
}

func TestApplyToRefusesMissingManifest(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "openki.exe")
	old := fakePE('O')
	if err := os.WriteFile(exePath, old, 0o755); err != nil {
		t.Fatal(err)
	}
	newExe := fakePE('N')
	d := digestOf(newExe)
	var pub string
	manifest, sig := signedManifest(t, &pub, Manifest{
		Version: "2.0.1",
		Asset:   "openki.exe",
		SHA256:  d,
		Size:    int64(len(newExe)),
	})
	withReleaseKey(t, pub)
	srv := httptest.NewTLSServer(&manifestServer{
		exe:            newExe,
		sidecar:        []byte(d + "  openki.exe\n"),
		manifest:       manifest,
		sig:            sig,
		manifestStatus: http.StatusNotFound,
	})
	defer srv.Close()
	if _, err := applyTo(exePath, srv.URL+"/openki.exe", "2.0.1", srv.Client()); err == nil {
		t.Fatal("missing manifest was accepted")
	}
	got, _ := os.ReadFile(exePath)
	if !bytes.Equal(got, old) {
		t.Fatal("exe changed after a refused update")
	}
}

func TestApplyToRefusesBadSignature(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "openki.exe")
	old := fakePE('O')
	if err := os.WriteFile(exePath, old, 0o755); err != nil {
		t.Fatal(err)
	}
	newExe := fakePE('N')
	d := digestOf(newExe)
	var pub string
	manifest, _ := signedManifest(t, &pub, Manifest{
		Version: "2.0.1",
		Asset:   "openki.exe",
		SHA256:  d,
		Size:    int64(len(newExe)),
	})
	withReleaseKey(t, pub)
	srv := httptest.NewTLSServer(&manifestServer{
		exe:      newExe,
		sidecar:  []byte(d + "  openki.exe\n"),
		manifest: manifest,
		sig:      bytes.Repeat([]byte{0}, ed25519.SignatureSize),
	})
	defer srv.Close()
	if _, err := applyTo(exePath, srv.URL+"/openki.exe", "2.0.1", srv.Client()); err == nil {
		t.Fatal("invalid signature was accepted")
	}
}

func TestApplyToRefusesNonPE(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "openki.exe")
	old := fakePE('O')
	if err := os.WriteFile(exePath, old, 0o755); err != nil {
		t.Fatal(err)
	}
	newExe := bytes.Repeat([]byte{'X'}, minExeSize+1)
	d := digestOf(newExe)
	var pub string
	manifest, sig := signedManifest(t, &pub, Manifest{
		Version: "2.0.1",
		Asset:   "openki.exe",
		SHA256:  d,
		Size:    int64(len(newExe)),
	})
	withReleaseKey(t, pub)
	srv := httptest.NewTLSServer(&manifestServer{
		exe:      newExe,
		sidecar:  []byte(d + "  openki.exe\n"),
		manifest: manifest,
		sig:      sig,
	})
	defer srv.Close()
	if _, err := applyTo(exePath, srv.URL+"/openki.exe", "2.0.1", srv.Client()); err == nil {
		t.Fatal("a non-PE payload was accepted")
	}
	got, _ := os.ReadFile(exePath)
	if !bytes.Equal(got, old) {
		t.Fatal("exe changed after a non-PE refusal")
	}
}

func TestApplyToRefusesDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "openki.exe")
	old := fakePE('O')
	if err := os.WriteFile(exePath, old, 0o755); err != nil {
		t.Fatal(err)
	}
	newExe := fakePE('N')
	d := digestOf(fakePE('Z'))
	var pub string
	manifest, sig := signedManifest(t, &pub, Manifest{
		Version: "2.0.1",
		Asset:   "openki.exe",
		SHA256:  d,
		Size:    int64(len(newExe)),
	})
	withReleaseKey(t, pub)
	srv := httptest.NewTLSServer(&manifestServer{
		exe:      newExe,
		sidecar:  []byte(d + "  openki.exe\n"),
		manifest: manifest,
		sig:      sig,
	})
	defer srv.Close()
	if _, err := applyTo(exePath, srv.URL+"/openki.exe", "2.0.1", srv.Client()); err == nil {
		t.Fatal("a checksum mismatch was accepted")
	}
}

func TestVerifyManifestRejectsTamperedSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	withReleaseKey(t, base64.StdEncoding.EncodeToString(pub))
	m := []byte(`{"version":"1.0.0"}`)
	if err := verifyManifest(m, ed25519.Sign(priv, m)); err != nil {
		t.Fatal(err)
	}
	if err := verifyManifest(m, append(ed25519.Sign(priv, m), 0)); err == nil {
		t.Fatal("a long signature was accepted")
	}
	if err := verifyManifest(m, ed25519.Sign(priv, []byte("other"))); err == nil {
		t.Fatal("a signature over different bytes was accepted")
	}
}

func TestBackupRetentionAndRestore(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "openki.exe")
	if err := os.WriteFile(exe, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe+".bak", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe+".bak.old", []byte("older"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe+".new-123", []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupBackup(exe)
	if _, err := os.Stat(exe + ".new-123"); !os.IsNotExist(err) {
		t.Fatal("partial download was kept")
	}
	if _, err := os.Stat(exe + ".bak"); !os.IsNotExist(err) {
		t.Fatal(".bak was not rotated to .bak.old")
	}
	bak, err := os.ReadFile(exe + ".bak.old")
	if err != nil || string(bak) != "old" {
		t.Fatalf(".bak.old = %q, %v", bak, err)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	restoreBackup(exe)
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "old" {
		t.Fatalf("restore = %q, %v", got, err)
	}
}
