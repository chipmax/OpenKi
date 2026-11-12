# Triển khai loimo

Tài liệu này mô tả ma trận triển khai chuẩn cho ba môi trường: **Windows (GUI +
tray)**, **WSL (headless + systemd)** và **VPS (headless + Caddy)**. Mọi ví dụ
dùng tên khóa cấu hình và cờ CLI đúng như chương trình đọc.

## Nguyên tắc bất biến

1. **Trang quản trị chỉ nghe loopback.** Mặc định gateway bind `127.0.0.1`.
   Không bật `allowLan` trên VPS. Ra Internet chỉ có `/v1/*` và `/healthz` qua
   Caddy TLS.
2. **Không bao giờ proxy `/admin*` qua Internet.** Caddy phải chặn trước khi
   reverse proxy; quản trị VPS đi bằng SSH tunnel.
3. **Mỗi máy một `mainKey`.** Không sao chép nguyên `config.json` giữa các máy.
4. **Secret nhập tay.** Token relay, API key nhà cung cấp, `agentKeys` không nằm
   trong file mẫu dùng chung.
5. **Một owner cho mỗi phạm vi tiêm cấu hình Agent.** Không để hai gateway cùng
   sửa một file cấu hình Agent.

## Kiến trúc

```text
Windows 10/11 (GUI + tray)                 WSL2 (headless, systemd)
  Agent: OpenCode/Codex/...                  Agent: OpenCode/Codex/...
        │                                          │
        ▼                                          ▼
  127.0.0.1:8787                            127.0.0.1:8788
  loimo GUI/tray                         loimo (LOIMO_HEADLESS=1)
        │                                          │
        └──────────────┬───────────────────────────┘
                       │  egressModes (đồng cấp)
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
     relay          singbox          warp
  Vercel (chính)   sidecar         userspace
  VPS (dự phòng)   tự cài          WireGuard

VPS (headless, systemd)
  Internet ── 443/TLS ── Caddy ── 127.0.0.1:8787 loimo
                          │
                          ├── /v1/*      → proxy (stream flush)
                          ├── /healthz   → proxy
                          └── /admin*    → chặn 404
  Quản trị: ssh -L 18787:127.0.0.1:8787 admin@vps
```

## Ma trận triển khai

| | Windows | WSL2 | VPS |
|---|---|---|---|
| Vai trò | Máy chính của người dùng | Môi trường Linux trên cùng máy | Gateway công khai cho Agent từ xa |
| Chế độ | GUI + tray | Headless | Headless |
| Cổng | `8787` | `8788` | `127.0.0.1:8787` (Caddy `443`) |
| Khởi động | Tray/autostart hoặc Task Scheduler lúc đăng nhập | `systemd` | `systemd` |
| Chạy dưới | Phiên đăng nhập của user | User `admin` (hoặc user WSL) | User `loimo` |
| `LOIMO_HEADLESS` | Không đặt (GUI) | `1` | `1` |
| `LOIMO_HOME` | `%APPDATA%\loimo` (mặc định) | `/home/admin/.config/loimo` | `/var/lib/loimo` |
| Tiêm cấu hình Agent | `agentScope: windows` (hoặc `host`) | `agentScope: wsl` | `--no-inject` |
| Lộ ra Internet | Không | Không | Chỉ `/v1/*`, `/healthz` qua Caddy |
| Quản trị | `http://127.0.0.1:8787` | `http://127.0.0.1:8788` | SSH tunnel + loopback |

WSL dùng `8788` **chỉ khi** chạy song song gateway Windows `8787`; nếu chỉ có
một instance trong WSL thì dùng `8787`. WSL2 chuyển tiếp localhost nên trình
duyệt trên Windows vẫn mở được cùng cổng. Đặt `port` trong `config.json` của
WSL hoặc `--port` trong unit, chỉ dùng một nguồn để tránh lệch cấu hình.

## Phạm vi sở hữu và tiêm cấu hình Agent

`agentScope` là khóa trong `config.json` quyết định gateway được phép tiêm cấu
hình vào Agent ở phía nào:

| Giá trị | Ý nghĩa |
|---|---|
| `host` | Chỉ môi trường đang chạy tiến trình |
| `all` | Cả hai phía (mặc định khi khóa vắng mặt) |
| `windows` | Chỉ Agent phía Windows |
| `wsl` | Chỉ Agent phía WSL |
| `all` | Cả hai phía — chỉ dùng khi **một** gateway duy nhất làm owner |
| `none` | Không tiêm |

Bỏ trống hoặc chuỗi rỗng = `all` (migration từ config cũ cũng đặt `all` để
không mất tính năng đang dùng). Khuyến nghị:

- Windows GUI: `agentScope: "windows"` — owner phía Windows.
- WSL systemd: `agentScope: "wsl"` — owner phía WSL.
- VPS: `--no-inject` — không có Agent cục bộ, không ghi vào `$HOME`.

Cờ CLI hỗ trợ kiểm soát theo từng lần chạy:

- `--no-inject`: tắt hoàn toàn việc tiêm/khôi phục cấu hình Agent trong tiến
  trình này. Dùng cho VPS, CI, kiểm thử, hoặc khi chạy instance phụ.
- `--inject-scope=<value>`: giới hạn phạm vi tiêm cho lần chạy, nhận cùng tập
  giá trị `host|windows|wsl|all|none`. Cờ này thắng `agentScope` trong file;
  `--no-inject` thắng tất cả.

Ví dụ WSL chỉ owner phía WSL:

```bash
LOIMO_HEADLESS=1 LOIMO_HOME=/home/admin/.config/loimo \
  loimo --inject-scope=wsl
```

Ví dụ VPS không tiêm:

```bash
LOIMO_HEADLESS=1 LOIMO_HOME=/var/lib/loimo \
  loimo --no-inject
```

## Egress: `egressModes`

`egressModes` là mảng trong `config.json`, chứa một hoặc nhiều giá trị:

```json
{ "egressModes": ["relay", "singbox", "warp"] }
```

- Giá trị hợp lệ: `relay`, `singbox`, `warp`.
- **Ba chế độ là đồng cấp**, không có thứ hạng ưu tiên cố định: không peer nào
  mặc định đứng trước hay là "dự phòng" của peer khác. Có thể chọn một mình
  hoặc kết hợp.
- Khi kết hợp, request luân phiên bắt đầu ở từng peer; lỗi transport hoặc
  `429`/`403` chuyển sang peer kế tiếp. Đó là failover theo lỗi tạm thời, không
  phải chuỗi ưu tiên; thứ tự khai báo không tạo ra peer "chính".
- Mảng rỗng hoặc không có khóa = giữ hành vi cũ theo `proxyMode`.
- `relay` dùng các mục trong `relayPool` (xem dưới).
- `singbox` dùng sidecar sing-box do người dùng tự cài (`singBoxPath`).
- `warp` dùng pool WARP userspace tích hợp trong gateway.

### `relayPool`

```json
{
  "relayPool": [
    { "id": "vercel-sin", "url": "https://relay-sin.example.com", "token": "<RELAY_TOKEN>", "enabled": true },
    { "id": "vps-fra",    "url": "https://relay-fra.example.com", "token": "<RELAY_TOKEN>", "enabled": true }
  ]
}
```

Mỗi mục gồm `id`, `url`, `token`, `enabled`. `url` phải là HTTPS. `token`
truyền qua header `X-LoiMo-Relay-Token`; đích upstream khai báo bằng
`X-LoiMo-Relay-Upstream` và bị giới hạn bởi allowlist phía relay. Không đưa token
vào file mẫu, log hay ảnh chụp màn hình.

### Vị trí relay

| Vị trí | Vai trò | Ghi chú |
|---|---|---|
| Vercel (Bun), **hai region** | Chính | Đặt cạnh upstream, độ trễ thấp; hai region để chịu lỗi vùng |
| VPS (Bun) | Phụ / dự phòng | Cùng hạ tầng với gateway hoặc độc lập; bật khi Vercel suy giảm |
| Cloudflare Worker | **Không dùng** | Không kiểm soát được TLS fingerprint theo cách luồng miễn phí OpenCode yêu cầu; dễ bị phân loại sai |

Không đặt Cloudflare Worker làm relay cho làn miễn phí OpenCode. Chi tiết giao
thức và các bước triển khai: [RELAY.md](RELAY.md).

## Triển khai Windows

1. Tải/cài bản GUI, chạy một lần để tạo `%APPDATA%\loimo\config.json`.
2. Mở `http://127.0.0.1:8787`, đặt `mainKey`, chọn Agent cần tiêm.
3. Giữ `agentScope: "windows"`; không bật `allowLan`.
4. Khởi động cùng phiên đăng nhập bằng tray/autostart có sẵn, hoặc Task
   Scheduler trigger **At logon**. **Không** cài làm Windows service — xem
   [deploy/windows/README.md](../deploy/windows/README.md).

Kiểm tra nhanh (PowerShell):

```powershell
Invoke-RestMethod http://127.0.0.1:8787/healthz
Get-NetTCPConnection -LocalPort 8787 -State Listen | Select-Object LocalAddress,LocalPort
```

`LocalAddress` phải là `127.0.0.1` (hoặc `::1`), không phải `0.0.0.0`.

## Triển khai WSL

Yêu cầu: WSL2 có systemd. Bật trong `/etc/wsl.conf` rồi `wsl --shutdown`:

```ini
[boot]
systemd=true
```

Cài binary headless và unit mẫu:

```bash
CGO_ENABLED=0 go build -tags headless -trimpath -o loimo ./cmd/loimo
sudo install -o root -g root -m 0755 loimo /usr/local/bin/loimo
sudo install -m 0644 deploy/wsl/loimo.service /etc/systemd/system/loimo.service
sudo systemctl daemon-reload
sudo systemctl enable --now loimo
systemctl status loimo --no-pager
journalctl -u loimo -f
```

Unit mẫu dùng `User=admin`, `LOIMO_HEADLESS=1`,
`LOIMO_HOME=/home/admin/.config/loimo`, `--inject-scope=wsl`,
`Restart=always`. Nếu user WSL khác `admin`, sửa `User=`, `Group=`, `HOME`,
`LOIMO_HOME`, `ReadWritePaths` cho khớp. Đặt `port: 8788` trong
`config.json` của WSL.

Muốn mở dashboard WSL từ Windows mà không thêm relay, đặt trong
`%UserProfile%\.wslconfig`:

```ini
[wsl2]
networkingMode=mirrored
```

rồi `wsl --shutdown`. WSL có thể bỏ listener localhost khi không còn phiên
nào, nên giữ một phiên ẩn lúc đăng nhập (ví dụ shortcut
`wsl.exe -d Debian -e sleep infinity`) nếu cần dashboard thường trực.

## Triển khai VPS

Giả định: Debian/Ubuntu, domain `gw.example.com` trỏ A/AAAA về VPS, mở `22`,
`80`, `443`; **không** mở `8787`.

```bash
# 1. User riêng, không đăng nhập
sudo useradd --system --home /var/lib/loimo --shell /usr/sbin/nologin loimo
sudo install -d -o loimo -g loimo -m 0750 /var/lib/loimo
sudo install -d -o root -g root -m 0755 /opt/loimo

# 2. Binary headless
CGO_ENABLED=0 go build -tags headless -trimpath -o loimo ./cmd/loimo
sudo install -o root -g root -m 0755 loimo /opt/loimo/loimo

# 3. Cấu hình (tạo từ deploy/config.example.json, sửa quyền sở hữu)
sudo install -o loimo -g loimo -m 0600 deploy/config.example.json /var/lib/loimo/config.json
sudo -u loimo nano /var/lib/loimo/config.json

# 4. systemd
sudo install -m 0644 deploy/vps/loimo.service /etc/systemd/system/loimo.service
sudo systemctl daemon-reload
sudo systemctl enable --now loimo

# 5. Caddy
sudo install -m 0644 deploy/vps/Caddyfile /etc/caddy/Caddyfile
sudo nano /etc/caddy/Caddyfile        # thay gw.example.com bằng domain thật
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Unit VPS chạy `User=loimo`, `LOIMO_HEADLESS=1`,
`LOIMO_HOME=/var/lib/loimo`, `ExecStart=/opt/loimo/loimo --no-inject`,
`Restart=always`, có hardening cơ bản. Caddyfile chỉ reverse proxy `/v1/*` và
`/healthz` tới `127.0.0.1:8787`, chặn `/admin*`, và tắt buffer để stream chảy
ngay (`flush_interval -1`).

Quản trị VPS (không mở `/admin` ra Internet):

```bash
ssh -N -L 18787:127.0.0.1:8787 admin@vps.example.com
# sau đó mở http://127.0.0.1:18787 trên máy local
```

Dùng cổng local `18787` để tránh đụng gateway Windows `8787`.

## Đồng bộ cấu hình giữa các máy

| Khóa | Chia sẻ? | Quy tắc |
|---|---|---|
| `mainKey` | **Không** | Sinh riêng cho từng máy; không dùng lại key của máy khác |
| `agentKeys` | **Không** | Sinh theo từng Agent/máy; thu hồi bằng cách xóa key |
| `agentScope` | Có (theo vai trò) | Windows `windows`, WSL `wsl`, VPS `none` (qua `--no-inject`) |
| `egressModes` | Có | Cùng chiến lược egress nếu cùng mạng |
| `relayPool[].id/url/enabled` | Có | URL và bật/tắt là cấu hình vận hành |
| `relayPool[].token` | **Không** | Secret; nhập tay trên từng máy |
| API key nhà cung cấp | **Không** | Secret; nhập tay |
| `port` | Không | Windows `8787`, WSL `8788`, VPS `8787` (loopback) |
| `allowLan` | Không | Luôn `false` trên VPS và WSL; Windows chỉ bật khi thật cần |

Quy trình:

1. Dùng `deploy/config.example.json` làm **mẫu không chứa secret**.
2. Chỉ chép các khóa phi bí mật đã thống nhất (ví dụ `egressModes`,
   `relayPool` URL/id/enabled, `agentScope`).
3. Nhập tay `mainKey`, token relay, API key trên từng máy.
4. Kiểm tra lại `mainKey` không trùng giữa các máy trước khi đưa vào sử dụng.
5. Sao lưu `config.json` trước khi nâng cấp; không commit file thật vào Git.

## Kiểm tra chấp nhận (smoke test)

Chạy trên từng môi trường sau khi cài. Thay `$MAIN_KEY`, `<model-id>`,
`gw.example.com` bằng giá trị thật.

```bash
# 1. Liveness — không cần key
curl -fsS http://127.0.0.1:8787/healthz        # Windows/VPS loopback
curl -fsS http://127.0.0.1:8788/healthz        # WSL

# 2. Danh sách mô hình — cần mainKey
curl -fsS -H "Authorization: Bearer $MAIN_KEY" \
  http://127.0.0.1:8787/v1/models | head -c 400

# 3. Stream thật (phải thấy từng chunk, không bị gom)
curl -N -sS -H "Authorization: Bearer $MAIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"<model-id>","stream":true,"messages":[{"role":"user","content":"ping"}]}' \
  http://127.0.0.1:8787/v1/chat/completions | head -c 800
```

VPS qua Caddy:

```bash
curl -fsS https://gw.example.com/healthz
curl -sS -o /dev/null -w '%{http_code}\n' https://gw.example.com/v1/models   # 401 khi không có key
curl -sS -o /dev/null -w '%{http_code}\n' https://gw.example.com/admin      # 404 (bị chặn)
curl -sS -o /dev/null -w '%{http_code}\n' https://gw.example.com/admin/api/state  # 404
curl -fS  -H "Authorization: Bearer $MAIN_KEY" https://gw.example.com/v1/models
```

Kiểm tra supervisor và bind:

```bash
systemctl is-active loimo caddy
journalctl -u loimo -n 50 --no-pager
ss -ltnp | grep -E '8787|8788'   # phải thấy 127.0.0.1, không phải 0.0.0.0
```

Windows (PowerShell):

```powershell
(Invoke-RestMethod http://127.0.0.1:8787/healthz).ok
Get-NetTCPConnection -LocalPort 8787 -State Listen | Select-Object LocalAddress,LocalPort
```

Tiêu chí đạt:

- `/healthz` trả `{"ok":true,...}` ở cả ba môi trường.
- `/v1/models` trả `401` khi thiếu key và `200` khi có `mainKey` đúng.
- Stream trả chunk ngay, không đợi hết phản hồi.
- `https://<domain>/admin*` trả `404`/`403`, không trả dashboard.
- `ss`/`Get-NetTCPConnection` xác nhận cổng gateway chỉ bind loopback.
- `systemctl is-active loimo` trả `active` trên WSL và VPS.

## Sự cố thường gặp

| Triệu chứng | Nguyên nhân thường gặp | Xử lý |
|---|---|---|
| Windows mở được `8787`, WSL không | WSL chưa có systemd hoặc unit chưa enable | Bật `systemd=true`, `daemon-reload`, `enable --now` |
| Hai gateway tranh cùng file Agent | `agentScope: all` ở cả hai, hoặc cùng `windows`/`wsl` | Đặt một owner mỗi phạm vi; instance còn lại `--no-inject` |
| VPS trả `404` cho mọi đường dẫn | Caddy chưa reload/không khớp domain, hoặc chưa mở `443` | `caddy validate`, `systemctl reload caddy`, kiểm tra DNS/firewall |
| `/admin` truy cập được từ Internet | Caddy thiếu rule chặn hoặc proxy cả `/` | Sửa Caddyfile theo mẫu; kiểm tra lại bằng `curl` bên ngoài |
| Stream bị gom thành một cục | Reverse proxy buffer | Giữ `flush_interval -1` trong Caddyfile |
| `401` dù đã đặt key | Sai `mainKey` trên máy đó (config bị chép từ máy khác) | Sinh `mainKey` riêng, cập nhật client |
| Relay `403` | Token sai/hết hiệu lực hoặc upstream không nằm trong allowlist | Xoay token hai phía, kiểm tra allowlist |

## Liên quan

- [RELAY.md](RELAY.md) — giao thức relay, header, allowlist, xoay token.
- [deploy/config.example.json](../deploy/config.example.json) — mẫu không chứa secret.
- [deploy/wsl/loimo.service](../deploy/wsl/loimo.service)
- [deploy/vps/loimo.service](../deploy/vps/loimo.service)
- [deploy/vps/Caddyfile](../deploy/vps/Caddyfile)
- [deploy/windows/README.md](../deploy/windows/README.md)
- [../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)
