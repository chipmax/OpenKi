package update

// Source-build update check. The public releases feed cannot describe a binary
// compiled from a checkout: this fork is installed with
// `CGO_ENABLED=0 go build -tags headless`, and a release asset may not exist
// yet. This file answers the two questions such an install can actually act on:
// which commit is running, and how far the local checkout is behind GitHub.

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// SourceRepo/SourceBranch name the upstream this fork tracks. They are
// variables so a fork can point the dashboard at its own repository without
// editing the rendering code.
var (
	SourceRepo   = "chipmax/loimo"
	SourceBranch = "master"
)

// Build identifies the running binary. Revision/Time/Modified come from the
// Go toolchain's VCS stamping; they are empty when the build had no VCS.
type Build struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Time     string `json:"time,omitempty"`
	Modified bool   `json:"modified,omitempty"`
}

// BuildInfo reads the running binary's build metadata once per call. The
// version is the gateway's linked version, while Revision is the exact commit
// the binary was compiled from.
func BuildInfo(version string) Build {
	b := Build{Version: strings.TrimSpace(version)}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = strings.TrimSpace(s.Value)
		case "vcs.time":
			b.Time = strings.TrimSpace(s.Value)
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	return b
}

// SourceStatus is the payload behind the dashboard's update page. Error is
// part of the payload on purpose: an unreachable GitHub must still let the
// page show the local build and the manual update commands.
type SourceStatus struct {
	Repo          string `json:"repo"`
	Branch        string `json:"branch"`
	Version       string `json:"version"`
	Current       string `json:"current,omitempty"`
	CurrentTime   string `json:"currentTime,omitempty"`
	CurrentDirty  bool   `json:"currentDirty,omitempty"`
	Latest        string `json:"latest,omitempty"`
	LatestShort   string `json:"latestShort,omitempty"`
	LatestSubject string `json:"latestSubject,omitempty"`
	LatestDate    string `json:"latestDate,omitempty"`
	LatestURL     string `json:"latestUrl,omitempty"`
	// Behind is the number of commits the checkout is behind. -1 means the
	// server could not compare (unknown local commit or missing remote), so the
	// UI must not call it "up to date".
	Behind      int    `json:"behind"`
	UpToDate    bool   `json:"upToDate"`
	CheckedAt   string `json:"checkedAt"`
	SourceDir   string `json:"sourceDir,omitempty"`
	SourceFound bool   `json:"sourceFound"`
	Error       string `json:"error,omitempty"`
}

// SourceDir finds a checkout without a shell command: the environment override
// first, then the layouts this project is normally cloned into. The service
// runs with the install user's HOME, so the usual ~/src path resolves.
func SourceDir() string {
	candidates := []string{}
	if v := strings.TrimSpace(os.Getenv("LOIMO_SOURCE")); v != "" {
		candidates = append(candidates, v)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, "src", "loimo"),
			filepath.Join(home, "loimo"),
			filepath.Join(home, "src", "loimo-source"),
			filepath.Join(home, "loimo-source"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	for _, dir := range candidates {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ""
}

// CheckSource compares the running build's commit with the branch head on
// GitHub. It never returns an error: transport and parse failures are carried
// in SourceStatus.Error so the page still renders local facts.
func CheckSource(client getJSONer, build Build) SourceStatus {
	sourceDir := SourceDir()
	st := SourceStatus{
		Repo:         SourceRepo,
		Branch:       SourceBranch,
		Version:      build.Version,
		Current:      build.Revision,
		CurrentTime:  build.Time,
		CurrentDirty: build.Modified,
		Behind:       -1,
		CheckedAt:    time.Now().Format(time.RFC3339),
		SourceDir:    sourceDir,
		SourceFound:  sourceDir != "",
	}
	if strings.TrimSpace(SourceRepo) == "" || strings.TrimSpace(SourceBranch) == "" {
		st.Error = "Chưa cấu hình repository nguồn"
		return st
	}
	if client == nil {
		st.Error = "Không có HTTP client để kiểm tra GitHub"
		return st
	}

	repoURL := "https://api.github.com/repos/" + repoPath(SourceRepo)
	if st.Current != "" {
		u := repoURL + "/compare/" + url.PathEscape(st.Current) + "..." + url.PathEscape(SourceBranch)
		data, err := client.Get(u)
		if err == nil {
			var cmp struct {
				AheadBy  int `json:"ahead_by"`
				BehindBy int `json:"behind_by"`
				Commits  []struct {
					SHA     string `json:"sha"`
					HTMLURL string `json:"html_url"`
					Commit  struct {
						Message string `json:"message"`
						Author  struct {
							Date string `json:"date"`
						} `json:"author"`
					} `json:"commit"`
				} `json:"commits"`
			}
			if err := json.Unmarshal(data, &cmp); err != nil {
				st.Error = "GitHub trả về dữ liệu so sánh không hợp lệ"
			} else {
				st.Behind = cmp.AheadBy
				st.UpToDate = cmp.AheadBy == 0 && cmp.BehindBy == 0
				if n := len(cmp.Commits); n > 0 {
					last := cmp.Commits[n-1]
					st.Latest = last.SHA
					st.LatestShort = shortSHA(last.SHA)
					st.LatestSubject = firstLine(last.Commit.Message)
					st.LatestDate = last.Commit.Author.Date
					st.LatestURL = last.HTMLURL
				}
				if st.Latest == "" {
					// Up to date: compare omits the commits array. The local
					// revision is the authoritative latest head in that case.
					st.Latest = st.Current
					st.LatestShort = shortSHA(st.Current)
					st.LatestURL = "https://github.com/" + SourceRepo + "/commit/" + st.Current
				}
				return st
			}
		} else if !isNotFound(err) {
			// Transport failure or a non-404 status: there is no point issuing
			// the head request, which would just hit the same dead proxy and
			// double the page's wait. Keep the local facts and surface the
			// error.
			st.Error = err.Error()
			return st
		}
	}

	// No local VCS stamp, or the local commit is not on the upstream (a
	// rebased checkout / a release built elsewhere): still show the head so the
	// user can compare manually, but leave Behind at -1.
	u := repoURL + "/commits/" + url.PathEscape(SourceBranch)
	data, err := client.Get(u)
	if err != nil {
		if strings.TrimSpace(st.Error) == "" {
			st.Error = err.Error()
		}
		return st
	}
	var head struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		st.Error = "GitHub trả về dữ liệu commit không hợp lệ"
		return st
	}
	st.Latest = head.SHA
	st.LatestShort = shortSHA(head.SHA)
	st.LatestSubject = firstLine(head.Commit.Message)
	st.LatestDate = head.Commit.Author.Date
	st.LatestURL = head.HTMLURL
	if st.Current != "" && strings.EqualFold(st.Current, head.SHA) {
		st.Behind = 0
		st.UpToDate = true
	}
	return st
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// repoPath keeps the owner/repository slash structural while escaping each
// segment: PathEscape on the whole string would produce %2F and no such API
// route.
func repoPath(repo string) string {
	parts := strings.SplitN(strings.Trim(repo, "/"), "/", 2)
	if len(parts) != 2 {
		return url.PathEscape(strings.Trim(repo, "/"))
	}
	return url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}
