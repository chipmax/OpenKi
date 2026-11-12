# Ghi chú Việt hoá mã nguồn Go — loimo

Ngày thực hiện: 2026-11-11 · Phạm vi: các file `.go` (không gồm `web/index.html` — phần đó thuộc đợt dịch giao diện web riêng).

## Tổng kết

- Khảo sát ban đầu: **662 chuỗi ký tự chứa chữ Hán** trong 75 file `.go` (không tính test, không tính comment).
- Đã dịch: **~630 chuỗi** hiển thị cho người dùng cuối (thông báo lỗi admin/client API, thông báo hệ thống/khay, nhãn trạng thái, mô tả mô hình & preset nhà cung cấp, trang HTML của `loimostats`, trang callback đăng nhập...). Tổng cộng 660 dòng `.go` thay đổi (gồm cả cập nhật test).
- Còn lại 31 chuỗi thuộc danh sách **cố ý không dịch** ở mục 3.
- Kiểm chứng: `CGO_ENABLED=0 go build -tags headless ./...` ✅ pass. `go test -tags headless ./...`: mọi package pass, trừ 5 test LAN trong `internal/gateway` đã fail sẵn ở baseline trước khi dịch (môi trường VM chặn kết nối tới IP LAN) — xem mục 4.

## 1. Số chuỗi/dòng đã dịch theo file (file .go, tính theo dòng thay đổi)

| Khu vực | File | Dòng |
|---|---|---|
| Điểm vào | `cmd/loimo/main.go` | 46 |
| | `cmd/loimostats/main.go` (HTML tổng quan + nhãn JS) | 19 |
| Agent | `internal/agents/tier2.go` | 22 |
| | `internal/agents/codex.go` | 10 |
| | `internal/agents/dsh.go` | 10 |
| | `internal/agents/zcode.go` | 8 |
| | `internal/agents/opencode.go` | 7 |
| | `internal/agents/qoder.go` | 6 |
| | `internal/agents/claude.go` | 5 |
| | `internal/agents/registry.go` | 5 |
| | `internal/agents/workbuddy.go` | 5 |
| | `internal/agents/autostart_linux.go` (Comment= của .desktop) | 1 |
| Gateway | `internal/gateway/admin.go` | 49 |
| | `internal/gateway/qoder.go` | 15 |
| | `internal/gateway/channel_wall.go` | 14 |
| | `internal/gateway/server.go` | 12 |
| | `internal/gateway/workbuddy.go` | 12 |
| | `internal/gateway/relay.go` | 11 |
| | `internal/gateway/effort_rungs.go` (nhãn mức suy luận) | 8 |
| | `internal/gateway/capsprobe.go` | 4 |
| | `internal/gateway/codearts.go` | 4 |
| | `internal/gateway/openref.go` | 4 |
| | `internal/gateway/ext_channels.go` | 3 |
| | `internal/gateway/anthropic.go` + `openai.go` + `routing.go` + `probetask.go` + `zcode_trae.go` | 2 mỗi file |
| Lane | `internal/lane/workbuddy_account.go` | 33 |
| | `internal/lane/ext.go` (mô tả kênh + lỗi) | 26 |
| | `internal/lane/catalog.go` (mô tả 19 mô hình) | 19 |
| | `internal/lane/trae_login.go` | 13 |
| | `internal/lane/codearts_account.go` | 12 |
| | `internal/lane/workbuddy.go` | 12 |
| | `internal/lane/zcode.go` | 11 |
| | `internal/lane/trae_pool.go` / `workbuddy_pool.go` / `zcode_pool.go` | 8 mỗi file |
| | `internal/lane/zcode_post.go` | 5 |
| | `internal/lane/client.go` (nhãn trạng thái thăm dò) | 4 |
| | `internal/lane/codearts.go` / `events.go` / `trae.go` / `effort.go` | 3 mỗi file |
| | `internal/lane/kilo.go` / `probe.go` | 1 mỗi file |
| Relay | `internal/relay/presets.go` (23 preset: giữ tên thương hiệu, dịch mô tả) | 23 |
| | `internal/relay/relay.go` | 10 |
| QoderHub | `internal/qoderhub/remote/oauth.go` | 11 |
| | `internal/qoderhub/httpapi/pool.go` / `remote/client.go` | 8 mỗi file |
| | `internal/qoderhub/remote/pool.go` | 6 |
| | `internal/qoderhub/remote/credentials.go` | 5 |
| Subs | `internal/subs/manager.go` | 20 |
| | `internal/subs/parse.go` | 18 |
| | `internal/subs/procattr_windows.go` | 8 |
| | `internal/subs/singbox.go` | 6 |
| | còn lại (`config/lifecycle/pick/procattr_other/rotate/util/ledger`) | 1–3 mỗi file |
| Khác | `internal/codearts/upstream/checkin.go` (nhãn trạng thái hoạt động tự sinh) | 11 |
| | `internal/openref/modelsdev.go` + `openref.go` | 4 + 3 |
| | `internal/tray/tray.go` (menu khay) | 5 |
| | `internal/window/window_windows.go` | 10 |
| | `internal/window/window_linux.go` | 1 |
| | `internal/announce/announce.go` | 2 |
| | `internal/autotag/autotag.go` | 3 |
| | `internal/store/store.go` (mặt nạ `‹đã ẩn›` khi xuất cấu hình) | 1 |
| | `internal/notify/*` | 0 — package này không chứa chuỗi Hán; tiêu đề/nội dung thông báo do caller (main.go) truyền vào, đã dịch ở caller |

## 2. Sửa logic đi kèm (không phải dịch)

Ba chỗ phải chạm nhẹ vào code để bản dịch nhất quán/đúng:

1. `internal/lane/workbuddy.go` — hậu tố lỗi tự sinh đổi sang ngoặc ASCII đồng bộ: `(upstream code N)` (bản gốc dùng ngoặc toàn chiều rộng `（上游 code N）`).
2. `internal/qoderhub/remote/credentials.go` — dấu nối các phần thông báo lỗi `；` (toàn chiều rộng) đổi thành `; `.
3. Khôi phục nguyên trạng một mẫu khớp bị dịch nhầm trong lúc rà soát (xem mục 3, `isQoderQueueBody`) — test `TestQueueBodyDetection` bắt được và đã sửa lại trước khi bàn giao.

## 3. Danh sách chuỗi CỐ Ý KHÔNG dịch (31 chuỗi) và lý do

| Chuỗi / vị trí | Lý do |
|---|---|
| `internal/codearts/upstream/client.go`: `"并发会话"` trong `strings.Contains` | Mẫu nhận dạng văn bản lỗi của upstream CodeArts để phân loại giới hạn — dịch là gãy phân loại |
| `internal/codearts/upstream/models.go`: `"智能体"` so với trường `alias_zh_cn` của API upstream | Giá trị upstream trả về, dùng để lọc đúng agent "CodeAgent" — là dữ liệu khớp, không phải hiển thị |
| `internal/lane/ext.go`: `"积分不足"`, `"额度已用完"`, `"余额不足"` trong danh sách marks của `extClassify` | Mẫu khớp thông điệp hết quota do các upstream Trung Quốc phát ra |
| `internal/lane/workbuddy.go`: regex `(?:reset at|将在)\s*(...)`; regex `(?i)...|请求过于频繁`; `strings.Contains(msg, "窗口")` | Regex/mẫu khớp văn bản upstream (giờ reset, "yêu cầu quá thường xuyên", chữ "cửa sổ" trong msg upstream) |
| `internal/lane/workbuddy_account.go`: `"已签到"` ×2 trong `strings.Contains(resp.Msg, ...)` | Khớp thông điệp "đã điểm danh" do upstream WorkBuddy trả về |
| `internal/qoderhub/remote/client.go`: `"模型负载较高"` trong `isQoderQueueBody` | Mẫu khớp thân lỗi upstream Qoder để nhận diện trạng thái xếp hàng (đã từng dịch nhầm, test bắt được, đã khôi phục) |
| `internal/qoderhub/service/service.go`: 15 chuỗi — 3 prompt thị giác (`请只根据图片内容回答…`, `请描述这张图片的主要内容。`) và danh sách cue (`让我/我来/我将/接下来/继续/查看/检查/搜索/读取/运行/执行/工具`) | Prompt gửi cho mô hình Lingma/Qoder (đổi ngôn ngữ = đổi hành vi mô hình) + danh sách từ khoá dò tìm mẫu câu "sắp gọi công cụ" trong đầu ra tiếng Trung của mô hình |
| `internal/store/store.go`: hằng số `TagSource*` = `实测 / 上游声明 / AI标注 / 规则 / 未标注` (5 giá trị) | Đây là **giá trị enum được lưu xuống `tags.json`** và được dashboard so khớp chính xác để vẽ huy hiệu `capability_source` (web/index.html chứa đúng các chuỗi này). Đổi giá trị = dữ liệu cũ mất nghĩa + huy hiệu gãy. Nếu muốn Việt hoá, phải đổi đồng bộ cả web UI + migration dữ liệu cũ — để đợt riêng |

Ngoài ra, theo quy tắc chung: **toàn bộ comment trong code giữ nguyên tiếng Trung** (không thuộc bề mặt người dùng), và các chuỗi Hán xuất hiện trong fixture của test (ví dụ `logs_api_test.go` dùng `免费车道耗尽` làm dữ liệu tìm kiếm mẫu) cũng giữ nguyên vì chúng là dữ liệu kiểm thử, không phải chuỗi sản phẩm phát ra.

## 4. Test

**Baseline (trước khi dịch)** — `CGO_ENABLED=0 go test -tags headless ./...`: mọi package pass trừ 5 test trong `internal/gateway`: `TestLanServedWhenAllowed`, `TestAdminFenceHoldsOverLan`, `TestRebindAppliesLanToggle`, `TestSettingsAllowLanRebinds`, `TestSettingsAllowLanRepliesOverLiveListener`. Nguyên nhân môi trường: các test này gọi tới IP LAN của VM (198.19.0.2) và bị lớp proxy của runtime chặn (lỗi "malformed HTTP status code Other"), không liên quan mã nguồn. (Chạy `go test ./...` không có thẻ `headless` trên VM Linux này còn fail build sẵn ở `cmd/loimo`, `internal/tray`, `internal/window`, `internal/gateway` do thiếu thư viện hệ thống ayatana-appindicator cho systray — cũng là vấn đề môi trường có từ trước.)

**Sau khi dịch**: 12 test fail vì assert đúng chuỗi hiển thị tiếng Trung cũ — tất cả đã được cập nhật sang chuỗi tiếng Việt mới:

| Test | File | Nội dung cập nhật |
|---|---|---|
| `TestWorkBuddyRefusalReadsAsASentence` | `internal/lane/workbuddy_test.go` | Hậu tố `(upstream code 14003)` + tiền tố "Cửa sổ miễn phí đã dùng hết" |
| `TestRoutingLineIsClassified` | `internal/gateway/access_log_test.go` | Dòng log định tuyến "Làn miễn phí đã cạn…" |
| `TestCapabilityProbeReportsEachModality` | `internal/gateway/capsprobe_events_test.go` | Tên bước: Hình ảnh/Âm thanh/Tệp |
| `TestDashboardProxyLabelCoversEveryMode` | `internal/gateway/dashboard_settings_test.go` | Nhãn proxy của dashboard đã Việt hoá; phép kiểm "chứa chữ Hán" đổi thành so khớp đúng nhãn tiếng Việt |
| `TestNewModelNamesAreHuman` | `internal/lane/roster_refresh_test.go` | Blurb phải chứa "đo thực tế" |
| `TestWBContentGateMarkMatchesTheEmittedMessage` | `internal/lane/wb_gate_cool_test.go` | Mark "Cổng nội dung" |
| `TestWorkBuddyRefusalReadsAsASentence` | `internal/lane/workbuddy_test.go` | Hậu tố `(upstream code 14003)` + tiền tố "Cửa sổ miễn phí đã dùng hết" |
| `TestPoolDeviceLoginRejectsUnknownRealm` | `internal/qoderhub/httpapi/pool_login_test.go` | Lỗi phải chứa "không xác định" |
| `TestPoolImportWithoutIDELoginExplainsItself` | `internal/qoderhub/httpapi/pool_test.go` | Lỗi phải chứa "đăng nhập" |
| `TestPoolAccountActionsValidateInput` | `internal/qoderhub/httpapi/pool_test.go` | "Thao tác không xác định" |
| `TestQuotaErrorMessageNamesTheKind` | `internal/qoderhub/remote/client_classify_test.go` | "đổi tài khoản không ích gì" / "đã cạn" |
| `TestQueueBodyDetection` | — | **Không sửa test** — đây là lỗi dịch thật (mẫu khớp upstream), đã khôi phục chuỗi gốc trong code |

**Kết quả cuối**: `go test -tags headless ./...` — 13 package `ok`; `internal/gateway` chỉ còn đúng 5 test LAN fail giống hệt baseline. Không có test nào fail vì lý do ngoài dự kiến.

## 5. Lưu ý kỹ thuật

- Nhiều file gốc dùng CRLF; quá trình dịch đã giữ nguyên CRLF cho các file vốn có CRLF để diff sạch (tổng diff Go ≈ 660 dòng, không có file nào bị viết lại toàn bộ vì line ending).
- Tên thương hiệu/dịch vụ giữ nguyên: WorkBuddy, CodeArts, Qoder, TRAE, ZCode (chỉ bỏ chú thích Hán trong ngoặc, ví dụ `ZCode (Zhipu)`, `TRAE (ByteDance)`, `Loomy (iFlytek)`); các preset trong `internal/relay/presets.go` tương tự (`SiliconFlow`, `ModelScope`, `BigModel (Zhipu)`...). Cụm "国内直连" trong mô tả preset được dịch thành "kết nối thẳng trong nước (Trung Quốc)" để không gây hiểu nhầm cho người dùng Việt.
