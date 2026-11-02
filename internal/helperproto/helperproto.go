// Package helperproto is the contract between the macOS launcher and
// poem-helper.exe, which runs inside the Wine prefix next to the game.
// The launcher writes a Plan as JSON; the helper reports Events as JSON lines
// on stdout. All paths in a Plan are Windows paths as seen inside the prefix.
package helperproto

// Plan describes one game launch.
type Plan struct {
	Game  string            `json:"game"`
	Early []string          `json:"early"` // loaded before the main thread resumes
	Late  []string          `json:"late"`  // loaded after the Unity window is ready
	Env   map[string]string `json:"env"`
	// TrustCert is a PEM root certificate added to the prefix's
	// CurrentUser\Root store before the game starts. Empty skips it.
	TrustCert string `json:"trustCert"`
	// BlurFPS > 0 publishes the de-blur module's shared configuration.
	BlurFPS int `json:"blurFPS"`
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
