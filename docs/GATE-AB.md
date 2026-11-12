# A/B cổng fingerprint và transport

Cổng client của OpenCode là một **body shape**, không phải header: nó so khớp
định nghĩa `read`/`shell` (mô tả + schema) và User-Agent. Vì vậy cách duy nhất
để biết cổng còn bắt buộc hay không là gửi hai lượt giống hệt nhau, một lượt có
tiêm fingerprint và một lượt không, rồi so kết quả. `gateprobe` làm đúng việc
đó; `loimo -gate-check` dùng cùng một hàm.

Tài liệu này ghi lại cách đo và kết quả đo trên máy này. Chính sách egress,
cooldown và governor token nằm ở [EGRESS-POLICY.md](EGRESS-POLICY.md).

## Cách chạy

```bash
# Một cặp A/B, đường trực tiếp (bỏ qua HTTP(S)_PROXY của shell):
go run -tags headless ./cmd/gateprobe -model glm-5.3-flash-free -via direct

# Năm cặp A/B để đánh giá độ nhiễu:
go run -tags headless ./cmd/gateprobe -model glm-5.3-flash-free -via direct -n 5

# Qua proxy pool tĩnh trong config.json (hoặc chỉ định -pool):
go run -tags headless ./cmd/gateprobe -model glm-5.3-flash-free -via pool
go run -tags headless ./cmd/gateprobe -via pool -pool 'socks5h://127.0.0.1:13001,direct'

# Qua WARP (nếu máy ra được UDP tới Cloudflare):
go run -tags headless ./cmd/gateprobe -model glm-5.3-flash-free -via warp
```

Mỗi cặp tốn hai request thật trên làn miễn phí. `-n` tối đa 10. `-via direct`
đặt transport trực tiếp, không kế thừa `HTTPS_PROXY`; `-via pool` cài rotator
tĩnh và đi qua đúng các exit đã cấu hình; `-via warp` bật pool WARP userspace
(nếu WARP không lên, request fallback về transport nền và log nói rõ). Cả ba
đường đều in `fingerprint hash` — hash của profile đang gửi (UA + mô tả +
schema), để so giữa hai lần chạy mà không phải diff body.

Kết quả `INCONCLUSIVE` không phải lỗi của công cụ: nó nói rằng **cả hai** phía
đều bị chặn bởi một lý do khác (quota/region/proxy), nên chưa nhìn thấy cổng.
Đừng đổi `fingerprintMode` dựa trên một lần INCONCLUSIVE.

## Kết quả đo 2026-11-11

Máy: WSL Debian, làn miễn phí qua gateway đang chạy; profile mặc định
`opencode/latest/2.1.3/cli`, hash `dfca0a028c169fd8`. Model `space-bunny-free`
đã trả `Model is unavailable` nên dùng `glm-5.3-flash-free` và
`deepseek-v4.1-flash-free`.

| Via | Model | Số cặp | Có fingerprint | Không fingerprint | Kết luận |
|---|---|---|---|---|---|
| direct | glm-5.3-flash-free | 2 | 429 `FreeUsageLimitError` | 429 `FreeUsageLimitError` | INCONCLUSIVE |
| direct | glm-5.3-flash-free | 3 (23:19) | 429 `FreeUsageLimitError` | 429 `FreeUsageLimitError` | INCONCLUSIVE |
| pool (8 exit tĩnh) | glm-5.3-flash-free | 1 | 429 `FreeUsageLimitError` | 429 `FreeUsageLimitError` | INCONCLUSIVE |
| pool (8 exit tĩnh) | deepseek-v4.1-flash-free | 2 | 429 `FreeUsageLimitError` | 429 `FreeUsageLimitError` | INCONCLUSIVE |
| warp | glm-5.3-flash-free | 1 | 429 sau khi WARP fallback (IPv6 unreachable) | 429 cùng lý do | INCONCLUSIVE |

Lần chạy direct đầu tiên (trước khi `-via direct` được thêm) còn cho thấy một
biến nhiễu khác: `HTTPS_PROXY=127.0.0.1:8888` của shell WSL đã chết, nên phía
có fingerprint nhận `TIMEOUT` 30 s trong khi phía không fingerprint nhận 429.
Đây là lý do `gateprobe` luôn đặt transport tường minh thay vì dựa vào môi
trường.

Ba cặp direct lúc 23:19 dùng đúng hash `dfca0a028c169fd8`; cả 6 request đều
trả 429 `FreeUsageLimitError` trong 0,25–1,5 s. Đây là bằng chứng quota đang che
cổng, không phải bằng chứng cổng đã đổi.

Kết luận từ bảng: **chưa đo được cổng trong phiên này**. Mọi exit khả dụng
(direct, 8 proxy tĩnh, WARP fallback) đều trả cùng một `FreeUsageLimitError`
cho cả hai body, nên không có phía nào chứng minh được định nghĩa `read`/`shell`
còn bắt buộc hay không. Gateway đang chạy `fingerprintMode=auto` cũng nhận 429
y hệt, tức fingerprint không phải nguyên nhân của đợt 429 hiện tại. Giữ nguyên
`auto`; chạy lại khi có exit/IP còn hạn mức.

## Đo transport (không chỉ cổng)

`gateprobe` so **body**, không so transport. Muốn biết một exit/peer cụ thể có
đi được không, gửi một lượt chat thật và đọc header:

```bash
curl -sD - -o /dev/null http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer $LOIMO_KEY" -H 'content-type: application/json' \
  -d '{"model":"glm-5.3-flash-free","messages":[{"role":"user","content":"ping"}],"max_tokens":8}'
```

- `x-loimo-exit` là exit lane đã phục vụ (node id; với subscription, header
  này ổn định theo IP).
- `X-LoiMo-Egress` là peer transport (`relay`, `warp`, `singbox`) nếu có.
- `Retry-After` xuất hiện khi 429; `EXIT_GOVERNOR` chỉ xuất hiện khi governor
  nội bộ từ chối, không phải upstream.

Ví dụ quan sát được trong phiên điều tra: cùng mô hình trả 200 lúc 14:28:57 và
`REGION_BLOCKED` lúc 14:31:10 khi exit đổi, rồi 6/6 request 200 ngay sau đó.
Đó là bằng chứng region là verdict của IP đầu ra, không phải của mô hình — chi
tiết trong [EGRESS-POLICY.md](EGRESS-POLICY.md#bằng-chứng-đo-được).
