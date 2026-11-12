// Command keygen creates the Ed25519 key pair used to sign update manifests.
// It prints the public key (base64) and writes the private key (base64) to
// -out. The private key must never be committed: store it as the repository
// secret OPENKI_UPDATE_SIGNING_KEY.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
)

func main() {
	out := flag.String("out", "", "file for the base64 private key (required)")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "keygen: -out is required; refusing to print the private key")
		os.Exit(2)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
}
