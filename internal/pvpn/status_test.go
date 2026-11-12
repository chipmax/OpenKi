package pvpn

import "testing"

func TestParseStatusReadsFieldsWithoutGuessing(t *testing.T) {
	out := "Status:   Connected\nServer:   JP#294\nCountry:  JP\nIP:       138.199.21.214\nProtocol: stealth\nDuration: 3h 30m 31s\nUpload:   17.1 MB\nDownload: 34.1 MB\n"
	st := ParseStatus(out)
	if !st.Connected {
		t.Fatal("Connected = false, want true")
	}
	if st.Server != "JP#294" || st.Country != "JP" || st.IP != "138.199.21.214" {
		t.Fatalf("parsed = %+v", st)
	}
	if st.Protocol != "stealth" || st.Duration != "3h 30m 31s" {
		t.Fatalf("protocol/duration = %q / %q", st.Protocol, st.Duration)
	}
	if st.Error != "" {
		t.Fatalf("unexpected error: %s", st.Error)
	}
}

func TestParseStatusDisconnectedIsNotConnected(t *testing.T) {
	st := ParseStatus("Status: disconnected\n")
	if st.Connected {
		t.Fatal(`"disconnected" contains "connected" but must not parse as connected`)
	}
	if st.Error != "" {
		t.Fatalf("a disconnected status is not an error: %s", st.Error)
	}
}

func TestParseStatusRejectsMissingStatusField(t *testing.T) {
	st := ParseStatus("Server: JP#294\n")
	if st.Error == "" {
		t.Fatal("output without a Status line must be an error, not a guessed state")
	}
}
