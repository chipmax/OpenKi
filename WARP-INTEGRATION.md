# Tích hợp WARP pool (opencode2api) vào fork OpenKi

## Thiết kế

- **Package `internal/warp/`**: sao chép gần như nguyên trạng từ opencode2api
  (`internal/warp`, ~1.965 dòng: pool/exit/registrar/bind) — userspace
  WireGuard + gVisor netstack, không cần TUN, root hay `NET_ADMIN`. Nguồn gốc
  và license ghi ở `internal/warp/NOTICE` (bản thân code này được adapted từ
  ai-route, MIT). Phiên bản wireguard được ghim đúng bản opencode2api dùng
  (`v0.0.0-20261006164505-2631ce99a06f`) vì bản mới hơn gây lỗi
  "ambiguous import" cho `tun/netstack` khi `go mod tidy`.
- **Lớp nối `internal/lane/warp.go`**: một `warpRoundTripper` bọc transport
  gốc mà `SetProxy` dựng trong `internal/lane/http.go`. Luồng mỗi request:
  1. `Acquire` một lease (exit) từ pool dùng chung — pool khởi tạo *lazy* ở
     request đầu tiên cần nó (kích thước = `warpPoolSize`, mặc định 2, stack
     `ipv6` như config mẫu của opencode2api).
  2. Clone transport gốc, thay `DialContext` bằng `lease.DialContext`
     (kèm `lease.DialNetwork()`), tắt proxy (tunnel chính là egress), tắt
     keep-alive và HTTP/2 (kết nối chết theo lease, giống opencode2api).
  3. Lease chỉ được `Release` khi **response Body đóng** (bọc bằng
     `leaseBody`, release đúng 1 lần nhờ `sync.Once`) — nhờ vậy exit không
     bao giờ xoay giữa chừng khi đang stream SSE.
  4. Nếu upstream trả **429/403**: đọc tối đa 32 KB body, khôi phục body cho
     caller (phần đã đọc + phần còn lại), rồi gọi
     `Pool.ReportWithRetryAfter(lease, status, body, retryAfter)` để exit bị
     phạt/cooldown theo `Retry-After`. Status khác không report.
  5. Đích loopback (ví dụ provider opencode2api chạy cùng máy) **bỏ qua
     WARP hoàn toàn** — WARP netstack không dial được 127.0.0.1.
- **Chọn mode là hàm thuần** `useWarp(warpEnabled, proxyMode)` trong
  `internal/lane/warp.go`, có test riêng (`internal/lane/warp_mode_test.go`).

## Cách bật

Trong `config.json` của openki (thư mục `OPENKI_HOME`), một trong hai:

```json
{ "warpEnabled": true, "warpPoolSize": 2 }
```

hoặc đặt `"proxyMode": "warp"`. `warpEnabled=true` được ưu tiên cao nhất —
bật là đi WARP bất kể `proxyMode` là gì. Cũng bật/tắt nóng được qua API
settings của dashboard (`warpEnabled` / `warpPoolSize` trong
`POST /admin/api/settings`; trạng thái hiện tại — gồm số lần fallback — xem ở
`/admin/api/state` qua các trường `warpEnabled`, `warpPoolSize`,
`warpFallbacks`). Hai trường config đều `omitempty` nên config cũ load y
nguyên, không bật thì hành vi như bản gốc.

## Hành vi fallback (quan trọng)

WARP cần UDP ra trực tiếp tới endpoint Cloudflare (162.159.192.1:2408) và
HTTPS đăng ký thiết bị. Mạng chặn một trong hai thứ đó thì pool không dựng
được — khi đó round-tripper **tự rơi về transport gốc** (direct/proxy theo
`ProxyMode`) thay vì làm chết request:

- Lỗi dựng pool được cache 60 giây (không thử đăng ký lại theo từng request),
  hết 60 giây thử lại — mạng mở ra là WARP tự sống lại.
- `Acquire` lỗi cũng fallback tương tự. Mọi lần fallback đều tăng bộ đếm
  `lane.WarpFallbacks()` (expose qua `/admin/api/state`).
- Lỗi transport *qua tunnel* (đã có lease) thì khác: exit bị `Pool.Fail` +
  release và lỗi trả về caller như bình thường — đó là bằng chứng exit hỏng,
  không phải bằng chứng WARP không khả dụng.

## Kết quả chạy thật trên VM của Muse (2026-11-11)

Build headless với `warpEnabled=true, warpPoolSize=2`, cổng 8797:

- Gateway lên bình thường: `/healthz` OK, `/v1/models` OK.
- **Đăng ký WARP thất bại — đúng như dự đoán cho VM này**: VM chỉ ra internet
  qua HTTP proxy, còn registrar của WARP đi trực tiếp, nên bước đăng ký chết ở
  `Post "https://api.cloudflareclient.com/v0i1909051800/reg": tls: first
  record does not look like a TLS handshake`. Kể cả qua được bước này,
  handshake WireGuard bằng UDP trực tiếp cũng không có đường ra trên VM.
- **Fallback hoạt động đúng thiết kế**: `/admin/api/state` ghi
  `warpFallbacks: 41` (probe + refresh catalog + chat), toàn bộ traffic vẫn
  được phục vụ qua transport gốc, request chat trả lời đúng nội dung trong
  ~1,1 giây. Nói cách khác: trên máy/mạng không có UDP, bật WARP = tốn một
  lần thử đăng ký, gateway không hề gián đoạn.
- Chưa kiểm chứng được đường WARP thành công (exit đăng ký được + request đi
  qua tunnel) — cần chạy trên máy thật có UDP ra trực tiếp (máy của Mia hoặc
  VPS thường là được).

## Build & test

- `CGO_ENABLED=0 go build -tags headless ./...` — pass.
- `go test ./internal/warp/ ./internal/lane/ ./internal/store/` — pass
  (warp không có test riêng; lane có thêm `warp_mode_test.go`: test
  `useWarp`, test `leaseBody` release đúng 1 lần, test fallback khi pool
  lỗi + đếm fallback, test bypass loopback, test parse `Retry-After`).

## Lưu ý license

openki (bản gốc) theo GPL-3.0; opencode2api là **GPL-3.0**; package warp
nguyên thủy trong ai-route là MIT. Toàn bộ fork openki phát hành theo
**GPL-3.0-only** — xem `LICENSE`. Không phát hành bản fork dưới
danh nghĩa MIT. Xem `internal/warp/NOTICE` và `THIRD_PARTY_NOTICES.md`.
