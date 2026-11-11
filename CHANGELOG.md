# Changelog

Các thay đổi của fork `chipmax/zen-gate-vi` so với upstream. Ngày theo giờ máy
phát triển (UTC+7).

## 2026-11-11

### Egress

- Ramp làm nguội exit thay vì tin `Retry-After`: 90 s → 180 s → 360 s → trần
  15 phút. Hint dài hơn trần (đo được 54.282 s, "nửa đêm UTC") bị coi là nhiễu.
- Chỉ park dài khi có ba lần 429 trải qua ít nhất hai cửa sổ cách nhau 10 phút
  (WARP pool); làn mô hình còn đòi thêm một lần dò độc lập xác nhận. Ba lần
  trong hai phút vẫn chỉ là ramp 6 phút.
- Governor token theo exit (mặc định 300.000 token/phút, `0` để tắt). Mỗi lượt
  chờ tối đa 20 giây; quá hạn trả `429` + `Retry-After` mã `EXIT_GOVERNOR`,
  không giữ kết nối và không tính strike cho model/exit.
- Ghi log số token và thời gian chờ khi governor phải hoãn hoặc từ chối lượt.

### Quan sát

- Mỗi lần thử upstream là một `CallRecord` riêng, có `exit`, `exitKind`,
  `egressIp`, `httpStatus`, `retryAfter`, `upstreamRequestId`.
- Peer WARP chọn lease bên trong transport; địa chỉ công khai của lease giờ
  được đưa ngược vào bản ghi (trước đây chỉ có với rotator subscription).
- Equal-peer selector xoá danh tính của peer trước khi thử peer kế tiếp, nên
  một WARP thất bại không để lại IP cũ trên bản ghi do relay phục vụ.
- Bảng "Yêu cầu gần đây" hiển thị `exitKind · egressIp`; cột Exit giữ node id
  trong tooltip.

### Phân loại lỗi

- Tách `DEPLOYMENT_PROTECTED`, `RELAY_UNAUTHORIZED`, `EGRESS_REFUSED`,
  `OVERLOADED` khỏi quota/model: chỉ lỗi cạn exit mới đáng thử exit khác.
- 429 `FreeUsageLimitError` (ba shape body), 529 busy, và hai lỗi relay được
  khoá bằng bảng test sáu body.

### Fingerprint

- Hash của profile đang gửi (UA + mô tả/schema `read`,`shell`) hiển thị trong
  Cài đặt cạnh `fingerprintMode`.
- Lần free-tier gate từ chối gần nhất (`freeTierGateAt`) được ghi nhận và hiển
  thị cảnh báo, kèm gợi ý chạy `gateprobe` và so hash; log lifecycle vẫn giới
  hạn một dòng/giờ.

### Test và tài liệu

- Mô phỏng 44 lần 429 trên 18 exit: mọi cooldown nằm trong (0, 15 phút].
- Test 5 lượt 140k token trên governor 300k/phút: không lượt nào chờ quá cap.
- Test lease WARP trả đúng IP công khai và test record xoá danh tính khi peer
  fallback, để một WARP thất bại không gắn IP cũ lên bản ghi của relay.
- Test `FreeTierGateAt` ghi nhận lần từ chối mới nhất.
- `EGRESS-POLICY.md` bổ sung số đo đợt 429 13 phút 48 giây (44 lần thử, 34M
  input, 6,7M trong 10 phút) và kết quả burst 5×140k.

### Giấy phép

- Fork phát hành GPL-3.0-only (`LICENSE`, `COPYING`); WARP port từ opencode2api
  ghi nguồn tại `internal/warp/NOTICE` và `THIRD_PARTY_NOTICES.md`.
