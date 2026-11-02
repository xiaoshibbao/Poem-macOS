package netproxy

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRSAFormatsAndResponseReplacement(t *testing.T) {
	normalized, err := NormalizeRSAKey("")
	if err != nil {
		t.Fatal(err)
	}
	der, _ := base64.StdEncoding.DecodeString(normalized)
	for _, input := range []string{DefaultRSAKey, normalized, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))} {
		got, err := NormalizeRSAKey(input)
		if err != nil || got != normalized {
			t.Fatalf("format normalization: %v", err)
		}
	}
	if _, err := NormalizeRSAKey("not a key"); err == nil {
		t.Fatal("accepted invalid key")
	}
	if _, err := x509.ParsePKIXPublicKey(der); err != nil {
		t.Fatal(err)
	}
	jsonBody := []byte(`{"rsa_public_key":"` + strings.Repeat("A", 128) + `"}`)
	got := patchRSAInBody(jsonBody, "application/json", normalized)
	var v map[string]string
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatal(err)
	}
	if v["rsa_public_key"] != normalized {
		t.Fatal("JSON replacement must be base64 DER")
	}
	xmlBody := []byte(`<RSAKeyValue><Modulus>old</Modulus><Exponent>AQAB</Exponent></RSAKeyValue>`)
	got = patchRSAInBody(xmlBody, "text/xml", normalized)
	roundtrip, err := NormalizeRSAKey(string(got))
	if err != nil || roundtrip != normalized {
		t.Fatalf("XML roundtrip: %v", err)
	}
}

func TestProxyForwardingPortFallbackAndShutdown(t *testing.T) {
	t.Chdir(t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query_dispatch" || r.URL.Query().Get("version") != "1" {
			t.Errorf("wrong forwarded URL %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"rsa_public_key":"`+strings.Repeat("A", 128)+`","url":"https://account.hoyoverse.com/login"}`)
	}))
	defer upstream.Close()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	p, err := New(upstream.URL, DefaultPatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	port, err := p.Start(occupied.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatal(err)
	}
	if port == occupied.Addr().(*net.TCPAddr).Port {
		t.Fatal("did not choose a free port")
	}
	u, _ := url.Parse("http://127.0.0.1:" + fmtPort(port))
	transport := &http.Transport{Proxy: http.ProxyURL(u)}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Get("http://dispatch.hoyoverse.com/query_dispatch?version=1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	transport.CloseIdleConnections()
	key, _ := NormalizeRSAKey("")
	if !strings.Contains(string(b), key) || !strings.Contains(string(b), upstream.URL+"/login") {
		t.Fatalf("response patches absent: %s", b)
	}
	idle, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop blocked on an idle client")
	}
}

func fmtPort(n int) string { return fmt.Sprint(n) }
