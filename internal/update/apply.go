package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Apply downloads the new binary from a release asset and swaps it with the
// running one. Windows refuses to overwrite a running exe but allows renaming
// it, so the swap is: current → .bak, .new → current, spawn the new exe,
// exit. The caller exits right after Apply returns success.
//
// Only Windows reaches this path — see SelfUpdateSupported. Everywhere else
// the release is a bundle the user installs by hand, because replacing a
// signed .app's inner binary in place would break the signature.
//
// download is the release asset URL (https only). Two sibling assets must
// exist:
//
//	<asset>.sha256          expected SHA-256 (integrity gate)
//	<asset>.manifest        {"version","asset","sha256","size"} (signed)
//	<asset>.manifest.sig    64-byte Ed25519 signature over the manifest
//
// The checksum alone only proves the bytes were not truncated or swapped on
// the release host; it travels over the same channel as the binary. The signed
// manifest adds authenticity: a compromised release account can replace the
// binary and the sidecar, but cannot produce a new signature without the
// private key. Release builds embed the public key (ReleasePublicKey); forks
// replace it with their own key. An empty key disables the manifest check,
// which is a development-only configuration.

const (
	minExeSize  = 1 << 20   // 1 MiB sanity floor
	maxExeSize  = 256 << 20 // 256 MiB ceiling: no release is anywhere near it
	sha256HexRE = `(?i)\b[0-9a-f]{64}\b`
)

var (
	sha256HexPattern = regexp.MustCompile(sha256HexRE)
	// applyMu serializes swaps. Two admin requests that both saw the same
	// update would otherwise race on <exe>.new-<pid> and <exe>.bak.
	applyMu sync.Mutex
)

// siblingURL appends a suffix to the asset path (".sha256", ".manifest",
// ".manifest.sig"). Keeping every verification artifact beside the asset means
// a custom feed has the same requirement as the bundled GitHub feed.
func siblingURL(downloadURL, suffix string) (string, error) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return "", fmt.Errorf("invalid update asset url: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("update asset url must be https")
	}
	u.Path += suffix
	return u.String(), nil
}

func checksumURL(downloadURL string) (string, error) {
	return siblingURL(downloadURL, ".sha256")
}

// fetchChecksum downloads and parses the sibling digest before the executable
// is touched. A missing checksum is a refusal, not a warning: an unverifiable
// binary must never replace the running gateway. The shared HTTP helper is
// used so a rate-limited proxy exit gets the same no-proxy retry as the feed.
func fetchChecksum(client *http.Client, downloadURL string) (string, error) {
	cu, err := checksumURL(downloadURL)
	if err != nil {
		return "", err
	}
	body, err := (HTTP{Client: client, Timeout: 30 * time.Second}).Get(cu)
	if err != nil {
		return "", fmt.Errorf("fetch update checksum: %w", err)
	}
	if len(body) > 4096 {
		return "", fmt.Errorf("update checksum file is too large")
	}
	return parseSHA256(body)
}

// parseSHA256 accepts the common sidecar forms:
//
//	<64 hex>
//	<64 hex>  openki.exe
//	sha256:<64 hex>
//	SHA256 (openki.exe) = <64 hex>
//
// Exactly one digest must be present, so a file carrying two candidates cannot
// be interpreted in the attacker's favour.
func parseSHA256(body []byte) (string, error) {
	matches := sha256HexPattern.FindAllString(string(body), -1)
	if len(matches) != 1 {
		return "", fmt.Errorf("update checksum must contain exactly one SHA-256 digest")
	}
	return strings.ToLower(matches[0]), nil
}

// isPE checks the two-byte DOS header. A custom feed could otherwise point at
// a zip or an HTML error page whose sidecar happens to match; only a real PE
// may replace the running exe.
func isPE(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false, err
	}
	return magic[0] == 'M' && magic[1] == 'Z', nil
}

// Apply performs the download-and-swap. After a true return the caller should
// start the new exe (Relaunch) and exit.
func Apply(downloadURL, wantVersion string, client *http.Client) (bool, error) {
	applyMu.Lock()
	defer applyMu.Unlock()
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	exePath, err := filepath.Abs(exe)
	if err != nil {
		return false, err
	}
	return applyTo(exePath, downloadURL, wantVersion, client)
}

// applyTo is the testable core: exePath is the binary to replace. Keeping the
// path out of Apply lets tests exercise the real swap on a temp file.
func applyTo(exePath, downloadURL, wantVersion string, client *http.Client) (bool, error) {
	if downloadURL == "" {
		return false, fmt.Errorf("no download url")
	}
	if len(downloadURL) < 8 || downloadURL[:8] != "https://" {
		return false, fmt.Errorf("update asset url must be https")
	}
	c := client
	if c == nil {
		c = &http.Client{}
	}
	want, err := fetchChecksum(c, downloadURL)
	if err != nil {
		return false, err
	}
	manifest, err := fetchManifest(c, downloadURL, wantVersion, want)
	if err != nil {
		return false, err
	}
	newPath := exePath + ".new-" + strconv.Itoa(os.Getpid())
	bakPath := exePath + ".bak"

	// Download to <exe>.new-<pid>. The request deadline rides the context
	// because a caller-provided proxy client may have no Timeout of its own.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("update download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxExeSize {
		return false, fmt.Errorf("update is larger than the %d-byte limit", maxExeSize)
	}
	f, err := os.Create(newPath)
	if err != nil {
		return false, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxExeSize+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if err != nil {
		os.Remove(newPath)
		return false, err
	}
	if syncErr != nil {
		os.Remove(newPath)
		return false, syncErr
	}
	if closeErr != nil {
		os.Remove(newPath)
		return false, closeErr
	}
	if n < minExeSize {
		os.Remove(newPath)
		return false, fmt.Errorf("downloaded update is suspiciously small (%d bytes)", n)
	}
	if n > maxExeSize {
		os.Remove(newPath)
		return false, fmt.Errorf("downloaded update exceeds the %d-byte limit", n)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.Remove(newPath)
		return false, fmt.Errorf("update checksum mismatch: got %s, want %s", got, want)
	}
	if manifest.Size > 0 && n != manifest.Size {
		os.Remove(newPath)
		return false, fmt.Errorf("update size mismatch: got %d, manifest says %d", n, manifest.Size)
	}
	ok, err := isPE(newPath)
	if err != nil {
		os.Remove(newPath)
		return false, err
	}
	if !ok {
		os.Remove(newPath)
		return false, fmt.Errorf("update payload is not a Windows executable")
	}

	// Swap: running exe → .bak, .new → exe. Windows permits renaming a
	// running binary, which is what makes this work without a helper.
	if err := os.Rename(exePath, bakPath); err != nil {
		os.Remove(newPath)
		return false, fmt.Errorf("could not move the running exe aside: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// Roll the backup back so the install stays bootable.
		_ = os.Rename(bakPath, exePath)
		os.Remove(newPath)
		return false, fmt.Errorf("could not place the new exe: %w", err)
	}
	syncDir(filepath.Dir(exePath))
	return true, nil
}

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// Relaunch starts the freshly installed exe detached and returns an error if
// it could not be started. The caller exits afterwards.
func Relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(exe).Start()
}

func backupPaths() (exe, bak, old string, err error) {
	p, err := os.Executable()
	if err != nil {
		return "", "", "", err
	}
	exe, err = filepath.Abs(p)
	if err != nil {
		return "", "", "", err
	}
	return exe, exe + ".bak", exe + ".bak.old", nil
}

// RestoreBackup recovers an install whose swap was interrupted between the two
// renames: the exe is missing but the previous binary is still on disk. It is
// called before any other startup work so a half-swapped install can boot.
func RestoreBackup() {
	exe, _, _, err := backupPaths()
	if err != nil {
		return
	}
	restoreBackup(exe)
}

func restoreBackup(exe string) {
	bak, old := exe+".bak", exe+".bak.old"
	if _, err := os.Stat(exe); err == nil {
		return // a binary is present; nothing to restore
	}
	for _, p := range []string{bak, old} {
		if _, err := os.Stat(p); err == nil {
			if os.Rename(p, exe) == nil {
				return
			}
		}
	}
}

// CleanupBackup removes leftovers from a prior Apply. Partial downloads are
// always dropped. The previous binary is kept for one restart: .bak becomes
// .bak.old now, and .bak.old is deleted on the next start. That gives the new
// build one boot cycle to prove itself before the rollback copy is gone.
func CleanupBackup() {
	exe, _, _, err := backupPaths()
	if err != nil {
		return
	}
	cleanupBackup(exe)
}

func cleanupBackup(exe string) {
	bak, old := exe+".bak", exe+".bak.old"
	if matches, _ := filepath.Glob(exe + ".new-*"); len(matches) > 0 {
		for _, p := range matches {
			_ = os.Remove(p)
		}
	}
	_ = os.Remove(old)
	if _, err := os.Stat(bak); err == nil {
		_ = os.Rename(bak, old)
	}
}
