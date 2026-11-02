package netproxy

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"math/big"
	"strings"
)

type rsaXML struct {
	XMLName  xml.Name `xml:"RSAKeyValue"`
	Modulus  string   `xml:"Modulus"`
	Exponent string   `xml:"Exponent"`
}

// NormalizeRSAKey accepts XML, PEM or base64 DER public keys, returning SPKI.
func NormalizeRSAKey(input string) (string, error) {
	key, err := parseRSAKey(input)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

func parseRSAKey(input string) (*rsa.PublicKey, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		input = DefaultRSAKey
	}
	invalid := errors.New("RSA 密钥无效；请粘贴 XML、PEM 或 Base64 格式的 RSA 公钥")
	var key *rsa.PublicKey
	if strings.HasPrefix(input, "<") {
		var v rsaXML
		if xml.Unmarshal([]byte(input), &v) != nil {
			return nil, invalid
		}
		n, e1 := base64.StdEncoding.DecodeString(strings.TrimSpace(v.Modulus))
		e, e2 := base64.StdEncoding.DecodeString(strings.TrimSpace(v.Exponent))
		if e1 != nil || e2 != nil || len(e) > 4 {
			return nil, invalid
		}
		ei := 0
		for _, b := range e {
			ei = ei*256 + int(b)
		}
		key = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: ei}
	} else {
		var der []byte
		if block, _ := pem.Decode([]byte(input)); block != nil {
			der = block.Bytes
		} else {
			var err error
			der, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(input), ""))
			if err != nil {
				return nil, invalid
			}
		}
		if v, err := x509.ParsePKIXPublicKey(der); err == nil {
			key, _ = v.(*rsa.PublicKey)
		} else {
			key, _ = x509.ParsePKCS1PublicKey(der)
		}
	}
	if key == nil || key.N == nil || key.N.BitLen() < 1024 || key.E < 3 || key.E%2 == 0 {
		return nil, invalid
	}
	return key, nil
}

func rsaXMLFromInput(input string) string {
	key, err := parseRSAKey(input)
	if err != nil {
		return ""
	}
	exp := big.NewInt(int64(key.E)).Bytes()
	b, _ := xml.Marshal(rsaXML{Modulus: base64.StdEncoding.EncodeToString(key.N.Bytes()), Exponent: base64.StdEncoding.EncodeToString(exp)})
	return string(b)
}
