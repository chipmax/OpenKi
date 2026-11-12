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
	"strings"
	"time"
)

// Apply downloads the new binary from a release asset and swaps it with the
// running one. Windows refuses to overwrite a running exe but allows renaming
// it, so the swap is: current → .bak, .new → current, spawn the new exe,
// exit. The caller exits right after Apply returns success; the .bak is
// cleaned up on the next boot (CleanupBackup).
//
// Only Windows reaches this path — see SelfUpdateSupported. Everywhere else
// the release is a bundle the user installs by hand, because replacing a
// signed .app's inner binary in place would break the signature.
//
// download is the release asset URL (https only — the whole point is that the
// payload comes from the GitHub release this update check just authenticated
// against indirectly). A sibling <asset>.sha256 must carry the expected digest;
// the release workflow publishes it. Requiring it turns a truncated download
// or a swapped asset into a refused update instead of a corrupted install.
//
// The checksum travels over the same channel as the binary, so it is an
// integrity gate, not a signature against a compromised release account. The
// admin CSRF fence is what prevents a web page from changing the feed; this
// gate keeps a truncated or CDN-swapped asset from being installed.

const (
	minExeSize  = 1 << 20   // 1 MiB sanity floor
	maxExeSize  = 256 << 20 // 256 MiB ceiling: no release is anywhere near it
	sha256HexRE = `(?i)\b[0-9a-f]{64}\b`
)

var sha256HexPattern = regexp.MustCompile(sha256HexRE)

// checksumURL is the sibling asset the release workflow writes next to the
// binary: openki.exe + ".sha256". Keeping the digest beside the asset means a
// custom feed has the same requirement as the bundled GitHub feed.
func checksumURL(downloadURL string) (string, error) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return "", fmt.Errorf("invalid update asset url: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("update checksum url must be https")
	}
	u.Path += ".sha256"
	return u.String(), nil
}

// fetchChecksum downloads and parses the sibling digest before the executable
// is touched. A missing checksum is a refusal, not a warning: an unverifiable
// binary must never replace the running gateway.
func fetchChecksum(client *http.Client, downloadURL string) (string, error) {
	cu, err := checksumURL(downloadURL)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cu, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch update checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("update checksum returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil {
		return "", fmt.Errorf("read update checksum: %w", err)
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

// Apply performs the download-and-swap. After a true return the caller should
// start the new exe (Relaunch) and exit.
func Apply(downloadURL string, client *http.Client) (bool, error) {
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
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	exePath, err := filepath.Abs(exe)
	if err != nil {
		return false, err
	}
	newPath := exePath + ".new"
	bakPath := exePath + ".bak"

	// Download to <exe>.new. The request deadline rides the context because a
	// caller-provided proxy client may have no Timeout of its own.
	timeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	closeErr := f.Close()
	if err != nil {
		os.Remove(newPath)
		return false, err
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
		return false, fmt.Errorf("downloaded update exceeds the %d-byte limit", maxExeSize)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.Remove(newPath)
		return false, fmt.Errorf("update checksum mismatch: got %s, want %s", got, want)
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
	return true, nil
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

// CleanupBackup removes the previous binary left by a prior Apply.
func CleanupBackup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	p, err := filepath.Abs(exe)
	if err != nil {
		return
	}
	_ = os.Remove(p + ".bak")
	_ = os.Remove(p + ".new")
}
