// Package netproxy implements the Go-native private-server patch:
// a loopback HTTPS MITM proxy that intercepts miHoYo-domain traffic and
// forwards it to a configurable private server, together with helpers for
// managing the proxy root CA.
package netproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const caCN = "Poem Local Proxy"

// The CA files live in the launcher data directory; SetStorageDir moves them.
var (
	caCertFile = "./Poem-data/proxy/poem-ca.pem"
	caKeyFile  = "./Poem-data/proxy/poem-ca-key.pem"
)

// SetStorageDir must be called before the first proxy is created.
func SetStorageDir(dir string) {
	caCertFile = filepath.Join(dir, "poem-ca.pem")
	caKeyFile = filepath.Join(dir, "poem-ca-key.pem")
}

// caState holds the proxy root CA and a cache of per-hostname leaf certs.
type caState struct {
	cert    *x509.Certificate
	certPEM []byte
	key     *ecdsa.PrivateKey

	leafMu sync.Mutex
	leaves map[string]tls.Certificate
}

// getCA loads the proxy root CA from disk, or generates and persists a new one
// when none exists or the existing cert is about to expire.
func getCA() (*caState, error) {
	ca := &caState{leaves: make(map[string]tls.Certificate)}

	certPEM, certErr := os.ReadFile(caCertFile)
	keyPEM, keyErr := os.ReadFile(caKeyFile)
	if certErr == nil && keyErr == nil {
		if tlsCert, err := tls.X509KeyPair(certPEM, keyPEM); err == nil {
			if cert, err := x509.ParseCertificate(tlsCert.Certificate[0]); err == nil {
				if time.Now().Before(cert.NotAfter.Add(-48 * time.Hour)) {
					ca.cert = cert
					ca.certPEM = certPEM
					ca.key = tlsCert.PrivateKey.(*ecdsa.PrivateKey)
					return ca, nil
				}
			}
		}
	}

	// Generate a new root CA.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: caCN, Organization: []string{"Poem"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(caCertFile), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(caCertFile, certPEM, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(caKeyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		return nil, err
	}

	ca.cert = cert
	ca.certPEM = certPEM
	ca.key = key
	return ca, nil
}

// CertFile returns the PEM path of the root CA. The Wine helper installs it
// into the prefix's trusted root store, where the game looks for it.
func (ca *caState) CertFile() (string, error) {
	if _, err := os.Stat(caCertFile); err != nil {
		if err := os.MkdirAll(filepath.Dir(caCertFile), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(caCertFile, ca.certPEM, 0644); err != nil {
			return "", err
		}
	}
	return filepath.Abs(caCertFile)
}

// leafCert returns a cached (or freshly-generated) TLS leaf certificate for
// hostname, signed by the proxy CA.
func (ca *caState) leafCert(hostname string) (tls.Certificate, error) {
	ca.leafMu.Lock()
	if c, ok := ca.leaves[hostname]; ok {
		ca.leafMu.Unlock()
		return c, nil
	}
	ca.leafMu.Unlock()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		DNSNames:     []string{hostname},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, err
	}
	c := tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}

	ca.leafMu.Lock()
	ca.leaves[hostname] = c
	ca.leafMu.Unlock()
	return c, nil
}
