# zen-gate-relay

Relay egress tối giản cho **zen-gate**, viết bằng Bun + TypeScript. Nhận request từ
zen-gate, xác thực token và allowlist, rồi chuyển tiếp nguyên vẹn sang upstream
(ví dụ `https://opencode.ai`) và stream response về client.

Mục đích: đưa request OpenCode Zen ra khỏi IP của máy chạy zen-gate, dùng được
cho nhiều máy/Agent mà không mở proxy công khai.

- **Vercel Bun** — relay chính, nên chạy hai project/region (ví dụ `sin1` và
  `hnd1`) để có hai đầu ra.
- **VPS Bun (Docker)** — relay phụ/fallback, đứng sau Caddy TLS.
- **Cloudflare Worker không phù hợp** cho làn miễn phí OpenCode: TLS fingerprint
  của Workers khác Bun, upstream dễ chặn. Dùng Bun runtime ở cả hai nơi.

## Hợp đồng request

Mọi path trừ `GET /healthz` đều là proxy. Client gửi:

| Header | Bắt buộc | Ý nghĩa |
|---|---|---|
| `X-Zen-Relay-Token` | Có | Shared secret, so sánh constant-time với `ZEN_GATE_RELAY_TOKEN`. |
| `X-Zen-Relay-Upstream` | Có | Chỉ gồm `scheme://host`, ví dụ `https://opencode.ai`. Không kèm path/query/credential. |

Relay giữ nguyên method, path, query, body và các header end-to-end
(`Authorization`, `x-api-key`, `anthropic-version`, `content-type`, ...).
Các header sau bị loại bỏ trước khi forward:

- Hop-by-hop: `connection`, `keep-alive`, `proxy-*`, `te`, `trailer`,
  `transfer-encoding`, `upgrade`, cùng các header được liệt kê trong `Connection`.
- Toàn bộ `X-Zen-Relay-*` (token và upstream không bao giờ tới upstream).

Response của upstream được **stream nguyên vẹn** (không buffer, SSE hoạt động),
giữ status và header an toàn. Relay dùng `fetch(..., { decompress: false })` nên
không giải nén, không biến đổi body; `Content-Encoding` và `Content-Length` của
upstream được giữ đúng. Redirect (`3xx`) được trả về client, relay không tự đi
theo. Response có thêm:

- `X-Relay-Region`: lấy từ `ZEN_GATE_RELAY_REGION`.
- `X-Relay-Egress`: best effort từ `cf-connecting-ip`, `x-forwarded-for` (IP đầu
  tiên) hoặc `x-real-ip`.

Không có CORS. Không log token, header hay body; log chỉ gồm method, path,
status, thời gian và region.

## Biến môi trường

| Tên | Bắt buộc | Mặc định | Ghi chú |
|---|---|---|---|
| `ZEN_GATE_RELAY_TOKEN` | Có | — | Rỗng ⇒ mọi request proxy trả `503` (fail closed). Sinh bằng `openssl rand -hex 32`. |
| `ZEN_GATE_RELAY_HOSTS` | Có | — | Danh sách host phân tách bằng dấu phẩy. Rỗng ⇒ mọi host bị từ chối (`403`), không có proxy mở. |
| `ZEN_GATE_RELAY_REGION` | Không | `local` (hoặc `VERCEL_REGION`) | Nhãn trả trong `X-Relay-Region`. |
| `ZEN_GATE_RELAY_ALLOW_HTTP` | Không | tắt | Chỉ `1` mới cho phép upstream `http://`; production phải để trống/`0`. |
| `ZEN_GATE_RELAY_MAX_BODY_BYTES` | Không | `67108864` (64 MiB) | Chặn body request lớn hơn; vượt ⇒ `413`. |
| `ZEN_GATE_RELAY_TIMEOUT_MS` | Không | `300000` (300 s) | Tổng thời gian tối đa của một request upstream; hết hạn ⇒ `504`. |
| `PORT` | Không | `8789` | Chỉ dùng local/Docker. |
| `HOST` | Không | `0.0.0.0` | Chỉ dùng local/Docker. |

Cổng mặc định của relay là **8789** để không đụng:

- Windows GUI/tray: `8787`
- WSL headless: `8788`
- zen-gate trên VPS: `127.0.0.1:8787` (sau Caddy TLS `443`)

Bun tự nạp `.env` khi chạy local. Không đóng gói `.env` vào image.

## Chạy local

```bash
cd relay-server
bun install
cp .env.example .env   # sửa token/hosts
bun run start
```

Kiểm tra nhanh:

```bash
curl -sS http://127.0.0.1:8789/healthz
# {"ok":true,"region":"local","version":"1.0.0"}

curl -sS http://127.0.0.1:8789/v1/models \
  -H "X-Zen-Relay-Token: $ZEN_GATE_RELAY_TOKEN" \
  -H "X-Zen-Relay-Upstream: https://opencode.ai"
```

Stream SSE (phải thấy dữ liệu về ngay, không bị gom):

```bash
curl -N http://127.0.0.1:8789/v1/chat/completions \
  -H "X-Zen-Relay-Token: $ZEN_GATE_RELAY_TOKEN" \
  -H "X-Zen-Relay-Upstream: https://opencode.ai" \
  -H "Content-Type: application/json" \
  -d '{"model":"<model>","stream":true,"messages":[{"role":"user","content":"ping"}]}'
```

Kiểm thử nội bộ (không dùng mạng ngoài): script tự mở upstream echo và relay
trên cổng ngẫu nhiên, kiểm tra token, allowlist, path/query/body, SSE streaming,
`Content-Encoding`, `502/504/413`.

```bash
bun run test/smoke.ts
bun run typecheck
```

## Triển khai trên Vercel (relay chính)

1. Tạo project Vercel từ repo, đặt **Root Directory** = `relay-server`.
2. Chạy `bun install` trong `relay-server` và commit `bun.lock`. Vercel chỉ nhận
   diện preset Bun (`Bun.serve()` toàn app) khi có `vercel.json` với
   `bunVersion`, **và** `bun.lock`, **và** entrypoint `server.ts`.
3. `vercel.json` đã đặt:
   - `"bunVersion": "1.4.x"`
   - `"functions": { "server.ts": { "maxDuration": 300 } }`
4. Khai báo Environment Variables cho từng môi trường (Production/Preview):
   `ZEN_GATE_RELAY_TOKEN`, `ZEN_GATE_RELAY_HOSTS`, `ZEN_GATE_RELAY_REGION`.
5. Tạo **hai project** (cùng repo, cùng Root Directory) và chọn Function Region
   khác nhau (`sin1`, `hnd1`). Đặt `ZEN_GATE_RELAY_REGION` tương ứng, ví dụ
   `vn-sin1`, `vn-hnd1`. Hai URL này là hai đầu ra độc lập trong `relayPool`.
6. Deploy: `bunx vercel --prod` (hoặc nối Git tự động).

Lưu ý Vercel:

- `maxDuration: 300` chỉ có hiệu lực trong giới hạn gói (Hobby thấp hơn
  Pro/Enterprise). Request dài hơn giới hạn sẽ bị nền tảng cắt.
- Body request của Vercel Functions có giới hạn riêng (vài MB). Với payload lớn
  gần 64 MiB, dùng relay VPS.
- Không bật thêm lớp nén phía trước cho `/v1/*`; có thể làm hỏng SSE hoặc
  `Content-Encoding`.

## Triển khai trên VPS (relay phụ/fallback)

Image chạy bằng user `bun` (non-root), không có dependency runtime:

```bash
cd relay-server
docker build -t zen-gate-relay .
docker run -d --name zen-gate-relay --restart unless-stopped \
  -p 127.0.0.1:8789:8789 \
  -e ZEN_GATE_RELAY_TOKEN='<openssl rand -hex 32>' \
  -e ZEN_GATE_RELAY_HOSTS='opencode.ai' \
  -e ZEN_GATE_RELAY_REGION='vn-vps' \
  zen-gate-relay
```

Chỉ publish ra loopback; TLS do Caddy đảm nhiệm. Ví dụ Caddy:

```caddyfile
relay.example.com {
    # Relay không có /admin; chặn sẵn để phòng cấu hình nhầm.
    @admin path /admin*
    respond @admin 404

    reverse_proxy 127.0.0.1:8789 {
        # Đẩy từng chunk ngay, cần cho SSE.
        flush_interval -1
    }
}
```

- Không thêm `encode` cho host relay.
- Panel/gateway zen-gate trên VPS (`127.0.0.1:8787`) **không** mở qua Caddy;
  truy cập bằng SSH tunnel: `ssh -L 8787:127.0.0.1:8787 user@vps`.
- Health check: `https://relay.example.com/healthz`.

## Mã lỗi

Mọi lỗi trả JSON `{"error":"<code>","message":"..."}`:

| Status | `error` | Nguyên nhân |
|---|---|---|
| `400` | `missing_upstream`, `invalid_upstream`, `invalid_request_url` | Thiếu/sai `X-Zen-Relay-Upstream`, có path/query/credential. |
| `401` | `unauthorized` | Thiếu hoặc sai token. |
| `403` | `upstream_not_allowed`, `insecure_upstream` | Host ngoài allowlist; upstream không phải https khi chưa opt-in. |
| `413` | `request_too_large` | Body vượt `ZEN_GATE_RELAY_MAX_BODY_BYTES`. |
| `502` | `upstream_unreachable` | Lỗi kết nối/DNS/TLS tới upstream. |
| `503` | `relay_not_configured` | Relay chưa có `ZEN_GATE_RELAY_TOKEN`. |
| `504` | `upstream_timeout` | Vượt `ZEN_GATE_RELAY_TIMEOUT_MS`. |

Timeout giữa lúc đang stream: response đã bắt đầu nên không thể đổi thành JSON;
kết nối bị đóng, client thấy stream kết thúc sớm.

## Bảo mật và xoay token

- Token so sánh constant-time trên SHA-256, không lộ độ dài.
- Allowlist rỗng từ chối tất cả; upstream phải là https trừ khi bật rõ
  `ZEN_GATE_RELAY_ALLOW_HTTP=1`.
- Không theo redirect, không có endpoint quản trị, không log secret.
- Mỗi entry trong `relayPool` có token riêng, nên xoay Vercel và VPS độc lập:
  1. Sinh token mới: `openssl rand -hex 32`.
  2. Cập nhật env của **một** relay rồi redeploy/restart container.
  3. Cập nhật `token` của entry tương ứng trong `relayPool` và reload zen-gate.
  4. Token cũ lập tức nhận `401`.
  Để tránh gián đoạn, thêm endpoint mới (token mới) vào `relayPool` trước, sau
  đó mới xoá endpoint cũ.

## Nối zen-gate vào relay

Các khoá cấu hình (store/config.json):

- `egressModes`: mảng, nhận `relay`, `singbox`, `warp`. Rỗng = dùng `proxyMode`
  kiểu cũ. `relay`, `singbox`, `warp` là **các lựa chọn ngang hàng**, có thể bật
  riêng hoặc kết hợp; không phải chuỗi ưu tiên.
- `relayPool`: mảng `{ id, url, token, enabled }`; mỗi entry là một relay.
- `agentScope`: `host`, `windows`, `wsl`, `all`, `none`; rỗng = tự động `host`.

Ví dụ:

```json
{
  "agentScope": "all",
  "egressModes": ["relay", "warp"],
  "relayPool": [
    { "id": "vercel-sin1", "url": "https://relay-sin1.example.com", "token": "<token>", "enabled": true },
    { "id": "vercel-hnd1", "url": "https://relay-hnd1.example.com", "token": "<token>", "enabled": true },
    { "id": "vps-hn", "url": "https://relay-vps.example.com", "token": "<token>", "enabled": true }
  ]
}
```

CLI liên quan: `--no-inject` (không tiêm cấu hình vào Agent),
`--inject-scope=<host|windows|wsl|all|none>`.

## Cấu trúc

```
relay-server/
  server.ts          # entrypoint Bun.serve cho Vercel preset
  src/app.ts         # xác thực, allowlist, forward, stream
  src/runtime.ts     # khởi động server, đọc env
  src/index.ts       # entrypoint local/Docker
  test/smoke.ts      # kiểm thử nội bộ, không dùng mạng ngoài
  Dockerfile         # oven/bun:1-alpine, non-root, cổng 8789
  vercel.json        # bunVersion 1.4.x, maxDuration 300
  .env.example
```

## Giấy phép

Phần `relay-server/` này theo **GPL-3.0-only**, cùng giấy phép với phần còn
lại của zen-gate-vi; xem `../LICENSE`.
