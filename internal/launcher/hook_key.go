package launcher

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"poem/internal/netproxy"
	"unicode/utf16"
)

// Environment variable names read by the prebuilt ProxyHook.dll. They are
// compiled into the DLL and must match it exactly.
const (
	hookEnvProxy     = "CYRENE_PROXY"
	hookEnvEc2bFile  = "CYRENE_EC2B_FILE"
	hookEnvWebTarget = "CYRENE_WEB_TARGET"
)

func wideBytes(s string) []byte {
	chars := utf16.Encode([]rune(s))
	b := make([]byte, len(chars)*2)
	for i, c := range chars {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

// ProxyHook embeds an SDK RSA key as well as the proxy's response key.
// Replace only the fixed-width modulus data; instruction bytes, NativeAOT
// string metadata, and layout are unchanged. Other DLL builds fail closed.
func configureHookRSA(hook []byte, input string) ([]byte, error) {
	normalized, err := netproxy.NormalizeRSAKey(input)
	if err != nil {
		return nil, err
	}
	der, _ := base64.StdEncoding.DecodeString(normalized)
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	key, ok := pub.(*rsa.PublicKey)
	if !ok || key.N.BitLen() != 2048 || key.E != 65537 {
		return nil, fmt.Errorf("当前代理模块的 SDK 密钥替换支持 2048 位 RSA 公钥，指数须为 65537 (AQAB)")
	}
	var template struct {
		Modulus string `xml:"Modulus"`
	}
	if err := xml.Unmarshal([]byte(netproxy.DefaultRSAKey), &template); err != nil {
		return nil, err
	}
	old := wideBytes(template.Modulus)
	replacement := wideBytes(base64.StdEncoding.EncodeToString(key.N.Bytes()))
	if len(old) != len(replacement) || bytes.Count(hook, old) != 1 {
		return nil, fmt.Errorf("代理模块中的 RSA 模板不匹配，无法安全应用自定义密钥")
	}
	at := bytes.Index(hook, old)
	if at < len(wideBytes("<Modulus>")) || !bytes.Equal(hook[at-len(wideBytes("<Modulus>")):at], wideBytes("<Modulus>")) {
		return nil, fmt.Errorf("代理模块 RSA 模板结构不匹配")
	}
	result := bytes.Clone(hook)
	copy(result[at:], replacement)
	return result, nil
}
