package update

import (
	"errors"
	"strings"
	"testing"
)

type fakeFetcher struct {
	urls map[string]string
	err  error
}

func (f fakeFetcher) Get(u string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	body, ok := f.urls[u]
	if !ok {
		return nil, &statusError{Code: 404}
	}
	return []byte(body), nil
}

func TestCheckSourceComparesBuildCommitWithHead(t *testing.T) {
	oldRepo, oldBranch := SourceRepo, SourceBranch
	SourceRepo, SourceBranch = "owner/repo", "main"
	defer func() { SourceRepo, SourceBranch = oldRepo, oldBranch }()

	const current = "1111111111111111111111111111111111111111"
	const latest = "2222222222222222222222222222222222222222"
	f := fakeFetcher{urls: map[string]string{
		"https://api.github.com/repos/owner/repo/compare/" + current + "...main": `{
			"ahead_by": 2,
			"behind_by": 0,
			"commits": [
				{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","html_url":"https://github.com/owner/repo/commit/a","commit":{"message":"first\nbody","author":{"date":"2026-01-01T00:00:00Z"}}},
				{"sha":"` + latest + `","html_url":"https://github.com/owner/repo/commit/b","commit":{"message":"second\nbody","author":{"date":"2026-01-02T00:00:00Z"}}}
			]
		}`,
	}}
	st := CheckSource(f, Build{Version: "1.2.3", Revision: current})
	if st.Behind != 2 || st.UpToDate {
		t.Fatalf("behind=%d upToDate=%v, want 2/false", st.Behind, st.UpToDate)
	}
	if st.Latest != latest || st.LatestShort != latest[:7] {
		t.Fatalf("latest=%q short=%q", st.Latest, st.LatestShort)
	}
	if st.LatestSubject != "second" || st.LatestDate != "2026-01-02T00:00:00Z" {
		t.Fatalf("subject/date not parsed: %+v", st)
	}
	if st.Error != "" {
		t.Fatalf("unexpected error: %s", st.Error)
	}
}

func TestCheckSourceFallsBackToHeadWhenCompareIsUnknown(t *testing.T) {
	oldRepo, oldBranch := SourceRepo, SourceBranch
	SourceRepo, SourceBranch = "owner/repo", "main"
	defer func() { SourceRepo, SourceBranch = oldRepo, oldBranch }()

	const head = "3333333333333333333333333333333333333333"
	f := fakeFetcher{urls: map[string]string{
		"https://api.github.com/repos/owner/repo/commits/main": `{
			"sha":"` + head + `",
			"html_url":"https://github.com/owner/repo/commit/c",
			"commit":{"message":"head commit\nbody","author":{"date":"2026-02-03T04:05:06Z"}}
		}`,
	}}
	st := CheckSource(f, Build{Version: "1.2.3", Revision: "deadbeef"})
	if st.Behind != -1 {
		t.Fatalf("behind=%d, want -1 when compare is unavailable", st.Behind)
	}
	if st.Latest != head || st.LatestSubject != "head commit" {
		t.Fatalf("fallback head not parsed: %+v", st)
	}
}

func TestCheckSourceKeepsLocalFactsWhenGitHubFails(t *testing.T) {
	oldRepo, oldBranch := SourceRepo, SourceBranch
	SourceRepo, SourceBranch = "owner/repo", "main"
	defer func() { SourceRepo, SourceBranch = oldRepo, oldBranch }()

	st := CheckSource(fakeFetcher{err: errors.New("offline")}, Build{Version: "1.2.3", Revision: "abc"})
	if st.Current != "abc" || st.Version != "1.2.3" {
		t.Fatalf("local build facts were lost: %+v", st)
	}
	if !strings.Contains(st.Error, "offline") {
		t.Fatalf("transport error not surfaced: %q", st.Error)
	}
}
