# relay (Bun) — triển khai, giao thức, vận hành

Tài liệu này mô tả **giao thức mức cao** và các bước triển khai relay. Mã
nguồn relay mẫu nằm ở `relay-server/`; tài liệu này không định nghĩa lại mã.

OpenKi hiểu hai giao thức relay trên cùng một `relayPool`:

| `kind` | Giao thức | Dùng khi |
|---|---|---|
| rỗng hoặc `openki` | `X-OpenKi-Relay-*` (native) | Relay Bun trong `relay-server/`, VPS tự dựng |
| `polygate` hoặc `vercel` | `X-Relay-*` (Polygate/opencode-relay) | Relay Vercel đã triển khai bởi Polygate hoặc opencode-relay |

`kind` chỉ chọn cách đóng gói request; cả hai vẫn là peer ngang hàng trong
`egressModes` và cùng cơ chế xoay vòng, failover, giới hạn theo token.

## Vai trò

Relay là một forwarding HTTPS mỏng: gateway gửi request OpenAI/Anthropic tới
relay, relay xác thực token, kiểm tra đích upstream nằm trong allowlist, rồi
chuyển tiếp nguyên method/path/body và stream phản hồi về. Relay không giữ
prompt, không cache nội dung, không thay đổi định dạng đường dây.

`relay` là **một trong ba egress đồng cấp** (`relay`, `singbox`, `warp`) trong
`egressModes`; nó không phải lớp ưu tiên cao hơn hay thấp hơn hai chế độ kia.

## Giao thức

### Request từ gateway — native (`kind` rỗng/`openki`)

| Thành phần | Giá trị |
|---|---|
| Method/path | Giữ nguyên đường dẫn API, ví dụ `POST /v1/chat/completions`, `POST /v1/responses`, `POST /v1/messages`, `GET /v1/models` |
| `Authorization` | Giữ nguyên key của gateway/upstream nếu gateway gửi kèm |
| `X-OpenKi-Relay-Token` | Token của relay; **bắt buộc**, so sánh constant-time |
| `X-OpenKi-Relay-Upstream` | `scheme://host` của upstream (thường `https://host`), **bắt buộc**; relay kiểm tra **host** theo allowlist |
| Body | Giữ nguyên JSON; relay không parse/sửa nội dung |

### Request từ gateway — Polygate/Vercel (`kind: "polygate"`)

| Thành phần | Giá trị |
|---|---|
| Method | Giữ nguyên method của request gốc |
| URL gọi relay | Chính `relayPool[].url` (thường `https://<project>.vercel.app`), **không** nối path upstream vào URL |
| `X-Relay-Target` | `scheme://host` của upstream, ví dụ `https://api.example.com`; **bắt buộc** |
| `X-Relay-Path` | Path + query của upstream, ví dụ `/v1/chat/completions?stream=1`; **bắt buộc** |
| `X-Relay-Key` | Token relay; **bắt buộc**, relay so sánh constant-time |
| Body | Giữ nguyên JSON; relay không parse/sửa nội dung |

Gateway xóa mọi header `X-Relay-*` do client gửi trước khi đặt bộ header trên,
để token/đích của client không thể ghi đè cấu hình. Relay Polygate trả thêm
`x-relay-egress`, `x-relay-runtime`, `x-relay-instance`; các header này chỉ để
chẩn đoán, gateway không dựa vào chúng để chọn peer.

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
| `401` | Thiếu/sai `X-OpenKi-Relay-Token` (native) hoặc `X-Relay-Key` (Polygate) |
| `403` | Đích không nằm trong allowlist, hoặc dùng `http://` khi chưa cho phép |
| `400` | Thiếu header bắt buộc hoặc đích không hợp lệ |
| `413` | Body vượt `OPENKI_RELAY_MAX_BODY_BYTES` |
| `503` | Relay chưa cấu hình token (fail closed) |
| `502`/`504` | Không kết nối được upstream hoặc upstream timeout |

Không trả chi tiết nội bộ (stack, danh sách allowlist, token) trong body lỗi.

## Bảo mật

- **HTTPS cho relay công khai.** `relayPool[].url` phải là HTTPS khi relay nằm
  ngoài máy; chỉ cho phép `http://` với loopback hoặc kiểm thử local. TLS kết
  thúc ở Vercel/VPS/Caddy; không chạy relay sau proxy không TLS.
- **Host allowlist.** Chỉ chuyển tiếp tới các host đã khai báo; từ chối IP
  literal, `localhost`, metadata endpoint và mọi đích ngoài allowlist. Kiểm tra
  sau khi chuẩn hóa hostname, chống `@`, redirect và DNS rebinding. Với relay
  Polygate/opencode-relay, allowlist mặc định chỉ `opencode.ai`; mở thêm host
  phải làm ở phía relay (`RELAY_ALLOWED_HOSTS`), không chỉ ở gateway.
- **Token tách biệt.** Mỗi môi trường/máy dùng token riêng; token không nằm
  trong repo, không ghi log, không đưa vào query string.
- **Giới hạn tài nguyên.** Đặt timeout kết nối/đọc hợp lý, giới hạn kích thước
  body và rate limit theo token nếu công khai.
- **Không log nội dung.** Chỉ log method, path, status, thời lượng; không log
  prompt, completion, `Authorization`, host đầy đủ, `X-OpenKi-Relay-Token` hay
  `X-Relay-Key`.

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
    { "id": "vps-fallback", "url": "https://relay-vps.example.com", "token": "<TOKEN>", "enabled": true },
    { "id": "polygate-sin", "kind": "polygate", "url": "https://polygate-relay-sin.vercel.app", "token": "<RELAY_KEY>", "enabled": true }
  ]
}
```

`kind` bỏ trống tương đương `openki`; `vercel` được chuẩn hóa thành
`polygate`. Với relay Polygate, `token` là `X-Relay-Key` do relay sinh, không
phải Vercel API token. OpenKi **không** lưu Vercel API token và không tự deploy
relay; việc tạo/xóa deployment nằm ở Polygate hoặc opencode-relay.

Trên dashboard, mở **Mạng → Relay**: tick **Bật Relay**, bấm **+ Thêm relay**,
nhập URL/token, chọn **OpenKi native** hoặc **Polygate / Vercel**, rồi bấm
**Lưu thay đổi** ở cuối trang. Có thể sửa/xóa từng dòng; token được giữ nguyên
khi chỉ sửa URL.

Mỗi dòng có nút **Test** để dò IP công khai, quốc gia và độ trễ qua đúng relay
đó; **Test tất cả relay** dò toàn bộ danh sách. Kết quả hiện ngay trong thẻ
Relay và trong bảng **Đầu ra** (trạng thái Khả dụng/Lỗi, quốc gia, độ trễ), kể
cả khi relay chưa được lưu. Nút **Dò tất cả đầu ra** cũng dò relay, node đăng
ký và đường ra hiện tại trong một lượt.

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

## Tham chiếu giao thức

Giao thức `X-Relay-*` và cách triển khai relay Vercel được tham chiếu từ hai dự
án sau; OpenKi chỉ triển khai phía client, không sao chép mã nguồn của họ:

- [B3hnamR/Polygate](https://github.com/B3hnamR/Polygate) (MIT) — template
  relay Bun trên Vercel và bộ header `x-relay-target`, `x-relay-path`,
  `x-relay-key`.
- [mahyartdb/opencode-relay](https://github.com/mahyartdb/opencode-relay) (MIT)
  — bản relay kế thừa Polygate, giữ nguyên bộ header và thêm allowlist mặc
  định `opencode.ai`; dùng để đối chiếu hành vi failover và probe.

## Liên quan

- [DEPLOY.md](DEPLOY.md) — ma trận Windows/WSL/VPS và smoke test.
- [../deploy/config.example.json](../deploy/config.example.json) — mẫu `relayPool`.
- [../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) — giấy phép relay mẫu (MIT).
