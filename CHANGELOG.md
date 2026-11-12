# Changelog

Các thay đổi của fork `chipmax/OpenKi` so với upstream. Ngày theo giờ máy
phát triển (UTC+7).

## 2026-11-12

### Repo

- Hoàn tất định danh OpenKi: module `github.com/chipmax/openki`, binary
  `openki`/`openkistats`, biến môi trường `OPENKI_*`, thư mục dữ liệu
  `openki`, service `openki.service` và toàn bộ README/tài liệu. Repo phát
  hành mới: `chipmax/OpenKi`.
- Đổi tên repo GitHub từ repo cũ sang **OpenKi**. URL release, badge, feed cập
  nhật, feed thông báo và dashboard trỏ `chipmax/OpenKi`; cấu hình cũ còn feed
  `chipmax/openki` được migrate ở schema v15, feed người dùng tự đặt vẫn giữ.
- Bỏ `COPYING` (bản sao trùng byte-for-byte của `LICENSE`), hai ảnh chụp không
  được tham chiếu và script patch dashboard một lần; repo gọn hơn, không đổi
  tính năng.
- WSL dùng `networkingMode=mirrored` để Windows dùng chung loopback; giữ một
  phiên WSL ẩn lúc đăng nhập vì localhost forwarding của WSL rớt khi phiên
  rảnh. Bỏ relay/nft/portproxy và shortcut relay cũ.
- Kiểm tra source trên dashboard dùng HTTP client trực tiếp thay vì lane proxy
  (hết timeout khi proxy pool chưa sẵn sàng) và nhận checkout `~/src/OpenKi`.
- Tích hợp **Proton VPN** thành egress peer thứ tư: `egressModes` nhận
  `proton`, `protonSocks5` trỏ listener SOCKS5 cục bộ (mặc định
  `socks5h://127.0.0.1:1080`), có nút kiểm tra và nhãn `X-OpenKi-Egress:
  proton`. OpenKi không giữ tài khoản Proton và không quản lý tunnel.
- README viết lại ngắn gọn, Linux/WSL-first; bỏ phần giới thiệu trùng lặp và
  các tham chiếu thương hiệu không cần thiết. Attribution WARP vẫn nằm trong
  `THIRD_PARTY_NOTICES.md`/`internal/warp/NOTICE` theo GPL-3.0.

### Thương hiệu

- Đổi tên fork `zen-gate-vi` thành **OpenKi** (`openki`): module Go, binary
  `openki`/`openkistats`, biến môi trường `OPENKI_*`, provider `openki` trong
  agent, thư mục dữ liệu `~/.config/openki`, service systemd và toàn bộ
  README/tài liệu. Các marker và provider `zen-gate` cũ vẫn được nhận diện
  để migrate cấu hình đang chạy, nhưng không còn được ghi mới.
- Repo phát hành chuyển sang `chipmax/OpenKi`; feed cập nhật và thông báo cũ
  được migrate tự động, feed người dùng tự trỏ nơi khác được giữ nguyên.
- Logo mới (vòng cung mở + tia sáng) thay cho icon zen-gate ở cửa sổ, tray,
  favicon, dashboard và bộ cài; sinh lại toàn bộ kích thước từ một nguồn SVG.
- Phát hành `v2.0.0` cho Windows x64 và Linux x64/arm64 (headless).

### Giao diện

- Trang "Mạng" có bảng đầu ra hợp nhất: IP hiện tại, node đăng ký và proxy
  tĩnh trong cùng một chỗ, kèm trạng thái, quốc gia, độ trễ, lần phục vụ cuối,
  lọc theo trạng thái/từ khoá và nút dò lại node. Cấu hình proxy thu gọn bên
  dưới để phần trạng thái không bị đẩy xuống.
- Trang "Cập nhật" riêng: so commit đang chạy với `master`, hiển thị checkout
  nguồn, lệnh `git pull`/build/cài/restart cho Linux và nút sao chép. Bản
  Linux không còn quảng cáo "một chạm"; mục Cập nhật trong Cài đặt được gỡ.
- Nút chuyển giao diện sáng/tối nằm ngay trên nút Thu gọn thanh bên; bỏ mục
  trùng lặp trong Cài đặt.
- Ẩn kênh WorkBuddy khỏi bảng kênh, bộ lọc và các nút cấu hình cache của nó.
- Tách cấu hình mạng khỏi trang Cài đặt: proxy thượng nguồn, danh sách proxy
  tĩnh, cổng LAN và luân phiên đăng ký sing-box nằm ở trang "Mạng" riêng, có
  nút lưu proxy độc lập.
- Thẻ "Thông tin hệ thống" luôn hiển thị trên Tổng quan: hệ điều hành/nhân,
  CPU/RAM, tiến trình, cổng nghe và lối ra; gateway trong WSL hiện rõ nền tảng
  WSL thay vì chỉ "Linux".
- Trang Agent chia hai nhóm Windows và WSL/Linux, đếm số đã cài/đang bật và
  dùng chung logo thương hiệu cho các biến thể `-windows` (trước đây rơi về
  biểu tượng mặc định).
- Chỉ dựng lại trang đang xem mỗi 10 giây thay vì toàn bộ dashboard.

### Sửa lỗi

- Feed cập nhật trả 404 (chưa có release) được coi là "không có bản mới" thay
  vì ghi cảnh báo mỗi 6 giờ.
- Trạng thái proxy tĩnh chỉ trả endpoint đã lược bỏ userinfo, kèm thời điểm
  tạm dừng; không lộ mật khẩu proxy ra dashboard.

### Dọn dẹp

- Xoá logo Agent không còn dùng trong bundle và hai icon 48/64 px không được
  tham chiếu.

## 2026-11-11

### Giao diện

- Bỏ trang "Kết nối" khỏi dashboard (mục điều hướng, khung trang và phần
  render). Base URL/Key vẫn nằm trong trạng thái admin cho agent adapter và
  các client tự cấu hình.

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

- Fork phát hành GPL-3.0-only (`LICENSE`); WARP port từ opencode2api
  ghi nguồn tại `internal/warp/NOTICE` và `THIRD_PARTY_NOTICES.md`.
