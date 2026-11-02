package launcher

import "testing"

func TestRuntimeSettingsOnlyApplyToProxyProcess(t *testing.T) {
	if len(proxyRuntimeEnvironment(false)) != 0 {
		t.Fatal("runtime changed without proxy")
	}
	env := proxyRuntimeEnvironment(true)
	if len(env) != 1 || env["DOTNET_gcConcurrent"] != "0" {
		t.Fatal("invalid foreground GC configuration")
	}
}

func TestGameHTTPProxyOverridesInheritedClash(t *testing.T) {
	env := map[string]string{"HTTP_PROXY": "http://127.0.0.1:7890", "HTTPS_PROXY": "http://127.0.0.1:7890", "ALL_PROXY": "socks5://127.0.0.1:7890", "NO_PROXY": "*"}
	setGameHTTPProxy(env, 45678)
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		if env[key] != "http://127.0.0.1:45678" {
			t.Fatal(key, env[key])
		}
	}
	if env["NO_PROXY"] != "localhost,127.0.0.1,::1" {
		t.Fatal("inherited bypass still active")
	}
}
