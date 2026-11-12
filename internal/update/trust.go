package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// ReleasePublicKey is the base64 Ed25519 public key that signs release
// manifests. It is public; the private key lives only in the release
// workflow's OPENKI_UPDATE_SIGNING_KEY secret. Forks must replace this value
// with their own key (tools/keygen prints a matching pair).
var ReleasePublicKey = "DnXCHCZoJaa6+ueCVBm4dPbe2aLdkkOxIUn7JHDq0co="

// Manifest is the signed statement binding a version to an asset and its
// digest. The signature covers these exact bytes, so an attacker cannot edit
// the manifest without the private key.
type Manifest struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

func manifestURL(downloadURL string) (string, error) {
	return siblingURL(downloadURL, ".manifest")
}

func signatureURL(downloadURL string) (string, error) {
	return siblingURL(downloadURL, ".manifest.sig")
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func assetBase(downloadURL string) string {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return path.Base(downloadURL)
	}
	return path.Base(u.Path)
}

func verifyManifest(manifest, sig []byte) error {
	if strings.TrimSpace(ReleasePublicKey) == "" {
		return fmt.Errorf("update signing key is not configured")
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ReleasePublicKey))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("update signing key is invalid")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("update signature has the wrong length")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), manifest, sig) {
		return fmt.Errorf("update signature is invalid")
	}
	return nil
}

// fetchManifest downloads and verifies the signed manifest before the binary
// is touched. wantDigest is the checksum sidecar's digest; the manifest must
// agree with it so the two artifacts cannot disagree silently.
func fetchManifest(client *http.Client, downloadURL, wantVersion, wantDigest string) (Manifest, error) {
	var m Manifest
	mu, err := manifestURL(downloadURL)
	if err != nil {
		return m, err
	}
	su, err := signatureURL(downloadURL)
	if err != nil {
		return m, err
	}
	body, err := (HTTP{Client: client, Timeout: 30 * time.Second}).Get(mu)
	if err != nil {
		return m, fmt.Errorf("fetch update manifest: %w", err)
	}
	if len(body) > 64<<10 {
		return m, fmt.Errorf("update manifest is too large")
	}
	sig, err := (HTTP{Client: client, Timeout: 30 * time.Second}).Get(su)
	if err != nil {
		return m, fmt.Errorf("fetch update signature: %w", err)
	}
	if err := verifyManifest(body, sig); err != nil {
		return m, err
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return m, fmt.Errorf("parse update manifest: %w", err)
	}
	if normalizeVersion(m.Version) == "" || !strings.EqualFold(normalizeVersion(m.Version), normalizeVersion(wantVersion)) {
		return m, fmt.Errorf("update manifest version %q does not match feed version %q", m.Version, wantVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(m.Asset), assetBase(downloadURL)) {
		return m, fmt.Errorf("update manifest asset %q does not match %q", m.Asset, assetBase(downloadURL))
	}
	if !strings.EqualFold(strings.TrimSpace(m.SHA256), wantDigest) {
		return m, fmt.Errorf("update manifest digest does not match the checksum sidecar")
	}
	return m, nil
}
