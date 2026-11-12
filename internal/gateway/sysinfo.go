package gateway

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chipmax/loimo/internal/store"
)

// SystemReport is the host/process information shown on the dashboard. It is
// deliberately plain JSON-friendly data: the browser renders it, and the
// gateway must not shell out on every ten-second state poll. Platform-specific
// files fill the parts the standard library cannot provide.
type SystemReport struct {
	OS              string `json:"os"`
	OSVersion       string `json:"osVersion,omitempty"`
	Kernel          string `json:"kernel,omitempty"`
	Arch            string `json:"arch"`
	Platform        string `json:"platform"`
	Hostname        string `json:"hostname,omitempty"`
	User            string `json:"user,omitempty"`
	CPU             string `json:"cpu,omitempty"`
	CPUThreads      int    `json:"cpuThreads"`
	MemoryTotal     uint64 `json:"memoryTotal,omitempty"`
	MemoryAvailable uint64 `json:"memoryAvailable,omitempty"`
	GoVersion       string `json:"goVersion"`
	GOMAXPROCS      int    `json:"gomaxprocs"`
	PID             int    `json:"pid"`
	StartedAt       string `json:"startedAt"`
	UptimeSec       int64  `json:"uptimeSec"`
	WSL             bool   `json:"wsl,omitempty"`
	WSLDistro       string `json:"wslDistro,omitempty"`
	DataDir         string `json:"dataDir"`
	ConfigPath      string `json:"configPath"`
	Listen          string `json:"listen"`
	LANURL          string `json:"lanUrl,omitempty"`
	AgentScope      string `json:"agentScope,omitempty"`
}

// systemInfo collects the report for adminState. Static host details are
// cheap to read; memory availability is sampled here too so a dashboard
// refresh reflects the machine rather than only process memory.
func (s *Server) systemInfo(cfg *store.Config) SystemReport {
	rep := SystemReport{
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		PID:        os.Getpid(),
		StartedAt:  startedAt.Format("2006-01-02 15:04:05"),
		UptimeSec:  int64(time.Since(startedAt).Seconds()),
		DataDir:    s.Store.Home,
		ConfigPath: filepath.Join(s.Store.Home, "config.json"),
		Listen:     fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		LANURL:     s.LANBaseURL(),
		AgentScope: cfg.AgentScope,
	}
	if host, err := os.Hostname(); err == nil {
		rep.Hostname = host
	}
	if u, err := user.Current(); err == nil {
		rep.User = u.Username
	}
	if rep.User == "" {
		rep.User = os.Getenv("USERNAME")
		if rep.User == "" {
			rep.User = os.Getenv("USER")
		}
	}
	fillPlatform(&rep)
	if rep.OS == "" {
		rep.OS = runtime.GOOS
	}
	if rep.Platform == "" {
		rep.Platform = runtime.GOOS
	}
	if rep.CPUThreads == 0 {
		rep.CPUThreads = runtime.NumCPU()
	}
	return rep
}
