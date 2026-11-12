// Command openkistats is the server-side companion of OpenKi: it receives the
// anonymous heartbeats the desktop gateway sends ({installId, version,
// country}) and serves a dashboard showing how many people are using it.
//
// Single binary, zero dependencies, one JSON file for storage:
//
//	openkistats.exe              # serves :8321 (dashboard + API)
//	OPENKISTATS_ADDR=:8321       # override listen address
//	OPENKISTATS_TOKEN=s3cret     # require ?token=s3cret on the dashboard (optional)
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed logo.png
var logoPNG []byte

// Install is one deployed OpenKi instance.
type Install struct {
	InstallID string `json:"installId"`
	Version   string `json:"version"`
	Country   string `json:"country,omitempty"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	Pings     int64  `json:"pings"`
}

type store struct {
	mu       sync.Mutex
	installs map[string]*Install
	dirty    bool
}

var db = &store{installs: map[string]*Install{}}

var dataFile string

func main() {
	addr := os.Getenv("OPENKISTATS_ADDR")
	if addr == "" {
		addr = ":8321"
	}
	exe, _ := os.Executable()
	dataFile = filepath.Join(filepath.Dir(exe), "stats-server.json")
	load()

	mux := http.NewServeMux()
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "image/png")
		w.Header().Set("cache-control", "public, max-age=86400")
		_, _ = w.Write(logoPNG)
	})
	mux.HandleFunc("/", handleDashboard)
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/stats", handleStats)
	mux.HandleFunc("/api/installs", handleInstalls)

	fmt.Printf("openkistats serving on %s (data: %s)\n", addr, dataFile)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Println("listen error:", err)
		os.Exit(1)
	}
}

func load() {
	b, err := os.ReadFile(dataFile)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &db.installs)
}

func saveLocked() {
	snapshot := make(map[string]*Install, len(db.installs))
	for k, v := range db.installs {
		cp := *v
		snapshot[k] = &cp
	}
	b, _ := json.MarshalIndent(snapshot, "", " ")
	tmp := dataFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, dataFile)
	}
	db.dirty = false
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in Install
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	in.InstallID = strings.TrimSpace(in.InstallID)
	if in.InstallID == "" || len(in.InstallID) > 64 {
		http.Error(w, "missing installId", http.StatusBadRequest)
		return
	}
	now := time.Now().UnixMilli()
	in.Version = strings.TrimSpace(in.Version)
	in.Country = strings.ToUpper(strings.TrimSpace(in.Country))
	in.LastSeen = now

	db.mu.Lock()
	defer db.mu.Unlock()
	cur, ok := db.installs[in.InstallID]
	if !ok {
		in.FirstSeen = now
		in.Pings = 1
		db.installs[in.InstallID] = &in
		db.dirty = true
	} else {
		cur.Version, cur.Country, cur.LastSeen = in.Version, in.Country, in.LastSeen
		cur.Pings++
		db.dirty = true
	}
	if db.dirty {
		saveLocked()
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.dirty {
		saveLocked()
	}
	now := time.Now().UnixMilli()
	active7, active30 := int64(0), int64(0)
	versions := map[string]int64{}
	countries := map[string]int64{}
	for _, in := range db.installs {
		if now-in.LastSeen <= 7*24*3600*1000 {
			active7++
		}
		if now-in.LastSeen <= 30*24*3600*1000 {
			active30++
		}
		if in.Version != "" {
			versions[in.Version]++
		}
		if in.Country != "" {
			countries[in.Country]++
		}
	}
	writeJSON(w, map[string]any{
		"totalInstalls": len(db.installs),
		"active7d":      active7,
		"active30d":     active30,
		"versions":      versions,
		"countries":     countries,
		"generatedAt":   now,
	})
}

// handleInstalls lists every install (the dashboard table + /api consumers).
func handleInstalls(w http.ResponseWriter, r *http.Request) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.dirty {
		saveLocked()
	}
	out := make([]Install, 0, len(db.installs))
	for _, in := range db.installs {
		out = append(out, *in)
	}
	writeJSON(w, out)
}

// --- dashboard ----------------------------------------------------------------

var dashboardToken = os.Getenv("OPENKISTATS_TOKEN")

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if dashboardToken != "" && r.URL.Query().Get("token") != dashboardToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML()))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func dashboardHTML() string { return dashHTML }

const dashHTML = `<!doctype html>
<html lang="vi"><head><meta charset="utf-8">
<title>OpenKi · Tổng quan người dùng</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="description" content="OpenKi – cổng mô hình miễn phí cục bộ · thống kê nhịp tim ẩn danh của các bản triển khai">
<link rel="icon" type="image/png" href="/logo.png">
<style>
:root { --bg:#141414; --panel:#1c1c1c; --panel-2:#262626; --ink:#ececec; --ink-dim:#a8a8a8; --ink-mute:#787878;
  --hairline:rgba(255,255,255,.08); --accent:#38bdf8; --ok:#34d399; --warn:#fbbf24; }
* { box-sizing:border-box; }
body { margin:0; background:radial-gradient(1200px 500px at 50% -200px, #1a2332 0%, var(--bg) 60%); color:var(--ink);
  font:14px/1.65 "Segoe UI",system-ui,-apple-system,sans-serif; min-height:100vh; }
.wrap { max-width:1100px; margin:0 auto; padding:26px 22px 70px; }
header { display:flex; align-items:center; gap:14px; margin-bottom:6px; }
header img { width:44px; height:44px; border-radius:10px; }
.tt h1 { font-size:19px; letter-spacing:.16em; margin:0; font-weight:600; }
.tt h1 b { color:var(--accent); font-weight:600; }
.tt .tag { font-size:12px; color:var(--ink-mute); }
.live { margin-left:auto; display:flex; align-items:center; gap:7px; font-size:12px; color:var(--ink-dim);
  background:var(--panel); border:1px solid var(--hairline); border-radius:20px; padding:5px 14px; }
.live .dot { width:8px; height:8px; border-radius:50%; background:var(--ok); animation:pulse 2s infinite; }
@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:.35} }
.sub { color:var(--ink-mute); font-size:12px; margin:0 0 26px; padding-left:58px; }
.cards { display:grid; grid-template-columns:repeat(auto-fit,minmax(200px,1fr)); gap:12px; margin-bottom:18px; }
.card { background:var(--panel); border:1px solid var(--hairline); border-radius:12px; padding:16px 18px;
  display:flex; gap:14px; align-items:center; }
.card .ic { width:38px; height:38px; border-radius:9px; background:rgba(56,189,248,.12); display:flex;
  align-items:center; justify-content:center; flex:none; }
.card .ic svg { width:19px; height:19px; stroke:var(--accent); fill:none; stroke-width:2; stroke-linecap:round; stroke-linejoin:round; }
.card .v { font-size:26px; font-weight:700; line-height:1.1; font-variant-numeric:tabular-nums; }
.card .l { font-size:12px; color:var(--ink-mute); }
.panel { background:var(--panel); border:1px solid var(--hairline); border-radius:12px; padding:18px 20px; margin-bottom:14px; }
.panel h3 { margin:0 0 14px; font-size:12px; color:var(--ink-mute); letter-spacing:.1em; font-weight:600; }
.bar-row { display:flex; align-items:center; gap:12px; margin:8px 0; }
.bar-row .nm { width:130px; font-size:12.5px; color:var(--ink-dim); text-align:right; flex:none; }
.bar-row .tr { flex:1; height:16px; background:var(--panel-2); border-radius:5px; overflow:hidden; }
.bar-row .tr i { display:block; height:100%; background:linear-gradient(90deg,#0ea5e9,var(--accent)); border-radius:5px; transition:width .6s ease; }
.bar-row .ct { width:60px; font-size:12px; color:var(--ink-mute); font-variant-numeric:tabular-nums; }
table { width:100%; border-collapse:collapse; font-size:12.5px; }
th { text-align:left; color:var(--ink-mute); font-weight:500; padding:6px 10px; border-bottom:1px solid var(--hairline); font-size:11.5px; letter-spacing:.05em; }
td { padding:7px 10px; border-bottom:1px solid rgba(255,255,255,.045); color:var(--ink-dim); font-variant-numeric:tabular-nums; }
tr:hover td { background:rgba(255,255,255,.02); }
.dot { width:7px; height:7px; border-radius:50%; background:var(--ok); display:inline-block; margin-right:8px; box-shadow:0 0 6px rgba(52,211,153,.6); }
.muted { color:var(--ink-mute); font-size:11.5px; }
.empty { color:var(--ink-mute); font-size:12.5px; padding:8px 0; }
footer { margin-top:34px; display:flex; justify-content:space-between; color:var(--ink-mute); font-size:11.5px; }
</style></head><body><div class="wrap">
<header>
  <img src="/logo.png" alt="OpenKi">
  <div class="tt"><h1>OPENKI <b>Tổng quan người dùng</b></h1><div class="tag">Cổng mô hình miễn phí cục bộ · Thống kê bản triển khai</div></div>
  <div class="live"><span class="dot"></span><span id="live">Trực tiếp</span></div>
</header>
<div class="sub">Dữ liệu từ nhịp tim ẩn danh của các bản triển khai (chỉ installId / phiên bản / quốc gia), báo cáo mỗi 6 giờ, chỉ lưu trên máy chủ này.</div>
<div class="cards">
  <div class="card"><div class="ic"><svg viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></div><div><div class="v" id="c-total">—</div><div class="l">Tổng lượt cài đặt (bản triển khai đã khử trùng lặp)</div></div></div>
  <div class="card"><div class="ic"><svg viewBox="0 0 24 24"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg></div><div><div class="v" id="c-7d">—</div><div class="l">Bản triển khai hoạt động trong 7 ngày</div></div></div>
  <div class="card"><div class="ic"><svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg></div><div><div class="v" id="c-30d">—</div><div class="l">Bản triển khai hoạt động trong 30 ngày</div></div></div>
  <div class="card"><div class="ic"><svg viewBox="0 0 24 24"><path d="M20.59 13.41 12 22l-9-9V3h10l7.59 7.59a2 2 0 0 1 0 2.82z"/><circle cx="7.5" cy="7.5" r="1.5"/></svg></div><div><div class="v" id="c-ver">—</div><div class="l">Số phiên bản đang dùng</div></div></div>
</div>
<div class="panel"><h3>Phân bố phiên bản</h3><div id="p-versions"><div class="empty">Đang tải…</div></div></div>
<div class="panel"><h3>Phân bố quốc gia / khu vực</h3><div id="p-countries"><div class="empty">Đang tải…</div></div></div>
<div class="panel"><h3>Nhịp tim gần đây</h3><table><thead><tr><th>Bản triển khai</th><th>Phiên bản</th><th>Quốc gia / Khu vực</th><th>Lần đầu trực tuyến</th><th>Nhịp tim gần nhất</th><th>Số lần báo cáo</th></tr></thead><tbody id="tb"><tr><td colspan="6" class="empty">Đang tải…</td></tr></tbody></table></div>
<footer><span>OpenKi · Nhịp tim mỗi 6 giờ · Dữ liệu chỉ lưu trên máy chủ này</span><span>Tự làm mới mỗi 30 giây · Cập nhật lúc <span id="gen">—</span></span></footer>
</div>
<script>
const f = n => new Date(n).toLocaleString("vi-VN", {hour12:false});
const ago = n => { const s = Math.max(0,(Date.now()-n)/1000);
  if (s < 90) return "Vừa xong"; if (s < 3600) return Math.round(s/60)+" phút trước";
  if (s < 86400) return Math.round(s/3600)+" giờ trước"; return Math.round(s/86400)+" ngày trước"; };
const flag = cc => { cc = (cc||"").toUpperCase(); if (!/^[A-Z]{2}$/.test(cc)) return "";
  return String.fromCodePoint(...[...cc].map(c=>127397+c.charCodeAt(0))); };
const shortId = s => (s||"").slice(0,8);
const bars = (obj, el) => {
  const items = Object.entries(obj||{}).sort((a,b)=>b[1]-a[1]);
  const max = Math.max(1, ...items.map(x=>x[1]));
  document.getElementById(el).innerHTML = items.length
    ? items.map(([k,v])=>'<div class="bar-row"><span class="nm">'+k+'</span><span class="tr"><i style="width:'+Math.round(v/max*100)+'%"></i></span><span class="ct">'+v+'</span></div>').join("")
    : '<div class="empty">Chưa có dữ liệu</div>';
};
const refresh = () => {
  fetch("/api/stats").then(r=>r.json()).then(d=>{
    document.getElementById("c-total").textContent = d.totalInstalls;
    document.getElementById("c-7d").textContent = d.active7d;
    document.getElementById("c-30d").textContent = d.active30d;
    document.getElementById("c-ver").textContent = Object.keys(d.versions||{}).length;
    bars(d.versions, "p-versions");
    bars(d.countries, "p-countries");
    document.getElementById("gen").textContent = f(d.generatedAt);
  });
  fetch("/api/installs").then(r=>r.json()).then(list=>{
    const rows = [...list].sort((a,b)=>b.lastSeen-a.lastSeen).slice(0,50);
    document.getElementById("tb").innerHTML = rows.length ? rows.map(x=>
      '<tr><td><span class="dot"></span>'+shortId(x.installId)+"…"+'</td><td>'+ (x.version||"—") +'</td><td>'
      + flag(x.country) +' '+ (x.country||"—") +'</td><td>'+f(x.firstSeen)+'</td><td>'+f(x.lastSeen)+'（'+ago(x.lastSeen)+'）</td><td>'+x.pings+'</td></tr>').join("")
      : '<tr><td colspan="6" class="empty">Chưa có bản triển khai nào trực tuyến</td></tr>';
  });
};
refresh();
setInterval(refresh, 30000);
</script></body></html>
`
