package alloy

const (
	Label     = "com.grafana.alloy"
	UIBase    = "http://127.0.0.1:12345"
	ReadyURL  = UIBase + "/-/ready"
	ReloadURL = UIBase + "/-/reload"
)

// Paths holds every filesystem location the collector touches. Values are
// system-wide (/etc, /usr/local/bin, /var/lib, /var/log, /Library/LaunchDaemons)
// so the install matches Homebrew / .deb / .rpm conventions and vendor UIs
// that tell users to "paste into /etc/alloy/config.alloy" just work.
//
// Writes to these paths require sudo; see internal/alloy/sudo.go.
type Paths struct {
	BinDir     string
	BinPath    string
	ConfigDir  string
	ConfigFile string
	DataDir    string
	LogDir     string
	StderrLog  string
	StdoutLog  string
	AgentDir   string
	AgentPath  string
}

func ResolvePaths() (Paths, error) {
	return Paths{
		BinDir:     "/usr/local/bin",
		BinPath:    "/usr/local/bin/alloy",
		ConfigDir:  "/etc/alloy",
		ConfigFile: "/etc/alloy/config.alloy",
		DataDir:    "/var/lib/alloy/data",
		LogDir:     "/var/log/alloy",
		StderrLog:  "/var/log/alloy/stderr.log",
		StdoutLog:  "/var/log/alloy/stdout.log",
		AgentDir:   "/Library/LaunchDaemons",
		AgentPath:  "/Library/LaunchDaemons/" + Label + ".plist",
	}, nil
}
