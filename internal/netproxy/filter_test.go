package netproxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestFilterWithholdsOnlySecurityFile(t *testing.T) {
	t.Chdir(t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "upstream "+r.URL.Path)
	}))
	defer upstream.Close()
	p, err := NewFilter()
	if err != nil {
		t.Fatal(err)
	}
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
	get := func(endpoint string) string {
		t.Helper()
		response, err := client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s: status=%d err=%v", endpoint, response.StatusCode, err)
		}
		return string(body)
	}
	// Answered by the proxy itself: no upstream is reachable under these names.
	for _, endpoint := range []string{"http://globaldp-prod-cn01.bhsr.com/query_security_file?file_key=CNPRODWin4.6.0", "https://globaldp-prod-cn01.bhsr.com/query_security_file?file_key=CNPRODWin4.6.0"} {
		if body := get(endpoint); body != "" {
			t.Fatalf("security file not withheld: %s %q", endpoint, body)
		}
	}
	// Everything else reaches its original destination unchanged.
	if body := get(upstream.URL + "/query_dispatch"); body != "upstream /query_dispatch" {
		t.Fatalf("request was not passed through: %q", body)
	}
}
