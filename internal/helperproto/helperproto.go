// Package helperproto is the contract between the macOS launcher and
// poem-helper.exe, which runs inside the Wine prefix next to the game.
// The launcher writes a Plan as JSON; the helper reports Events as JSON lines
// on stdout. All paths in a Plan are Windows paths as seen inside the prefix.
package helperproto

import "strings"

// Plan describes one game launch.
type Plan struct {
	Game string   `json:"game"`
	Args []string `json:"args"`
	// Wrapper, when set, is jadeite.exe: the game is started through it with
	// Args and no modules are loaded (Early, Late and BlurFPS must be empty).
	Wrapper string            `json:"wrapper"`
	Early   []string          `json:"early"` // loaded before the main thread resumes
	Late    []string          `json:"late"`  // loaded after the Unity window is ready
	Env     map[string]string `json:"env"`
	// TrustCert is a PEM root certificate added to the prefix's
	// CurrentUser\Root store before the game starts. Empty skips it.
	TrustCert string `json:"trustCert"`
	// BlurFPS > 0 publishes the de-blur module's shared configuration.
	BlurFPS int `json:"blurFPS"`
	// Registry is written before the game starts.
	Registry []RegValue `json:"registry"`
}

// Registry value kinds.
const (
	RegString = "string"
	RegDWord  = "dword"
	RegBinary = "binary"
	RegDelete = "delete" // remove the value if present
	// RegPrepend makes String the first entry of a REG_MULTI_SZ value and
	// keeps the existing entries.
	RegPrepend = "prepend"
)

// PrependEntry puts entry first in list, dropping copies of it (ignoring case).
func PrependEntry(list []string, entry string) []string {
	result := []string{entry}
	for _, item := range list {
		if !strings.EqualFold(item, entry) {
			result = append(result, item)
		}
	}
	return result
}

// RegValue is one value under HKCU or HKLM. Key starts with HKCU\ or HKLM\.
type RegValue struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	String string `json:"string,omitempty"`
	DWord  uint32 `json:"dword,omitempty"`
	Binary []byte `json:"binary,omitempty"`
}

// Event kinds.
const (
	EventLog     = "log"
	EventStarted = "started"
	EventError   = "error"
	EventExited  = "exited"
)

// Event is one stdout line from the helper.
type Event struct {
	Event   string `json:"event"`
	PID     uint32 `json:"pid,omitempty"`
	Code    uint32 `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
