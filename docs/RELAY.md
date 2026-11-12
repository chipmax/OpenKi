# relay (Bun) — triển khai, giao thức, vận hành

Tài liệu này chỉ mô tả **giao thức mức cao** và các bước triển khai relay. Mã
nguồn relay nằm ở `relay-server/`; tài liệu này không định nghĩa lại mã.

## Vai trò

Relay là một forwarding HTTPS mỏng: gateway gửi request OpenAI/Anthropic tới
relay, relay xác thực token, kiểm tra đích upstream nằm trong allowlist, rồi
chuyển tiếp nguyên method/path/body và stream phản hồi về. Relay không giữ
prompt, không cache nội dung, không thay đổi định dạng đường dây.

`relay` là **một trong ba egress đồng cấp** (`relay`, `singbox`, `warp`) trong
`egressModes`; nó không phải lớp ưu tiên cao hơn hay thấp hơn hai chế độ kia.

## Giao thức

### Request từ gateway

| Thành phần | Giá trị |
|---|---|
| Method/path | Giữ nguyên đường dẫn API, ví dụ `POST /v1/chat/completions`, `POST /v1/responses`, `POST /v1/messages`, `GET /v1/models` |
| `Authorization` | Giữ nguyên key của gateway/upstream nếu gateway gửi kèm |
| `X-OpenKi-Relay-Token` | Token của relay; **bắt buộc**, so sánh constant-time |
| `X-OpenKi-Relay-Upstream` | `scheme://host` của upstream (thường `https://host`), **bắt buộc**; relay kiểm tra **host** theo allowlist |
| Body | Giữ nguyên JSON; relay không parse/sửa nội dung |

Relay trả về đúng status/header/body của upstream. Với phản hồi streaming, relay
phải chuyển tiếp từng chunk ngay (không gom buffer) và giữ kết nối mở tới khi
upstream kết thúc.

### Health

`GET /healthz` trả `200` với JSON dạng
`{"ok":true,"region":"<region>","version":"<version>"}`; không yêu cầu token,
không lộ cấu hình hay allowlist. Dùng endpoint này cho health check của Vercel,
VPS, Caddy/uptime monitor.

### Lỗi

| Status | Khi nào |
|---|---|
| `401` | Thiếu/sai `X-OpenKi-Relay-Token` |
| `403` | `X-OpenKi-Relay-Upstream` không nằm trong allowlist, hoặc dùng `http://` khi chưa cho phép |
| `400` | Thiếu header bắt buộc hoặc đích không hợp lệ |
| `413` | Body vượt `OPENKI_RELAY_MAX_BODY_BYTES` |
| `503` | Relay chưa cấu hình `OPENKI_RELAY_TOKEN` (fail closed) |
| `502`/`504` | Không kết nối được upstream hoặc upstream timeout |

Không trả chi tiết nội bộ (stack, danh sách allowlist, token) trong body lỗi.

## Bảo mật

- **Chỉ HTTPS.** Không chấp nhận `http://` cho `url` trong `relayPool` và
  không chạy relay sau proxy không TLS. TLS kết thúc ở Vercel/VPS/Caddy.
- **Host allowlist.** Chỉ chuyển tiếp tới các host đã khai báo; từ chối IP
  literal, `localhost`, metadata endpoint và mọi đích ngoài allowlist. Kiểm tra
  sau khi chuẩn hóa hostname, chống `@`, redirect và DNS rebinding.
- **Token tách biệt.** Mỗi môi trường/máy dùng token riêng; token không nằm
  trong repo, không ghi log, không đưa vào query string.
- **Giới hạn tài nguyên.** Đặt timeout kết nối/đọc hợp lý, giới hạn kích thước
  body và rate limit theo token nếu công khai.
- **Không log nội dung.** Chỉ log method, path, status, thời lượng; không log
  prompt, completion, `Authorization`, host đầy đủ hay `X-OpenKi-Relay-Token`.

## Vị trí triển khai

| Vị trí | Vai trò | Lý do |
|---|---|---|
| Vercel (Bun), hai region | Chính | Chạy gần upstream, độ trễ thấp; hai region để chịu lỗi vùng |
| VPS (Bun) | Phụ / dự phòng | Cùng hoặc khác hạ tầng gateway; bật khi Vercel suy giảm |
| Cloudflare Worker | **Không dùng** | Không kiểm soát được TLS fingerprint theo yêu cầu của luồng miễn phí OpenCode |

Không đặt Cloudflare Worker làm relay cho làn miễn phí OpenCode: TLS fingerprint
do Worker tạo ra không khớp luồng mà upstream chấp nhận, dẫn tới chặn/403 dù
token và allowlist đúng.

## Các bước triển khai

### 1. Chuẩn bị secret và allowlist

Sinh token đủ dài (ví dụ 32 byte ngẫu nhiên, mã hóa base64url). Chuẩn bị danh
sách host upstream được phép, chỉ gồm host thật sự cần:

```text
OPENKI_RELAY_TOKEN=<random-token>
OPENKI_RELAY_HOSTS=api.example-upstream.com,api.example-2.com
OPENKI_RELAY_REGION=sin1
# tuỳ chọn: OPENKI_RELAY_MAX_BODY_BYTES, OPENKI_RELAY_TIMEOUT_MS
```

Không đặt `OPENKI_RELAY_ALLOW_HTTP=1` trên môi trường thật; biến này chỉ dành
cho kiểm thử local với upstream `http://`. Không commit các giá trị trên. Trên
Vercel dùng Environment Variables (Production/Preview tách biệt); trên VPS dùng
file `EnvironmentFile=` quyền `0600` hoặc secret manager.

### 2. Triển khai Vercel (chính, hai region)

1. Tạo project từ thư mục `relay-server/` theo hướng dẫn trong đó.
2. Đặt `OPENKI_RELAY_TOKEN`, `OPENKI_RELAY_HOSTS` trong Project Settings →
   Environment Variables.
3. Cấu hình hai region gần upstream (ví dụ Singapore + Frankfurt, hoặc theo
   vị trí upstream thực tế); đặt `OPENKI_RELAY_REGION` khác nhau nếu muốn
   phân biệt trong log/header.
4. Deploy, sau đó kiểm tra `GET https://<vercel-domain>/healthz`.
5. Ghi lại URL HTTPS của từng region để đưa vào `relayPool`.

### 3. Triển khai VPS (phụ/dự phòng)

1. Cài Bun và chạy relay dưới systemd (service riêng, user riêng, `Restart=always`).
2. Chỉ bind loopback nếu Caddy cùng máy đứng trước; hoặc bind sau firewall nếu
   dùng TLS trực tiếp. Không mở cổng relay trần ra Internet. Cổng nội bộ mặc
   định là `8789` (`PORT` có thể override).
3. Đặt `OPENKI_RELAY_TOKEN`, `OPENKI_RELAY_HOSTS` (và tuỳ chọn
   `OPENKI_RELAY_REGION`) qua `EnvironmentFile` quyền `0600`.
4. Caddy/nginx đứng trước phải stream không buffer và chỉ proxy các đường dẫn
   cần thiết; không proxy trang quản trị của relay nếu có.
5. Kiểm tra `GET https://<vps-domain>/healthz`.

### 4. Khai báo trong gateway

Thêm từng relay vào `relayPool` với `id`, `url`, `token`, `enabled`:

```json
{
  "egressModes": ["relay"],
  "relayPool": [
    { "id": "vercel-sin", "url": "https://relay-sin.example.com", "token": "<TOKEN>", "enabled": true },
    { "id": "vercel-fra", "url": "https://relay-fra.example.com", "token": "<TOKEN>", "enabled": true },
    { "id": "vps-fallback", "url": "https://relay-vps.example.com", "token": "<TOKEN>", "enabled": true }
  ]
}
```

`url` phải là HTTPS. Bật/tắt từng mục bằng `enabled`; không cần xóa mục khi
tạm ngưng. Token nhập tay trên từng máy, không đồng bộ qua file mẫu.

### 5. Kiểm tra chấp nhận

```bash
# Health trực tiếp
curl -fsS https://relay-sin.example.com/healthz

# Thiếu token phải bị từ chối
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H 'X-OpenKi-Relay-Upstream: https://api.example-upstream.com' \
  https://relay-sin.example.com/v1/models          # 401

# Sai đích phải bị từ chối
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "X-OpenKi-Relay-Token: $OPENKI_RELAY_TOKEN" \
  -H 'X-OpenKi-Relay-Upstream: https://169.254.169.254' \
  https://relay-sin.example.com/v1/models          # 403

# Đích hợp lệ + token đúng (status phụ thuộc upstream)
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "X-OpenKi-Relay-Token: $OPENKI_RELAY_TOKEN" \
  -H 'X-OpenKi-Relay-Upstream: https://api.example-upstream.com' \
  https://relay-sin.example.com/v1/models
```

Sau đó kiểm tra qua gateway: `egressModes` có `relay`, `/v1/models` trả `200`,
và một request streaming trả chunk ngay. Nếu tất cả relay `enabled: false`,
gateway phải vẫn chạy với egress còn lại (nếu có) và trả lỗi rõ ràng, không treo.

## Xoay token

Mẫu relay hiện xác thực **một** `OPENKI_RELAY_TOKEN`, nên xoay token có một
cửa sổ ngắn request bị `401`. Quy trình:

1. Sinh token mới; cập nhật `OPENKI_RELAY_TOKEN` trên **tất cả** instance
   relay (Vercel hai region + VPS) và `relayPool[].token` trên từng máy.
2. Cập nhật `relayPool[].token` bằng tay (không chép file), ưu tiên cùng lúc
   với deploy relay.
3. Kiểm tra request mới qua relay thành công; nếu còn `401`, kiểm tra instance
   nào chưa nhận token mới.
4. Xác nhận token cũ trả `401`, rồi xóa khỏi secret manager.
5. Ghi lại thời điểm xoay; không tái sử dụng token đã thu hồi.

Muốn tránh gián đoạn hoàn toàn, chạy tạm một bản relay chấp nhận cả hai token
(hoặc một instance thứ hai), chuyển gateway sang token mới, rồi bỏ token cũ —
việc hỗ trợ hai token phải được triển khai trong mã relay, không chỉ cấu hình.

## Liên quan

- [DEPLOY.md](DEPLOY.md) — ma trận Windows/WSL/VPS và smoke test.
- [../deploy/config.example.json](../deploy/config.example.json) — mẫu `relayPool`.
- [../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) — giấy phép relay mẫu (MIT).
