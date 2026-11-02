package launcher

import "fmt"

// Keep NativeAOT background GC out of the hook installer's thread-context operations.
// This affects only the launched process; regular foreground collection remains enabled.
func proxyRuntimeEnvironment(proxyEnabled bool) map[string]string {
	env := map[string]string{}
	if proxyEnabled {
		env["DOTNET_gcConcurrent"] = "0"
	}
	return env
}

// Supply proxy settings before the process executes any game/SDK code. The
// delayed native hooks alone cannot change clients which already cached a
// different proxy, and child SDK browsers inherit this environment as well.
func setGameHTTPProxy(env map[string]string, port int) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		env[key] = endpoint
	}
	// Do not inherit a wildcard or official-domain bypass from the parent.
	env["NO_PROXY"] = "localhost,127.0.0.1,::1"
}
