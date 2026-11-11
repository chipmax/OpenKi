package lane

import "testing"

func TestClassifyRelayProtection(t *testing.T) {
	e := ClassifyFailure(401, `{"error":"Authentication Required: This deployment is protected by Vercel Authentication."}`, 0)
	if e.Code != CodeDeploymentProtected {
		t.Fatalf("code = %q, want %q", e.Code, CodeDeploymentProtected)
	}
	if switchableFailure(e) {
		t.Fatal("a protected deployment must not spend failover attempts")
	}
}

func TestClassifyRelayTokenAndEgress(t *testing.T) {
	unauth := ClassifyFailure(401, `{"error":"unauthorized relay request"}`, 0)
	if unauth.Code != CodeRelayUnauthorized {
		t.Fatalf("relay token code = %q", unauth.Code)
	}
	if switchableFailure(unauth) {
		t.Fatal("a bad relay token is not a model failure")
	}
	spent := ClassifyFailure(502, `{"error":"the address of this instance is used up"}`, 0)
	if spent.Code != CodeEgressRefused {
		t.Fatalf("egress code = %q", spent.Code)
	}
	if !egressRetryable(spent.Code) || !switchableFailure(spent) {
		t.Fatal("an exhausted exit should be retryable on another exit/model")
	}
}

func TestClassifyOverloadedIsTransient(t *testing.T) {
	e := ClassifyFailure(503, `{"error":{"type":"overloaded_error","message":"server is busy"}}`, 0)
	if e.Code != CodeOverloaded {
		t.Fatalf("code = %q, want %q", e.Code, CodeOverloaded)
	}
	if e.RetryAfter != 2 {
		t.Fatalf("RetryAfter = %d, want a 2s jitter hint", e.RetryAfter)
	}
	if !egressRetryable(e.Code) || !switchableFailure(e) {
		t.Fatal("overloaded should be retryable but never a model verdict")
	}
}

func TestPlainUnauthorizedStaysCredential(t *testing.T) {
	e := ClassifyFailure(401, `{"error":{"message":"Incorrect API key provided"}}`, 0)
	if e.Code != CodeCredential {
		t.Fatalf("code = %q, want %q", e.Code, CodeCredential)
	}
	e = ClassifyFailure(401, `{"error":"Authentication Required"}`, 0)
	if e.Code != CodeCredential {
		t.Fatalf("generic 401 became %q, want credential", e.Code)
	}
}

// The six bodies that shaped the egress policy, in one table so a future
// regexp edit cannot silently move quota, capacity or relay failures into the
// wrong branch.
func TestClassifyLiveFailureBodies(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		want       string
		switchable bool
	}{
		{
			name:       "quota type at error.type",
			status:     429,
			body:       `{"error":{"type":"FreeUsageLimitError","message":"Rate limit exceeded. Please try again later."}}`,
			want:       CodeQuota,
			switchable: true,
		},
		{
			name:       "quota wrapped by a top-level type",
			status:     429,
			body:       `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Rate limit exceeded."}}`,
			want:       CodeQuota,
			switchable: true,
		},
		{
			name:       "quota message only",
			status:     429,
			body:       `{"error":{"message":"Rate limit exceeded, try again later"}}`,
			want:       CodeQuota,
			switchable: true,
		},
		{
			name:       "overloaded is transient",
			status:     529,
			body:       `{"error":{"type":"overloaded_error","message":"server is busy"}}`,
			want:       CodeOverloaded,
			switchable: true,
		},
		{
			name:   "protected relay deployment",
			status: 401,
			body:   `{"error":"Authentication Required: This deployment is protected by Vercel Authentication."}`,
			want:   CodeDeploymentProtected,
		},
		{
			name:   "wrong relay token",
			status: 401,
			body:   `{"error":"unauthorized relay request"}`,
			want:   CodeRelayUnauthorized,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := ClassifyFailure(c.status, c.body, 0)
			if e.Code != c.want {
				t.Fatalf("code = %q, want %q", e.Code, c.want)
			}
			if got := switchableFailure(e); got != c.switchable {
				t.Fatalf("switchable = %v, want %v", got, c.switchable)
			}
			if e.Code == CodeQuota && e.RetryAfter != quotaCooldownNoHeaderSec {
				t.Fatalf("quota RetryAfter = %d, want %d", e.RetryAfter, quotaCooldownNoHeaderSec)
			}
			if e.Code == CodeOverloaded && e.RetryAfter == 0 {
				t.Fatal("overloaded response lost its retry hint")
			}
		})
	}
}
