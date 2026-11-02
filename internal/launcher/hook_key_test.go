package launcher

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"os"
	"poem/internal/netproxy"
	"testing"
)

func TestHookKeyMatchesProxyKeyWithoutChangingLayout(t *testing.T) {
	// ProxyHook.dll is a prebuilt module and is not distributed with this repository.
	hook, err := os.ReadFile("../../assets/ProxyHook.dll")
	if err != nil {
		t.Skip("assets/ProxyHook.dll not present")
	}
	original, err := configureHookRSA(hook, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, hook) {
		t.Fatal("default key changed original DLL")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	encoded := base64.StdEncoding.EncodeToString(der)
	patched, err := configureHookRSA(hook, encoded)
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Modulus string `xml:"Modulus"`
	}
	xml.Unmarshal([]byte(netproxy.DefaultRSAKey), &old)
	start := bytes.Index(hook, wideBytes(old.Modulus))
	end := start + len(wideBytes(old.Modulus))
	if len(patched) != len(hook) || !bytes.Equal(patched[:start], hook[:start]) || !bytes.Equal(patched[end:], hook[end:]) {
		t.Fatal("patch modified data outside modulus")
	}
	if !bytes.Equal(patched[start:end], wideBytes(base64.StdEncoding.EncodeToString(key.N.Bytes()))) {
		t.Fatal("SDK key differs from proxy key")
	}
	if _, err := configureHookRSA([]byte("wrong DLL"), encoded); err == nil {
		t.Fatal("accepted unknown DLL")
	}
}
