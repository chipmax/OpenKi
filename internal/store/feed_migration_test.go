package store

import (
	"os"
	"path/filepath"
	"testing"
)

// A v13 install already points at the old fork repository, so the v13
// migration cannot repair it: the schema version is already past the branch.
// v14 must rewrite exactly those feed defaults and leave a custom feed alone.
func TestOpenMigratesLegacyFeedURLs(t *testing.T) {
	const (
		wantUpdate = "https://api.github.com/repos/chipmax/loimo/releases/latest"
		wantAnn    = "https://api.github.com/repos/chipmax/loimo/contents/announcements.json?ref=master"
	)
	cases := []struct {
		name       string
		updateFeed string
		annFeed    string
		wantCustom bool
	}{
		{
			name:       "legacy fork",
			updateFeed: "https://api.github.com/repos/chipmax/zen-gate-vi/releases/latest",
			annFeed:    "https://api.github.com/repos/chipmax/zen-gate-vi/contents/announcements.json?ref=master",
		},
		{
			name:       "upstream",
			updateFeed: "https://api.github.com/repos/LAGcomcom/zen-gate/releases/latest",
			annFeed:    "https://raw.githubusercontent.com/LAGcomcom/zen-gate/master/announcements.json",
		},
		{
			name:       "custom",
			updateFeed: "https://example.test/releases/latest",
			annFeed:    "https://example.test/announcements.json",
			wantCustom: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("LOIMO_HOME", home)
			raw := `{"mainKey":"sk-test","schemaVersion":13,"updateFeed":"` + tc.updateFeed +
				`","announcementFeed":"` + tc.annFeed + `"}`
			if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			st, err := Open()
			if err != nil {
				t.Fatal(err)
			}
			cfg := st.Config()
			if cfg.SchemaVersion != 14 {
				t.Fatalf("schemaVersion = %d, want 14", cfg.SchemaVersion)
			}
			if tc.wantCustom {
				if cfg.UpdateFeed != tc.updateFeed || cfg.AnnouncementFeed != tc.annFeed {
					t.Fatalf("custom feeds were rewritten: %+v", cfg)
				}
				return
			}
			if cfg.UpdateFeed != wantUpdate || cfg.AnnouncementFeed != wantAnn {
				t.Fatalf("feeds = %q, %q; want %q, %q", cfg.UpdateFeed, cfg.AnnouncementFeed, wantUpdate, wantAnn)
			}
		})
	}
}
