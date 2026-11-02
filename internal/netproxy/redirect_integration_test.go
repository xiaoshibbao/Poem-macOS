package netproxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOfficialEndpointsRedirectBeforeClash(t *testing.T) {
	t.Chdir(t.TempDir())
	var mu sync.Mutex
	var logs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"source":"custom-server"}`)
	}))
	defer server.Close()
	p, err := New(server.URL, PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p.SetLogger(func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	port, err := p.Start(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	roots := x509.NewCertPool()
	roots.AddCert(p.ca.cert)
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	for _, endpoint := range []string{"http://globaldp-beta-os01.starrails.com/query_dispatch?ticket=secret-value", "https://globaldp-beta-os01.starrails.com/query_dispatch?ticket=secret-value", "https://hkrpg-beta-sdk-os.hoyoverse.com/login?ticket=secret-value"} {
		response, err := client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(body) != `{"source":"custom-server"}` {
			t.Fatalf("official endpoint escaped forwarding: %s %s %v", endpoint, body, err)
		}
	}
	transport.CloseIdleConnections()
	p.Stop()
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(logs, "\n")
	if strings.Contains(joined, "secret-value") {
		t.Fatal("query token leaked into log")
	}
	if strings.Count(joined, "proxy forwarded") != 3 {
		t.Fatal(joined)
	}
}
