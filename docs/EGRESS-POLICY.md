# Chính sách egress và quan sát exit

Tài liệu này là tham chiếu cho tầng egress của làn miễn phí: cách một lần thử
được gán exit, khi nào exit bị làm nguội hoặc loại, governor token hoạt động
ra sao, và những gì được ghi lại để điều tra một chuỗi 429. Cấu hình mẫu nằm ở
[docs/OPERATIONS.md](OPERATIONS.md#giới-hạn-token-theo-exit-và-phản-hồi-429).

## Hai lớp egress, hai tên gọi

Một request có thể đi qua hai lớp độc lập:

1. **Exit của lane** — rotator chọn một node/IP cho request. Đây là đơn vị mà
   hạn mức miễn phí của upstream tính theo. Response mang
   `x-openki-exit`; với stream, header chỉ kịp gửi trước byte nội dung đầu
   tiên, sau đó giá trị nằm trong chunk kết thúc (`x_openki_exit`) hoặc
   `message_delta`.
2. **Peer transport** — relay / WARP / sing-box bọc đường đi ra ngoài. Response
   mang `X-OpenKi-Egress`; đây là peer đã phục vụ, không phải exit mà lane
   chọn.

Khi điều tra, đọc cả hai: một lỗi `429` trên peer `warp` và một `429` trên exit
`pool-3` là hai câu chuyện khác nhau. `CallRecord` ghi cả hai lớp qua
`Exit`/`ExitKind`/`EgressIP` và header tương ứng.

## Bản ghi một lần thử

Mỗi lần thử vật lý (không phải mỗi lượt người dùng) tạo một `CallRecord`; danh
sách "yêu cầu gần đây" trong bảng quản lý hiển thị từng bản ghi, nên một chuỗi
retry là nhiều dòng liên tiếp, không phải một dòng gộp.

| Trường | Ý nghĩa |
|---|---|
| `model`, `at`, `trace` | Mô hình được yêu cầu, thời điểm, id tương quan của request |
| `exit` | Node id của exit đã chọn (rỗng = không có rotator) |
| `exitKind` | `direct`, `warp`, `peer` (selector relay/WARP/sing-box không có rotator), `proxy` (proxy pool tĩnh) hoặc `singbox`; khi peer WARP tự báo danh tính, bản ghi dùng `warp` thay cho `peer` |
| `egressIp` | IP công khai khi rotator định danh exit bằng IP (subscription) hoặc khi peer WARP chọn lease trong transport; rỗng với pool tĩnh vì exit được định danh bằng node id |
| `errCode` | Lớp lỗi đã phân loại (bảng dưới) |
| `httpStatus` | Mã HTTP thật của upstream (0 nếu lỗi transport) |
| `retryAfter` | `Retry-After` đã parse, trước khi bị kẹp theo ramp |
| `upstreamRequestId` | Id hỗ trợ của nhà cung cấp, nếu có |

## Phân loại lỗi

| Dấu hiệu / mã | Hành động |
|---|---|
| `RATE_LIMIT` (429) | Làm nguội mô hình 90 s, gấp đôi tới trần 15 phút; dưới rotation, thử exit khác trước khi tin rằng mô hình đã hết hạn mức |
| `REGION_BLOCKED` (403 "not available in your country") | Verdict của **IP**, không phải mô hình: thử exit khác; không park mô hình |
| `FREE_TIER_BLOCKED` | Cổng client của upstream chặn toàn làn; thử exit vô ích, chuyển sang kênh dự phòng nếu có |
| `TRANSPORT`, `TRANSPORT_RESET`, `TIMEOUT` | Lỗi tunnel/exit; thử exit khác, ghi strike vào node |
| `SERVER`, `EMPTY_RESPONSE` | Upstream đã trả lời nhưng lỗi; có thể thử exit/model khác |
| `EGRESS_REFUSED` ("address of this instance is used up") | Exit đã cạn; xoay peer/exit |
| `OVERLOADED` (529/busy) | Retry ngắn 1–3 s trên cùng mô hình |
| `EXIT_GOVERNOR` | Governor nội bộ từ chối trước khi gửi; trả 429 + `Retry-After`, **không** phải verdict của upstream |
| `CLIENT_ERROR` | Body/request sai; gửi lại y hệt sẽ lặp lại lỗi |
| `DEPLOYMENT_PROTECTED`, `RELAY_UNAUTHORIZED` | Lỗi cấu hình relay, không phải mô hình |

`Retry-After` kiểu "next UTC midnight" (đo được 54.282 s) bị coi là nhiễu: nó
không phải thời điểm hồi phục thật và sẽ park một exit khoẻ cả buổi chiều.
Chỉ hint nằm trong trần ramp mới được dùng, và chỉ khi nó dài hơn bước ramp
hiện tại.

## Làm nguội và loại exit

- **Mô hình (lane):** 90 s → 180 s → 360 s → trần 15 phút. Ba lần 429 trải qua
  ít nhất 20 phút **và** một lần thăm dò độc lập xác nhận mới park dài; một
  lần 429 đơn lẻ không bao giờ đủ.
- **WARP (pool):** cùng cấp số nhân 90 s → 900 s. Hint quá trần bị bỏ. Ba lần
  lỗi trải qua ít nhất `2 × 10 phút` mới bị coi là chặn dai dẳng.
- **Proxy pool tĩnh:** ledger theo cặp `(exit, model)`; 429 chỉ phạt đúng cặp,
  không phạt cả node cho mọi mô hình. Lỗi tunnel được đếm riêng theo node.
- **Subscription (sing-box):** node bị bỏ khỏi vòng chọn sau khi dial thất bại
  hoặc bị đánh dấu chết; health check đưa node trở lại khi dial được.

Điểm quan trọng: hạn mức miễn phí tính theo **IP đầu ra**, không theo mô hình.
Vì vậy một burst qua một exit có thể đốt cửa sổ dùng chung của mọi mô hình trên
IP đó; xoay mô hình không cứu được, xoay exit mới cứu được.

## Governor token theo exit

`exitTokensPerMinute` giữ nhịp token trên từng exit:

- **Mặc định 300.000 token/phút** khi `config.json` không có khóa. Đặt `0` để
  tắt hẳn. Giá trị dương là trần mỗi phút cho một exit.
- Trước khi gửi, lane ước lượng `prompt + max_tokens + 16` (4 ký tự/token, cộng
  overhead mỗi message; ảnh/âm thanh/tệp tính theo kích thước dữ liệu). Đây là
  ước lượng chỉ có thể thu rộng hơn, không thể tiêu ít hơn thực tế.
- Nếu exit ưa thích đã cạn, lane thử tối đa ba exit khác trước khi chờ. Một hội
  thoại bình thường bám một exit để giữ cache tiền tố; nhảy exit là ngoại lệ
  dưới áp lực ngân sách.
- Chờ tối đa **20 giây**. Quá hạn, request nhận `429` với mã
  `EXIT_GOVERNOR` và `Retry-After` (giới hạn 3600 s), thay vì giữ kết nối treo.
  Governor không gọi upstream, nên 429 này không tính là một strike của mô hình
  hay exit.
- Bucket được khoá theo `ExitKey` của rotator: IP với subscription, node id với
  proxy pool tĩnh, `direct` khi không có rotator.

## Bằng chứng đo được

Các con số dưới đây lấy từ log/comment của phiên điều tra ngày 2026-11-11 trên
chính máy này; chúng là lý do các ngưỡng trên tồn tại, không phải giá trị cấu
hình.

| Quan sát | Hệ quả |
|---|---|
| Cùng mô hình `cat-jam-free`: 200 lúc 14:28:57, `REGION_BLOCKED` lúc 14:31:10, rồi 6/6 request 200 ngay sau đó | Region là verdict của exit IP; phải thử exit khác thay vì bỏ mô hình |
| Một exit nhận TCP rồi không trả byte nào: request 217 s kết thúc `TRANSPORT`, cùng mô hình qua exit khác ~15 s | Cần `responseHeaderTimeout` 90 s và retry theo exit |
| `Retry-After` upstream: 54.282 s (nửa đêm UTC) | Kẹp hint theo ramp thay vì park theo header |
| Một đợt 429 kéo dài 13 phút 48 giây (15:54–16:08): 44 lần thử, 34M token đầu vào, 6,7M token trong 10 phút, cùng một exit | Cần governor token theo exit và ramp; retry mù chỉ đốt thêm cửa sổ |
| 5 lượt liên tiếp 140k token trên một exit 300k/phút | Hai lượt đầu đi ngay, các lượt sau chờ tối đa 20 giây rồi trả 429 `EXIT_GOVERNOR`; không lượt nào bị giữ quá cap |
| Pool 8 exit tĩnh, nhiều node có thể ra cùng một IP | Khoá bucket theo IP khi biết, không theo số node |
| Governor mặc định 300k/phút | Một lượt coding lớn mỗi phút mỗi exit; burst không đốt hết cửa sổ trong một phút |

## Kiểm tra nhanh

```bash
# Xem exit thật của một lượt (khi chưa mở stream):
curl -sD - -o /dev/null http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer $OPENKI_KEY" -H 'content-type: application/json' \
  -d '{"model":"glm-5.3-flash-free","messages":[{"role":"user","content":"ping"}],"max_tokens":8}'
# → x-openki-exit, X-OpenKi-Egress, Retry-After (nếu 429)

# Bảng quản lý → yêu cầu gần đây: cột Exit + HTTP; rê chuột lên Exit để xem node id.
```

Nếu `x-openki-exit` trống nhưng `X-OpenKi-Egress` có giá trị, request đi
qua peer transport mà không có rotator lane (ví dụ WARP thuần): `exitKind` sẽ
là `warp` và không có IP lane riêng.
