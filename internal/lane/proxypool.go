package lane

// Static proxy pool: the opencode2api "proxies" mechanism, without its
// sing-box dependency. A pool is a fixed list of HTTP(S)/SOCKS5 endpoints that
// every dial is spread across, so the free tier sees several egress IPs
// instead of the machine's one.
//
// It plugs into the same Rotator interface as the subscription sidecar
// (internal/subs): Pick chooses one exit per conversation, Dial/DialThrough
// route a connection through it, and Report/ReportDead feed the same
// (exit, model) ledger semantics. That is deliberate — the lane already
// retries a 429 on another exit and derives the upstream session from the
// exit, so a second egress implementation with its own rules would make the
// same model behave differently depending on which way it left.
//
// The pool is chosen with proxyMode "pool"; an empty/invalid list is reported
// to the settings caller instead of silently degrading to a direct dial.

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

// poolNode is one configured exit. userinfo stays out of id: ids are logged,
// printed in the node table and embedded in session seeds, so a proxy password
// must never leak through them.
type poolNode struct {
	id     string
	scheme string
	addr   string
	user   *url.Userinfo
}

// ProxyPool implements Rotator over a static list of proxy URLs.
//
// Cooldowns mirror internal/subs: the ledger only removes candidates, never
// promotes a "known good" one; the first park is short (a failure is usually
// jitter), a rate limit parks longer, and a region verdict parks for minutes
// because it is a stable property of that exit. A served request clears the
// pair through Revive.
type ProxyPool struct {
	nodes []poolNode
	byID  map[string]int
	rr    atomic.Uint64

	mu       sync.Mutex
	fails    map[string]int
	failAt   map[string]time.Time
	banUntil map[string]time.Time

	deadFails  map[string]int
	deadFailAt map[string]time.Time
	deadUntil  map[string]time.Time
	deadParks  map[string]int
	sticky     map[string]string
	stickyAt   map[string]int64
}

const (
	poolBanBaseOther   = 3 * time.Second
	poolBanBaseLimited = 30 * time.Second
	poolBanRegion      = 15 * time.Minute
	poolBanCeilOther   = time.Minute
	poolBanCeilLimited = 10 * time.Minute
	poolFailMemory     = 10 * time.Minute
	poolDeadStrikes    = 2
	poolDeadBase       = time.Minute
	poolDeadCeil       = 15 * time.Minute
	// poolStickyConversations bounds the conversation→exit table; the least
	// recently used entry is evicted past this.
	poolStickyConversations = 512
)

// NewProxyPool parses the configured entries. "direct" is accepted as an
// explicit endpoint (useful as a last-resort entry); any malformed entry is an
// error, because silently dropping one turns an 8-exit pool into a 7-exit one
// nobody can see.
func NewProxyPool(entries []string) (*ProxyPool, error) {
	p := &ProxyPool{
		byID:       map[string]int{},
		fails:      map[string]int{},
		failAt:     map[string]time.Time{},
		banUntil:   map[string]time.Time{},
		deadFails:  map[string]int{},
		deadFailAt: map[string]time.Time{},
		deadUntil:  map[string]time.Time{},
		deadParks:  map[string]int{},
		sticky:     map[string]string{},
		stickyAt:   map[string]int64{},
	}
	seen := map[string]bool{}
	for i, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		node, err := parsePoolNode(raw)
		if err != nil {
			return nil, fmt.Errorf("proxy thứ %d (%s): %w", i+1, redactProxy(raw), err)
		}
		if seen[node.scheme+"\x00"+node.addr+"\x00"+userKey(node.user)] {
			continue // duplicates only shorten the pool; keep the first position
		}
		seen[node.scheme+"\x00"+node.addr+"\x00"+userKey(node.user)] = true
		node.id = fmt.Sprintf("pool-%d-%s://%s", i+1, node.scheme, node.addr)
		p.byID[node.id] = len(p.nodes)
		p.nodes = append(p.nodes, node)
	}
	if len(p.nodes) == 0 {
		return nil, errors.New("danh sách proxy rỗng")
	}
	return p, nil
}

// NormalizeProxyPool validates and trims a list for the settings writer. It
// returns the cleaned entries without dropping malformed ones.
func NormalizeProxyPool(entries []string) ([]string, error) {
	clean := make([]string, 0, len(entries))
	for i, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, err := parsePoolNode(raw); err != nil {
			return nil, fmt.Errorf("proxy thứ %d (%s): %w", i+1, redactProxy(raw), err)
		}
		clean = append(clean, raw)
	}
	return clean, nil
}

// parsePoolNode accepts direct, http(s):// and socks5(h)://. A missing port on
// an HTTP endpoint takes the scheme default; SOCKS5 has no standard default and
// requires one.
func parsePoolNode(raw string) (poolNode, error) {
	if strings.EqualFold(raw, "direct") {
		return poolNode{scheme: "direct"}, nil
	}
	if !strings.Contains(raw, "://") {
		return poolNode{}, errors.New("thiếu scheme (http://, https://, socks5://, socks5h:// hoặc direct)")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return poolNode{}, errors.New("URL không hợp lệ")
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return poolNode{}, errors.New("scheme không hỗ trợ: " + scheme)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return poolNode{}, errors.New("thiếu host")
	}
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return poolNode{}, errors.New("proxy SOCKS5 cần cổng")
		}
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return poolNode{}, errors.New("cổng không hợp lệ")
	}
	return poolNode{scheme: scheme, addr: net.JoinHostPort(host, port), user: u.User}, nil
}

func userKey(u *url.Userinfo) string {
	if u == nil {
		return ""
	}
	pass, _ := u.Password()
	return u.Username() + "\x00" + pass
}

// redactProxy keeps enough of an entry for an error message without echoing a
// password back into the dashboard/log.
func redactProxy(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		u.User = url.User(u.User.Username())
		return u.String()
	}
	return raw
}

func poolBanKey(nodeID, model string) string { return nodeID + "\x00" + model }

// usable reports whether one exit may serve a model now. Caller holds p.mu.
func (p *ProxyPool) usable(node poolNode, model string, now time.Time) bool {
	if t := p.deadUntil[node.id]; !t.IsZero() && now.Before(t) {
		return false
	}
	if t := p.banUntil[poolBanKey(node.id, model)]; !t.IsZero() && now.Before(t) {
		return false
	}
	return true
}

// Pick chooses an exit for one conversation. Sticky first (prompt-cache
// affinity), then a deterministic hash so a new conversation spreads across
// the pool without a global counter that would pin every restart to exit 1.
func (p *ProxyPool) Pick(seed, model string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.nodes) == 0 {
		return ""
	}
	now := time.Now()

	if seed != "" {
		if id, ok := p.sticky[seed]; ok {
			if i, found := p.byID[id]; found && p.usable(p.nodes[i], model, now) {
				p.stickyAt[seed] = now.UnixMilli()
				return id
			}
			delete(p.sticky, seed)
			delete(p.stickyAt, seed)
		}
	}

	candidates := make([]poolNode, 0, len(p.nodes))
	for _, n := range p.nodes {
		if p.usable(n, model, now) {
			candidates = append(candidates, n)
		}
	}
	// All exits cooling is not a reason to send the request out with no exit:
	// the retry budget is already spent either way, and a direct dial would
	// leak the machine's own IP. Fall back to the full list and let the
	// (exit, model) ledger decide after the attempt.
	if len(candidates) == 0 {
		candidates = p.nodes
	}

	var chosen poolNode
	if seed == "" {
		chosen = candidates[int(p.rr.Add(1)-1)%len(candidates)]
	} else {
		h := fnv.New64a()
		_, _ = h.Write([]byte(seed))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(model))
		chosen = candidates[int(h.Sum64()%uint64(len(candidates)))]
		if len(p.sticky) >= poolStickyConversations {
			p.evictOldestSticky()
		}
		p.sticky[seed] = chosen.id
		p.stickyAt[seed] = now.UnixMilli()
	}
	return chosen.id
}

// evictOldestSticky drops the least recently used conversation. Caller holds
// p.mu.
func (p *ProxyPool) evictOldestSticky() {
	oldest, oldestAt := "", int64(1<<62-1)
	for k, v := range p.stickyAt {
		if v < oldestAt {
			oldest, oldestAt = k, v
		}
	}
	if oldest != "" {
		delete(p.sticky, oldest)
		delete(p.stickyAt, oldest)
	}
}

// Healthy reports exits that are not parked for transport reasons. It does not
// run probes: a static endpoint is judged by real traffic, and the first dial
// failure is a cheaper signal than a synthetic request per boot.
func (p *ProxyPool) Healthy() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	n := 0
	for _, node := range p.nodes {
		if t := p.deadUntil[node.id]; t.IsZero() || !now.Before(t) {
			n++
		}
	}
	return n
}

// Dial connects through the next usable exit, trying every node once before
// giving up. Starting the walk at the round-robin cursor keeps concurrent
// dials (which do not carry an ExitPlan) spread instead of all retrying the
// same first entry.
func (p *ProxyPool) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if len(p.nodes) == 0 {
		return nil, errors.New("pool proxy rỗng")
	}
	// Try unparked exits first; a parked one is a last resort so a single dead
	// entry cannot burn the head of every fallback dial. The order still starts
	// at the round-robin cursor inside each group.
	now := time.Now()
	p.mu.Lock()
	order := make([]poolNode, 0, len(p.nodes))
	for _, n := range p.nodes {
		if t := p.deadUntil[n.id]; t.IsZero() || !now.Before(t) {
			order = append(order, n)
		}
	}
	for _, n := range p.nodes {
		if t := p.deadUntil[n.id]; !t.IsZero() && now.Before(t) {
			order = append(order, n)
		}
	}
	p.mu.Unlock()

	start := int(p.rr.Add(1)-1) % len(order)
	var lastErr error
	for off := range len(order) {
		n := order[(start+off)%len(order)]
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c, err := p.dialNode(ctx, n, network, addr)
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("không có proxy khả dụng")
	}
	return nil, lastErr
}

// DialThrough dials the exit named by an ExitPlan, so the session derived from
// that exit is the one actually used.
func (p *ProxyPool) DialThrough(ctx context.Context, network, addr, nodeID string) (net.Conn, error) {
	i, ok := p.byID[nodeID]
	if !ok {
		return nil, fmt.Errorf("proxy không tồn tại: %s", nodeID)
	}
	return p.dialNode(ctx, p.nodes[i], network, addr)
}

// ExitKey uses the node id: these are raw proxies, not sing-box nodes whose
// egress IP is probed. Distinct endpoints therefore get distinct sessions,
// which is the safe direction — two entries leaving through one IP would spend
// two session identities, not one.
func (p *ProxyPool) ExitKey(nodeID string) string { return nodeID }

// Report books one (exit, model) failure. The growth rules are copied from
// internal/subs (see ledger.go there for why these values) so the two egress
// implementations park the same failure class for the same time.
func (p *ProxyPool) Report(nodeID, model, class string) {
	if nodeID == "" || model == "" {
		return
	}
	k := poolBanKey(nodeID, model)
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if last, ok := p.failAt[k]; ok && now.Sub(last) > poolFailMemory {
		p.fails[k] = 0
	}
	p.fails[k]++
	p.failAt[k] = now
	n := p.fails[k]

	var d time.Duration
	switch class {
	case "region":
		d = poolBanRegion
	case "limited":
		d = poolBanBaseLimited << min(n-1, 5)
		if d > poolBanCeilLimited {
			d = poolBanCeilLimited
		}
	default:
		d = poolBanBaseOther << min(n-1, 5)
		if d > poolBanCeilOther {
			d = poolBanCeilOther
		}
	}
	p.banUntil[k] = now.Add(d)
}

// ReportDead books one tunnel-level failure. Two strikes inside the memory
// window park the endpoint for a minute, doubling on every further strike up
// to fifteen. A static proxy cannot be probed back to life, so time is the
// only revival path; the cap keeps a permanently dead entry from being
// retried in a tight loop.
func (p *ProxyPool) ReportDead(nodeID string) {
	if nodeID == "" {
		return
	}
	if _, ok := p.byID[nodeID]; !ok {
		return
	}
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if t := p.deadUntil[nodeID]; !t.IsZero() && now.Before(t) {
		return // already parked; another strike must not extend the wait
	}
	if last, ok := p.deadFailAt[nodeID]; ok && now.Sub(last) > poolFailMemory {
		p.deadFails[nodeID] = 0
	}
	p.deadFails[nodeID]++
	p.deadFailAt[nodeID] = now
	if p.deadFails[nodeID] < poolDeadStrikes {
		return
	}
	p.deadFails[nodeID] = 0
	p.deadParks[nodeID]++
	d := poolDeadBase << min(p.deadParks[nodeID]-1, 4)
	if d > poolDeadCeil {
		d = poolDeadCeil
	}
	p.deadUntil[nodeID] = now.Add(d)
}

// Revive clears one (exit, model) pair after real traffic succeeded through
// it. Node-level death is not cleared here: it was parked because the tunnel
// failed twice, and one success on another connection does not prove the
// endpoint stable.
func (p *ProxyPool) Revive(nodeID, model string) {
	if nodeID == "" || model == "" {
		return
	}
	k := poolBanKey(nodeID, model)
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.banUntil, k)
	delete(p.fails, k)
	delete(p.failAt, k)
}

// dialNode opens a connection through one entry.
func (p *ProxyPool) dialNode(ctx context.Context, n poolNode, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("pool chỉ hỗ trợ TCP, nhận %q", network)
	}
	switch n.scheme {
	case "direct":
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if n.user != nil {
			pass, _ := n.user.Password()
			auth = &proxy.Auth{User: n.user.Username(), Password: pass}
		}
		d, err := proxy.SOCKS5("tcp", n.addr, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, errors.New("SOCKS5 dialer không hỗ trợ context")
		}
		return cd.DialContext(ctx, network, addr)
	case "http", "https":
		return dialHTTPProxy(ctx, n, addr)
	}
	return nil, errors.New("scheme không hỗ trợ: " + n.scheme)
}

// dialHTTPProxy opens a CONNECT tunnel through an HTTP(S) proxy. net/http's
// Transport can do this for a single configured proxy, but the pool dials per
// node, so the handshake is explicit here.
func dialHTTPProxy(ctx context.Context, n poolNode, addr string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", n.addr)
	if err != nil {
		return nil, err
	}
	closeOnErr := func(err error) (net.Conn, error) {
		_ = conn.Close()
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	}
	if n.scheme == "https" {
		host, _, _ := net.SplitHostPort(n.addr)
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return closeOnErr(fmt.Errorf("TLS tới proxy: %w", err))
		}
		conn = tlsConn
	}

	var req strings.Builder
	req.WriteString("CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n")
	if n.user != nil {
		pass, _ := n.user.Password()
		token := base64.StdEncoding.EncodeToString([]byte(n.user.Username() + ":" + pass))
		req.WriteString("Proxy-Authorization: Basic " + token + "\r\n")
	}
	req.WriteString("\r\n")
	if _, err := conn.Write([]byte(req.String())); err != nil {
		return closeOnErr(fmt.Errorf("ghi CONNECT: %w", err))
	}

	br := bufio.NewReader(conn)
	tp := textproto.NewReader(br)
	line, err := tp.ReadLine()
	if err != nil {
		return closeOnErr(fmt.Errorf("đọc phản hồi proxy: %w", err))
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		return closeOnErr(fmt.Errorf("phản hồi proxy không hợp lệ: %q", line))
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil {
		return closeOnErr(fmt.Errorf("mã trạng thái proxy không hợp lệ: %q", fields[1]))
	}
	if _, err := tp.ReadMIMEHeader(); err != nil {
		return closeOnErr(fmt.Errorf("đọc header proxy: %w", err))
	}
	if code < 200 || code > 299 {
		return closeOnErr(fmt.Errorf("proxy từ chối CONNECT: HTTP %d", code))
	}
	_ = conn.SetDeadline(time.Time{})
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

// bufferedConn returns bytes the CONNECT response reader already pulled off
// the socket before the tunnel was handed back as a plain net.Conn.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.r.Read(b) }
