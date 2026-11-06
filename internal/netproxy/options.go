package netproxy

import "strings"

// DefaultRSAKey is the default public key used by Poem.
// Used when RSAPatch is enabled and no custom key is set.
const DefaultRSAKey = `<RSAKeyValue><Modulus>mzu3w4uXJtssxWuVGhakeLP9DNlYERlwWqFyaQGWCYceypvvPAZ1TMDAgdTXQ+F5bCCzmAxarm1Ls9LL4MY8dirJYHtCB4e86hOUHfVQ6zV+OS/B45/ngz6clWewOml8ZOCKfLodrVLD08ENBdx3oVuJqU/L/4y7OOorIyTqbbSTu/0CWxfK8M3NkH2wX7CZZRT4Rt8X2SUsmN3rmRrZCNfPpDJ0t23qDke5OEz7hOBbOOAnELUqGV2FDGieSNeYtdsiUuAn1b/vdNADkJ5LO2m3gWWpNvrbdkMBW6J6Zr/LERvi7/Aa/TpnaVvmdkHiE7+/q1Ik9PjxDN6SV9oG5w==</Modulus><Exponent>AQAB</Exponent></RSAKeyValue>`

// DefaultWebHosts is the WebRedirectHosts list from the reference project.
// URL rewriting in response bodies is applied to these exact hostnames only.
var DefaultWebHosts = []string{
	"account.hoyoverse.com",
	"user.hoyoverse.com",
	"sdk.hoyoverse.com",
	"webstatic.hoyoverse.com",
	"webstatic-sea.hoyoverse.com",
	"hkrpg-sdk-os-static.hoyoverse.com",
	"sdk-os-static.hoyoverse.com",
}

// PatchOptions controls the optional patching features of the MITM proxy.
// Zero value disables all patching; callers should use DefaultPatchOptions()
// to get the recommended defaults.
type PatchOptions struct {
	// RSAPatch enables replacement of RSA public key values in JSON/text responses.
	RSAPatch bool `json:"rsaPatch"`
	// RSAKey overrides the injected RSA key. Empty → DefaultRSAKey.
	RSAKey string `json:"rsaKey"`
	// WebRedirect enables URL rewriting in text/html/json/js responses.
	WebRedirect bool `json:"webRedirect"`
	// WebHosts is a comma- or newline-separated list of exact hostnames whose
	// URLs are rewritten. Empty → DefaultWebHosts.
	WebHosts string `json:"webHosts"`
}

// DefaultPatchOptions returns the recommended settings matching the reference project.
func DefaultPatchOptions() PatchOptions {
	return PatchOptions{RSAPatch: true, WebRedirect: true}
}

// ResolvedRSAKey returns the effective RSA key for body patching.
func (o PatchOptions) ResolvedRSAKey() string {
	if o.RSAKey != "" {
		return o.RSAKey
	}
	return DefaultRSAKey
}

// ResolvedWebHosts returns the effective host list for URL rewriting.
func (o PatchOptions) ResolvedWebHosts() []string {
	if o.WebHosts == "" {
		return DefaultWebHosts
	}
	parts := strings.FieldsFunc(o.WebHosts, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, h := range parts {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, strings.ToLower(h))
		}
	}
	return out
}
