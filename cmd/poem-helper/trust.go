//go:build windows

package main

import (
	"encoding/pem"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// installTrust adds the proxy root CA to CurrentUser\Root of this Wine prefix.
// certutil is not part of Wine, so the store is written through crypt32.
func installTrust(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("证书文件不是 PEM 格式")
	}
	root, _ := windows.UTF16PtrFromString("ROOT")
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0, windows.CERT_SYSTEM_STORE_CURRENT_USER, uintptr(unsafe.Pointer(root)))
	if err != nil {
		return fmt.Errorf("CertOpenStore: %w", err)
	}
	defer windows.CertCloseStore(store, 0)
	ctx, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, &block.Bytes[0], uint32(len(block.Bytes)))
	if err != nil {
		return fmt.Errorf("CertCreateCertificateContext: %w", err)
	}
	defer windows.CertFreeCertificateContext(ctx)
	if err := windows.CertAddCertificateContextToStore(store, ctx, windows.CERT_STORE_ADD_REPLACE_EXISTING, nil); err != nil {
		return fmt.Errorf("CertAddCertificateContextToStore: %w", err)
	}
	return nil
}
