# Vận hành openki

Runbook cho mô hình đang dùng: một bản Windows có GUI/khay, một bản WSL headless,
và một VPS headless chỉ lộ API qua Caddy TLS. Cài đặt ban đầu nằm ở
[`DEPLOY.md`](DEPLOY.md); giao thức relay nằm ở [`RELAY.md`](RELAY.md).

## Nguyên tắc

- Mỗi instance có dữ liệu, cổng và key riêng. Không chép nguyên `config.json`
  giữa Windows, WSL và VPS: cổng, đường dẫn agent và egress khác nhau.
- Trang quản trị chỉ nghe loopback. Trên VPS, `/admin*` bị Caddy chặn; chỉ vào
  bằng SSH tunnel.
- `egressModes` là **tập peer được bật**, không phải chuỗi ưu tiên. `relay`,
  `singbox`, `warp` ngang hàng; có thể bật một hoặc nhiều cái cùng lúc.
- Thay đổi cấu hình bằng dashboard khi dịch vụ đang chạy. Nếu sửa tay
  `config.json`, dừng dịch vụ trước rồi mới sửa, tránh instance ghi đè khi lưu.

## Cổng, dữ liệu, giao diện

| Instance | Cổng | Giao diện | Thư mục dữ liệu |
|---|---|---|---|
| Windows | `127.0.0.1:8787` | GUI + khay hệ thống | `%APPDATA%\openki` |
| WSL (headless) | `127.0.0.1:8788` | trình duyệt | `$XDG_CONFIG_HOME/openki` (mặc định `~/.config/openki`) |
| VPS (headless) | `127.0.0.1:8787` nội bộ; Caddy TLS `443` | trình duyệt qua SSH tunnel | `<OPENKI_HOME>` (ví dụ `/var/lib/openki`) |

- Windows: mở `http://127.0.0.1:8787/admin`.
- WSL: mặc định `http://127.0.0.1:8787/admin` khi chỉ chạy một instance; dùng
  `8788` nếu chạy song song gateway Windows. WSL2 chuyển tiếp localhost.
  Đặt cổng bằng `"port"` trong `config.json` hoặc `--port` trong `ExecStart`;
  chỉ dùng một nguồn để tránh lệch cấu hình.
- VPS: API công khai đi qua `https://<domain>/v1/*`; quản trị **không** đi qua
  tên miền. Tạo tunnel:

  ```bash
  ssh -N -L 18787:127.0.0.1:8787 <user>@<vps-host>
  ```

  Sau đó mở `http://127.0.0.1:18787/admin`. Dùng cổng local `18787` để không
  đụng gateway Windows `8787`. Tunnel đặt `Host` về loopback nên qua được kiểm
  tra admin-only của gateway.

- Không mở trực tiếp cổng `8787`/`8788` ra Internet. Trên VPS kiểm tra listener
  chỉ nằm ở loopback:

  ```bash
  sudo ss -ltnp | grep -E ':(8787|8788)\b'
  ```

- Caddy phải chặn `/admin*` (mẫu trả `404`; `403` cũng chấp nhận):

  ```bash
  curl -s -o /dev/null -w '%{http_code}\n' https://<domain>/admin
  curl -s -o /dev/null -w '%{http_code}\n' https://<domain>/admin/api/state
  ```

  Kết quả mong đợi là `404` hoặc `403`, không bao giờ là `200`. Nếu là `200`,
  dừng việc mở rộng và sửa Caddy trước.

## Systemd và khởi động lại

WSL và VPS dùng cùng bộ lệnh; tên unit của gateway là `openki` (theo
`deploy/wsl/openki.service` và `deploy/vps/openki.service`).

```bash
sudo systemctl status openki
sudo systemctl restart openki
sudo systemctl reload-or-restart openki
sudo systemctl stop openki
```

Unit relay trên VPS (tên cụ thể theo mẫu triển khai):

```bash
sudo systemctl status <relay-unit>
sudo systemctl restart <relay-unit>
sudo systemctl reload caddy
```

Windows: thoát bằng menu khay trước khi backup/khôi phục. Kiểm tra tiến trình
còn sót:

```powershell
Get-Process openki -ErrorAction SilentlyContinue
```

Không dùng `taskkill`/`Stop-Process` như bước đầu tiên nếu còn thao tác ghi cấu
hình đang chạy; thoát từ khay để cửa sổ WebView2 và sidecar đóng sạch.

Headless bằng tay (không systemd) trên WSL/Linux:

```bash
OPENKI_HEADLESS=1 openki --port 8788
```

## Log

| Nguồn | Đường dẫn / lệnh |
|---|---|
| Gateway (mọi nền tảng) | `<OPENKI_HOME>/logs/<YYYY-MM-DD>.log` |
| Windows | `%APPDATA%\openki\logs\` |
| WSL | `~/.config/openki/logs/` |
| VPS | `journalctl -u openki` và `<OPENKI_HOME>/logs/` |
| sing-box sidecar | `<OPENKI_HOME>/subs/sing-box.log`; pid: `subs/sing-box.pid`; cấu hình: `subs/sing-box.json` |
| Relay | `journalctl -u <relay-unit>`; log nền tảng Vercel (theo dashboard Vercel) |
| Backup agent | `<OPENKI_HOME>/backups/<agent-id>/` |
| Dashboard | tab log trực tiếp; tải qua nút tải log |

Xem nhanh:

```bash
sudo journalctl -u openki -n 200 --no-pager
sudo journalctl -u openki -f
sudo journalctl -u <relay-unit> -n 200 --no-pager
tail -n 200 "$OPENKI_HOME/subs/sing-box.log"
```

Trên Windows, mở thư mục log:

```powershell
explorer "$env:APPDATA\openki\logs"
```

Mức log và loại log đặt trong Cài đặt (`logLevel`, `logCategories`,
`logKeepDays`). Khi điều tra sự cố, bật tạm `upstream`/`routing`; tắt lại sau khi
xong vì đây là hai lớp log dung lượng cao.

## Kiểm tra sức khỏe

`/healthz` (và `/health`) không cần key, chỉ trả liveness:

```bash
curl -fsS http://127.0.0.1:8787/healthz
# kỳ vọng: {"ok":true,"service":"openki"}

curl -fsS http://127.0.0.1:8788/healthz   # WSL
```

Kiểm tra API bằng key thật (đặt biến trong shell, không dán key vào tài liệu):

```bash
export LM_KEY='<main-key>'
curl -fsS -H "Authorization: Bearer $LM_KEY" http://127.0.0.1:8787/v1/models
```

Từ ngoài VPS, chỉ kiểm tra phần đã được phép lộ:

```bash
curl -fsS https://<domain>/healthz
curl -fsS -H "Authorization: Bearer $LM_KEY" https://<domain>/v1/models
```

Kiểm tra sidecar và cổng:

```bash
systemctl is-active openki
systemctl is-active caddy      # chỉ trên VPS
pgrep -a sing-box
ss -ltnp | grep -E ':(21001|21002)\b'   # socks inbound của từng node
```

Kiểm tra relay peer (health công khai, không cần token):

```bash
export RELAY_URL='https://<relay-host>'
export RELAY_TOKEN='<relay-token>'
export RELAY_UPSTREAM='<upstream-host>'

curl -fsS "$RELAY_URL/healthz"       # kỳ vọng: {"ok":true}

# Thiếu token phải bị từ chối (401)
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "X-OpenKi-Relay-Upstream: $RELAY_UPSTREAM" \
  "$RELAY_URL/v1/models"

# Token đúng + đích trong allowlist: status phụ thuộc upstream
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "X-OpenKi-Relay-Token: $RELAY_TOKEN" \
  -H "X-OpenKi-Relay-Upstream: $RELAY_UPSTREAM" \
  "$RELAY_URL/v1/models"
```

Đích ngoài allowlist phải trả `403`; token sai phải trả `401`. Chi tiết giao
thức và allowlist nằm trong [`RELAY.md`](RELAY.md).

## Egress: chọn và kiểm tra từng peer

### Cấu hình

`egressModes` là mảng, nhận bất kỳ giá trị nào trong `relay`, `singbox`,
`warp`, theo thứ tự nào cũng được. Bỏ trống mảng này thì giữ nguyên
hành vi cũ theo `proxyMode` (`env`, `system`, `direct`, `custom`, `rotate`,
`pool`, `warp`) cùng `warpEnabled`/`subsEnabled`.

`relayPool` là mảng peer relay, mỗi phần tử:

```json
{ "id": "vercel-1", "url": "https://<relay-primary>", "token": "<relay-token>", "enabled": true }
```

- `kind` tùy chọn: bỏ trống là relay OpenKi native; `polygate`/`vercel` là relay
  Polygate/opencode-relay dùng header `X-Relay-*`, token là `X-Relay-Key`.
- Token gửi qua header `X-OpenKi-Relay-Token` (native) hoặc `X-Relay-Key`
  (Polygate), bị che khi export cấu hình. `url` là base HTTPS, không có dấu `/`
  cuối; không dùng `http://` trên mạng.
- Chỉ peer `enabled: true` được dùng; request được trải trên các peer đang bật
  và thử peer kế tiếp khi tunnel chết.
- Đổi `enabled` để cô lập một peer khi kiểm tra. Bật `relay` mà không có peer
  nào dùng được thì gateway ghi chú và quay về transport nền; kiểm tra
  `relayPool` trước khi kết luận relay đang chạy.

Ví dụ bật relay + WARP cùng lúc (hai peer ngang hàng):

```json
{
  "port": 8787,
  "egressModes": ["relay", "warp"],
  "relayPool": [
    { "id": "vercel-1", "url": "https://<relay-primary>", "token": "<relay-token>", "enabled": true },
    { "id": "vps-1", "url": "https://<relay-secondary>", "token": "<relay-token>", "enabled": true }
  ]
}
```

Tắt toàn bộ egress peer: bỏ `egressModes`, đặt `proxyMode: "direct"`,
`warpEnabled: false`, `subsEnabled: false`. Không thêm giá trị lạ vào
`egressModes`; chỉ ba giá trị trên được hiểu.

### Nguyên tắc kiểm tra độc lập

Mỗi lần chỉ bật **một** peer để biết request đi đâu. Tắt các peer còn lại
(`relayPool[].enabled: false`, hoặc `egressModes` chỉ còn một giá trị), gửi một
request, rồi kiểm tra log và IP đầu ra. Bật lại lần lượt từng peer sau khi peer
trước đã đạt.

Request mẫu qua gateway:

```bash
curl -fsS http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer $LM_KEY" \
  -H 'content-type: application/json' \
  -d '{"model":"<model-id>","messages":[{"role":"user","content":"ping"}],"max_tokens":8}'
```

Khi `egressModes` đang bật, response có header `X-OpenKi-Egress` cho biết peer
đã phục vụ request (`relay`, `warp` hoặc `singbox`):

```bash
curl -sS -D - -o /dev/null http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer $LM_KEY" \
  -H 'content-type: application/json' \
  -d '{"model":"<model-id>","messages":[{"role":"user","content":"ping"}],"max_tokens":8}' \
  | grep -i '^x-openki-egress:'
```

Header này chỉ nói peer được chọn, không chứng minh đường ra thực tế đã đúng;
đối chiếu thêm log upstream/routing, log relay và IP mà upstream ghi nhận.
Dashboard có `egressIP`/`egressCountry` cho bài kiểm tra proxy legacy, không thay
thế kiểm tra riêng cho `egressModes`.

### Relay

Vị trí: Vercel (Bun) là chính, chạy **hai region**; VPS (Bun) là phụ/dự phòng.
Không dùng Cloudflare Worker cho làn miễn phí OpenCode vì TLS fingerprint do
Worker tạo không khớp luồng upstream chấp nhận, dù token/allowlist đúng.

1. `egressModes: ["relay"]`; chỉ để `enabled: true` cho đúng peer cần test.
2. `/healthz` của peer trả `200`; request qua gateway trả header
   `X-OpenKi-Egress: relay`.
3. Gửi request mẫu. Nếu peer chết mà peer khác đang bật, gateway có thể đã
   chuyển sang peer kia — vì vậy phải tắt các peer còn lại khi test.
4. Test peer dự phòng bằng cách đảo `enabled` giữa primary (Vercel) và
   secondary (VPS). Không sửa URL/token trong lúc test.
5. Nếu mọi peer cùng lỗi, kiểm tra token, DNS/TLS tới `<relay-host>`, và log
   relay. Lỗi TLS fingerprint upstream không sửa được bằng Cloudflare Worker;
   dùng relay Bun.

### WARP

1. `egressModes: ["warp"]` (hoặc legacy `warpEnabled: true`).
2. Gửi request mẫu. WARP cần UDP trực tiếp tới endpoint Cloudflare và HTTPS
   đăng ký thiết bị; mạng chỉ có HTTP proxy sẽ không dựng được pool.
3. Xem `warpFallbacks` trong `/admin/api/state` (hoặc dashboard) **trước và sau**
   request. Nếu bộ đếm tăng, request đã fallback về transport cũ: WARP chưa
   thực sự được dùng dù request vẫn thành công.
4. Header `X-OpenKi-Egress: warp` chỉ cho biết peer WARP được chọn; khi pool
   lỗi, WARP có thể fallback trong nội bộ peer. Vì vậy `warpFallbacks` và IP đầu
   ra mới là bằng chứng. WARP hỏng không làm chết request, nên “chat trả lời
   được” một mình không phải bằng chứng WARP hoạt động.

### sing-box

1. Bảo đảm có ít nhất một subscription/node đang bật và binary sing-box tồn tại
   (`<OPENKI_HOME>/bin/`, hoặc `singBoxPath` trỏ tới binary hợp lệ).
2. `egressModes: ["singbox"]`.
3. Kiểm tra `subs/sing-box.log`, pid và các cổng socks `21001+`.
4. Gửi request mẫu; header `X-OpenKi-Egress` phải là `singbox`, log sidecar
   phải có node, và IP đầu ra phải là node — không phải IP máy chạy gateway.
   Khi chưa có node, peer vẫn có thể dial trực tiếp, nên header một mình không
   đủ.
5. Nếu sidecar không lên: dừng mọi `zen-subs`/sing-box cũ đang giữ cổng
   `21001+` trước khi thử lại.

### Kết hợp

`["relay", "warp"]` hoặc `["relay", "singbox"]` nghĩa là cả hai lớp peer cùng
được phép dùng, không phải “thử relay trước, WARP sau”. Selector xoay vòng theo
từng request; lỗi transport, `429` hoặc `403` chuyển request sang peer kế tiếp
trước khi tầng retry quota của lane nhìn thấy response. Header
`X-OpenKi-Egress` trên từng response cho biết peer nào đã phục vụ request đó;
khi điều tra, luôn quay về một peer duy nhất, khi vận hành bình thường mới bật
kết hợp.

### Giới hạn token theo exit và phản hồi 429

Header `x-openki-exit` trên response của làn miễn phí cho biết đúng đầu ra đã
phục vụ lượt đó (`direct` khi không qua peer). Nó khác `X-OpenKi-Egress` của
relay: header sau là peer transport, header trước là exit đã được rotator chọn
và cũng là giá trị ghi trong `CallRecord.Exit`. Với stream, header chỉ kịp gửi
khi chưa có byte nội dung nào; nếu đã mở stream, trường `x_openki_exit` nằm
trong chunk kết thúc (OpenAI) hoặc `message_delta` (Anthropic).

Hạn mức miễn phí tính theo **IP đầu ra**, không theo mô hình. Đặt
`exitTokensPerMinute` trong `config.json` (hoặc Cài đặt) để governor giữ nhịp
token trên từng exit. Mặc định là **300.000 token/phút** khi khóa vắng mặt;
`0` là tắt hẳn. Governor ước lượng prompt + đệm `max_tokens` trước khi gửi, thử
tối đa ba exit khác khi exit ưa thích đã cạn, rồi chờ tối đa **20 giây**.
Quá hạn, request nhận `429` mã `EXIT_GOVERNOR` kèm `Retry-After` thay vì giữ
kết nối treo. Đây là ước lượng, không phải số đo tuyệt đối: nó chỉ có thể thu
rộng hơn, không thể tiêu ít hơn thực tế. Chi tiết đầy đủ về phân loại lỗi,
ramp làm nguội và các trường quan sát: [EGRESS-POLICY.md](EGRESS-POLICY.md).

```json
{
  "exitTokensPerMinute": 300000,
  "fingerprintMode": "auto"
}
```

Một `429` bắt đầu chuỗi 90 giây, gấp đôi sau mỗi lần lặp và chặn trên ở 15
phút; `Retry-After` kiểu “next UTC midnight” bị bỏ qua vì nó không phải thời
điểm hồi phục thật. Mô hình chỉ bị park 15 phút khi **vừa** có ba lần 429 trải
qua ít nhất 20 phút **vừa** một lần thăm dò độc lập cũng thấy giới hạn. Một lần
429 đơn lẻ không bao giờ đủ. Exit WARP/sing-box có chuỗi làm nguội riêng theo
cùng cấp số nhân, và ba lần lỗi trải qua ít nhất 20 phút mới bị coi là chặn dai
dẳng. `503`/`overloaded` chỉ retry ngắn trên cùng mô hình trước khi xét chuyển
sang mô hình khác, và không bao giờ bị coi là mô hình hỏng.

Lỗi relay được phân biệt để không đổ nhầm thành “sai key”:

| Dấu hiệu | Mã | Hành động |
|---|---|---|
| Vercel Authentication Protection | `DEPLOYMENT_PROTECTED` | Tắt Deployment Protection hoặc dùng bypass token |
| `unauthorized relay request` | `RELAY_UNAUTHORIZED` | Kiểm tra `relayPool[].token` |
| `address of this instance is used up` | `EGRESS_REFUSED` | Đổi exit; xoay sang peer khác |
| `overloaded` / `server is busy` / HTTP 529 | `OVERLOADED` | Retry ngắn 1–3 giây trên cùng mô hình |

### Kiểm tra cổng fingerprint

Làn Zen có thể vẫn yêu cầu đúng định nghĩa `read`/`shell` của OpenCode, nhưng
cổng này thay đổi theo thời gian. `gateprobe` gửi hai lượt chat tối thiểu —
một có định nghĩa, một không — rồi kết luận:

```bash
go run -tags headless ./cmd/gateprobe -model glm-5.3-flash-free -via direct
go run -tags headless ./cmd/gateprobe -model glm-5.3-flash-free -via pool -n 5
# hoặc bản đã cài:
openki -gate-check -gate-model glm-5.3-flash-free
```

`-via` chọn transport (`direct` bỏ qua proxy môi trường, `pool` dùng rotator
tĩnh, `warp` dùng pool WARP); `-n` lặp cặp A/B tối đa 10 lần. Mỗi lần chạy in
`fingerprint hash` của profile đang gửi (UA + mô tả + schema), để so hai lần
mà không cần diff body. Có thể override profile trong `config.json` bằng
`gateUserAgent` và `gateTools` (mô tả/schema của `read`/`shell`); hash và log
khởi động sẽ phản ánh giá trị thực tế.

Kết quả `FINGERPRINT_REQUIRED` nghĩa là giữ `fingerprintMode: "auto"`;
`GATE_OPEN` nghĩa là có thể thử `"off"`; `INCONCLUSIVE` nghĩa là cả hai lượt
đều lỗi (thường do quota/region) và phải chạy lại sau. Công cụ này tiêu hai
request thật trên làn miễn phí, nên chỉ chạy khi cần chẩn đoán. Đặt
`fingerprintMode` thành `"force"` chỉ khi muốn kiểm chứng một kênh không phải
Zen cũng chấp nhận cùng định nghĩa. Cách đo, kết quả ngày 2026-11-11 và cách
đọc header transport: [GATE-AB.md](GATE-AB.md).

## Xoay key

### mainKey

1. Backup `config.json` (mục sau).
2. Trong dashboard: Cài đặt → xoay vòng Key chính. Hoặc gọi từ loopback:

   ```bash
   curl -fsS -X POST http://127.0.0.1:8787/admin/api/key/rotate
   ```

   Endpoint trả key mới trong trường `key`; lưu vào trình quản lý mật khẩu.
   Với WSL, đổi cổng gọi thành `8788`; với VPS qua tunnel, đổi thành `18787`.
3. Cập nhật mọi client dùng mainKey (`Authorization: Bearer ...` hoặc
   `x-api-key`). Agent đã inject thường dùng agent key riêng; kiểm tra lại cấu
   hình agent sau khi xoay nếu client của bạn dùng mainKey.
4. Kiểm tra key cũ trả `401`, key mới trả `200`:

   ```bash
   curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer <new-key>" http://127.0.0.1:8787/v1/models
   ```

5. Nếu đang chạy nhiều instance, xoay riêng từng instance. Không export
   `?secrets=1` ra kênh không mã hóa.

Agent key là key con theo từng agent, không tự đổi khi xoay mainKey. Muốn đổi
key của agent, xoay/tắt-bật lại agent đó trong dashboard để inject lại.

### Token relay

Token relay là bí mật chia sẻ giữa gateway (`relayPool[].token`) và relay server.
Mỗi relay/region nên dùng token riêng; xoay lần lượt từng peer để không mất
toàn bộ đường relay cùng lúc.

1. Sinh token mới:

   ```bash
   openssl rand -hex 32
   ```

2. Cập nhật phía relay trước: đặt `RELAY_TOKEN` (và giữ nguyên
   `UPSTREAM_ALLOWLIST`) trong Environment Variables của Vercel rồi redeploy;
   trên VPS sửa `EnvironmentFile` quyền `0600` của unit relay, sau đó
   `sudo systemctl restart <relay-unit>`.
3. Nếu relay hỗ trợ hai token trong cửa sổ chuyển đổi, thêm token mới bên cạnh
   token cũ trước khi đổi gateway; nếu không, chấp nhận gián đoạn ngắn và đổi
   ngay.
4. Cập nhật `relayPool[].token` trên từng gateway. Dashboard áp dụng cấu hình;
   nếu sửa tay `config.json`, khởi động lại dịch vụ (`openki`).
5. Kiểm tra `healthz` và một request mẫu với token mới.
6. Thu hồi token cũ ở relay sau khi mọi gateway đã đổi. Kiểm tra token cũ bị từ
   chối (`401`), token mới thành công. Ghi lại thời điểm xoay; không tái sử
   dụng token đã thu hồi.

## Backup và khôi phục

Dữ liệu cần giữ: `config.json`, `stats.json`, `tags.json`, `quota.json`,
`perf.json`, `backups/`, `subs/` (chứa subscription URL và cấu hình sidecar),
`channels/`. Có thể bỏ `bin/` và binary tải lại được để giảm dung lượng.
Backup chứa key/token dạng rõ nếu chép cả `config.json` — cất ngoài repo, mã hóa
hoặc ổ ngoài.

### Windows

```powershell
# Thoát openki từ khay trước.
$lm  = Join-Path $env:APPDATA 'openki'
$dst = Join-Path $env:USERPROFILE ("openki-backup-" + (Get-Date -Format 'yyyyMMdd-HHmmss') + ".zip")
Compress-Archive -Path "$lm\*" -DestinationPath $dst
```

Khôi phục (khi đã thoát ứng dụng):

```powershell
$stamp   = Get-Date -Format 'yyyyMMdd-HHmmss'
$restore = Join-Path $env:APPDATA 'openki-restore'
Expand-Archive -Path '<backup>.zip' -DestinationPath $restore -Force
Move-Item (Join-Path $env:APPDATA 'openki') (Join-Path $env:APPDATA "openki.old-$stamp")
Move-Item $restore (Join-Path $env:APPDATA 'openki')
```

Mở lại ứng dụng, kiểm tra `/healthz`, `/v1/models`, trạng thái agent và egress.
Chỉ xóa `openki.old-*` sau khi đã xác nhận.

### WSL

```bash
LM_HOME="${OPENKI_HOME:-$HOME/.config/openki}"
sudo systemctl stop openki
tar -C "$(dirname "$LM_HOME")" -czf "$HOME/openki-$(date +%F-%H%M).tgz" "$(basename "$LM_HOME")"
```

Khôi phục:

```bash
sudo systemctl stop openki
LM_HOME="${OPENKI_HOME:-$HOME/.config/openki}"
mv "$LM_HOME" "$LM_HOME.bad-$(date +%F-%H%M)"
tar -C "$(dirname "$LM_HOME")" -xzf "$HOME/openki-<timestamp>.tgz"
sudo systemctl start openki
```

Chạy `tar`/`mv` bằng đúng user sở hữu `$LM_HOME` (trong unit mẫu là `admin`);
nếu dùng `sudo`, giữ nguyên chủ sở hữu khi khôi phục.

### VPS

```bash
sudo systemctl stop openki
sudo tar -C <OPENKI_HOME_PARENT> -czf /root/openki-$(date +%F-%H%M).tgz <OPENKI_HOME_BASENAME>
```

Ví dụ với `/var/lib/openki`: `-C /var/lib ... openki`.

Khôi phục:

```bash
sudo systemctl stop openki
sudo mv <OPENKI_HOME> <OPENKI_HOME>.bad-$(date +%F-%H%M)
sudo tar -C <OPENKI_HOME_PARENT> -xzf /root/openki-<timestamp>.tgz
sudo chown -R <service-user>:<service-user> <OPENKI_HOME>
sudo systemctl start openki
```

Luôn khôi phục khi dịch vụ đã dừng; không giải nén đè lên thư mục đang chạy.
Sau khôi phục, kiểm tra quyền file (`config.json` chỉ user dịch vụ đọc), log khởi
động, `/healthz`, `/v1/models` và một request mẫu.

Dashboard còn có export/import cấu hình:

```bash
curl -fsS http://127.0.0.1:8787/admin/api/export -o openki-config.masked.json
```

Mặc định export đã che key/token. `?secrets=1` chứa bí mật dạng rõ, chỉ dùng
trên máy tin cậy. Import chỉ áp dụng cấu hình, không mang theo stats/backup; cổng
đang chạy được giữ nguyên. Với WSL/VPS, đổi `8787` thành `8788`/`18787` tương
ứng.

## Dừng dịch vụ legacy (o2a / zen-subs)

Mục tiêu: không còn hai tiến trình cùng inject agent, cùng giữ cổng hoặc cùng
tranh chấp egress.

1. Kiểm kê unit, unit user và container:

   ```bash
   systemctl list-units --type=service --all | grep -Ei 'o2a|opencode2api|zen.?subs|sing-box'
   systemctl --user list-units --type=service --all | grep -Ei 'o2a|opencode2api|zen.?subs|sing-box'
   sudo docker ps --format '{{.Names}}\t{{.Image}}\t{{.Ports}}' | grep -Ei 'o2a|opencode2api|zen.?subs|sing-box'
   ```

2. Backup **trước khi dừng**: unit file, biến môi trường, thư mục dữ liệu và
   volume (nếu chạy Docker).

   ```bash
   sudo mkdir -p /root/legacy-backup
   sudo systemctl cat <unit> | sudo tee /root/legacy-backup/<unit>.service.txt >/dev/null
   sudo cp -a <legacy-config-dir> /root/legacy-backup/
   sudo docker inspect <container> | sudo tee /root/legacy-backup/<container>.inspect.json >/dev/null
   sudo docker inspect -f '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' <container>
   # Với từng đường dẫn mount in ra ở trên:
   sudo tar -C <mount-source> -czf /root/legacy-backup/<container>-volume.tgz .
   ```

3. Xác nhận openki đã phục vụ thay thế: model/agent hoạt động, relay hoặc egress
   đã cấu hình, và agent đã được inject bởi đúng instance.

4. Dừng mềm, chưa xóa:

   ```bash
   sudo systemctl stop <legacy-unit>
   sudo docker stop <container>                  # nếu là container
   sudo docker update --restart=no <container>
   ```

   Kiểm tra cổng đã nhả:

   ```bash
   sudo ss -ltnp | grep -E ':(8787|8788|21001|21002)\b'
   ```

5. Theo dõi ít nhất 24 giờ. Chỉ sau đó mới `sudo systemctl disable <legacy-unit>`.
   Không xóa dữ liệu, unit file hay volume trong cùng ngày.

6. Rollback:

   ```bash
   sudo systemctl enable --now <legacy-unit>
   ```

   Trước khi bật lại legacy, đặt `agentScope: "none"` hoặc chạy openki với
   `--no-inject` để tránh hai bên cùng ghi cấu hình agent. Với container, bật
   lại restart policy và `sudo docker start <container>`.

Lưu ý riêng cho `zen-subs`: sidecar sing-box của openki dùng cổng socks
`21001+` và pidfile trong `subs/`. Phải dừng `zen-subs`/sing-box cũ trước khi
bật `singbox` trong `egressModes`, nếu không sidecar mới không bind được cổng.

## Checklist ownership khi chạy hai instance

`agentScope` (store option) và `--inject-scope=<value>` (CLI, chỉ cho lần chạy)
nhận: `host`, `windows`, `wsl`, `all`, `none`. Bỏ trống = `all`. Đây là
**nền tảng đích của file agent**, không phải tên instance:

| Giá trị | Tác dụng |
|---|---|
| `host` | Chỉ agent của nền tảng đang chạy (Windows trên Windows; WSL trên WSL; Linux trên VPS) |
| `windows` | Chỉ cấu hình agent phía Windows, kể cả khi chạy trong WSL qua `/mnt/c` |
| `wsl` | Chỉ cấu hình agent phía WSL/Linux |
| `all` | Cả hai phía |
| `none` | Không inject |

`--no-inject` bỏ qua toàn bộ bước inject khi khởi động; dùng khi backup, khôi
phục, sửa tay cấu hình agent hoặc bàn giao instance. Cờ này không tự gỡ entry
đã inject trước đó — muốn gỡ, tắt agent trong dashboard trước.

Ví dụ chạy WSL ở cổng riêng và chỉ ghi cấu hình phía WSL trong lần chạy đó:

```bash
openki --port 8788 --inject-scope=wsl
```

Chạy không inject (ví dụ trong lúc backup/khôi phục):

```bash
openki --no-inject
```

Cờ CLI chỉ áp dụng cho tiến trình đang chạy; nó không sửa `agentScope` đã lưu.
`--inject-scope` thắng `agentScope` trong file; `--no-inject` thắng tất cả.

Checklist:

- [ ] Mỗi file cấu hình agent chỉ có **một** instance chủ. Thiết lập khuyến
      nghị: Windows chạy `agentScope: "windows"`, WSL chạy `agentScope: "wsl"`;
      không bên nào inject chéo.
- [ ] Nếu muốn WSL inject hộ cấu hình Windows (ví dụ `omp (Windows)`), đặt WSL
      `agentScope: "windows"` và Windows `agentScope: "none"`. Không để cả hai
      cùng `windows` hoặc cùng `all`.
- [ ] VPS dùng `--no-inject` (hoặc `agentScope: "none"`) trừ khi nó thực sự
      quản lý agent trên chính máy chủ đó.
- [ ] Mỗi instance cổng riêng: Windows `8787`, WSL `8788`; không chạy hai bản
      cùng cổng.
- [ ] Mỗi instance `mainKey`/agent key riêng; client trỏ đúng cổng và key.
- [ ] Trước khi đổi chủ: dừng instance cũ (hoặc khởi động lại với
      `--no-inject`/`agentScope: "none"`), xác nhận file agent không còn bị
      ghi, rồi mới bật instance mới.
- [ ] Kiểm tra backup `<OPENKI_HOME>/backups/<agent-id>/` có bản mới nhất;
      đây là đường lùi khi inject sai.
- [ ] Sau mỗi lần đổi scope/khởi động lại, mở dashboard agent của **cả hai**
      instance để xác nhận chỉ một bên báo `enabled` cho mỗi agent, rồi thử một
      request từ agent.
- [ ] Khi ngừng hẳn một instance, khởi động lại nó với `none`/`--no-inject`
      trước, sau đó mới tắt dịch vụ; không để instance chết ở trạng thái “vẫn
      sở hữu” cấu hình.

## Kiểm tra nhanh sau thay đổi

```bash
# Gateway sống
curl -fsS http://127.0.0.1:8787/healthz

# API và key đúng
curl -fsS -H "Authorization: Bearer $LM_KEY" http://127.0.0.1:8787/v1/models

# Egress đúng một peer, rồi gửi request mẫu ở trên
# WARP: warpFallbacks không tăng; sing-box: pid + log + cổng 21001;
# relay: log peer + /healthz + token mới.

# VPS: admin bị chặn từ Internet, tunnel loopback vẫn vào được
curl -s -o /dev/null -w '%{http_code}\n' https://<domain>/admin
curl -fsS http://127.0.0.1:18787/healthz   # trong phiên tunnel
```
