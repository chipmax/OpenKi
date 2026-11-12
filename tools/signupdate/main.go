// Command signupdate signs an update manifest with the Ed25519 private key.
// The key comes from -key or OPENKI_UPDATE_SIGNING_KEY (base64). The signature
// is written as raw 64 bytes to -out.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	keyFile := flag.String("key", "", "file containing the base64 private key")
	in := flag.String("in", "", "manifest file")
	out := flag.String("out", "", "signature file")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "signupdate: -in and -out are required")
		os.Exit(2)
	}
	key := strings.TrimSpace(os.Getenv("OPENKI_UPDATE_SIGNING_KEY"))
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		key = strings.TrimSpace(string(b))
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "signupdate: no key (set OPENKI_UPDATE_SIGNING_KEY or -key)")
		os.Exit(2)
	}
	priv, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		fmt.Fprintln(os.Stderr, "signupdate: key is not a base64 Ed25519 private key")
		os.Exit(2)
	}
	manifest, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), manifest)
	if err := os.WriteFile(*out, sig, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
