# loimo — Fork tiếng Việt của Lối Mở

> **Fork tiếng Việt — loimo**
>
> Đây là bản fork đã Việt hoá từ [LAGcomcom/zen-gate](https://github.com/LAGcomcom/zen-gate) (bản gốc tiếng Trung được giữ nguyên ở [README.zh-CN.md](README.zh-CN.md)):
>
> - Toàn bộ giao diện quản lý (dashboard), thông báo hệ thống, menu khay, thông báo lỗi và mô tả mô hình/nhà cung cấp trong mã nguồn Go đã được dịch sang tiếng Việt. Chi tiết phạm vi dịch và các chuỗi cố ý giữ nguyên (mẫu nhận dạng lỗi của upstream, giá trị enum dùng để so khớp) xem [GO-L10N-NOTES.md](GO-L10N-NOTES.md).
> - **WARP (từ opencode2api): đã tích hợp.** Pool WARP userspace của [opencode2api](https://github.com/jasonxu114514/opencode2api) đã được đưa vào lane egress của fork: mỗi request thuê một exit riêng, exit dính 429/403 bị phạt (kèm cooldown /64), lease giữ nguyên trong suốt stream, và khi WARP không khả dụng gateway tự fallback về đường truyền thường. Bật bằng `warpEnabled: true` trong `config.json` (xem thêm `WARP-INTEGRATION.md`). Lưu ý: phần mã WARP thuộc GPL-3.0; toàn bộ fork này cũng phát hành theo GPL-3.0 (xem [Giấy phép](#giấy-phép)).

<div align="center">

<img src="assets/icon-256.png" width="88" alt="Lối Mở">

# LỐI—MỞ

**Đưa các mô hình miễn phí của OpenCode Zen vào mọi AI Agent của bạn.**

Một chương trình khay hệ thống trên máy tính (Windows 10/11, macOS 13+, Linux): mở một cổng (gateway) cục bộ tương thích OpenAI / Anthropic và tự động kết nối với các AI Agent đã cài trên máy — các mô hình miễn phí xuất hiện ngay trong bộ chọn mô hình của Agent.

[![release](https://img.shields.io/github/v/release/chipmax/loimo?style=flat-square&label=phiên%20bản)](https://github.com/chipmax/loimo/releases/latest)
[![go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![platform](https://img.shields.io/badge/Windows-10%2F11%20%7C%20macOS%2013%2B%20%7C%20Linux-0078D6?style=flat-square)](https://github.com/chipmax/loimo/releases)
[![license](https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square)](LICENSE)

</div>

---

## Đây là gì

OpenCode Zen cung cấp một loạt mô hình miễn phí **không cần đăng nhập, không tính phí**, nhưng chúng không có cơ chế API Key tiêu chuẩn nên các client thông thường không kết nối được. Lối Mở đóng gói "làn" này trên máy của bạn thành **giao diện tương thích OpenAI / Anthropic tiêu chuẩn**, và tự động hoàn tất việc tiêm cấu hình cho mọi Agent.

- **Tự động thích ứng toàn diện**: phát hiện OpenCode / Codex / Claude Code / DeepSeek Harness / omp (bản Windows và bản WSL là hai mục riêng) là tiêm cấu hình ngay (sao lưu trước, tắt là khôi phục). Khởi động lại Agent tương ứng là thấy mô hình miễn phí trong bộ chọn mô hình.
- **Thêm API tuỳ chỉnh**: ngoài làn miễn phí, có thể thêm bất kỳ upstream tương thích OpenAI / Anthropic nào — tích hợp sẵn 14 preset nhà cung cấp miễn phí (NVIDIA NIM, Google Gemini, GitHub Models, Groq, Cerebras, Mistral, OpenRouter, HuggingFace, SiliconFlow, ModelScope, BigModel...) để điền sẵn, bạn chỉ cần dán Key của mình; mô hình xuất hiện trong mọi Agent dưới dạng `nhà cung cấp/mô hình`.
- **Pool miễn phí Kilo**: nguồn miễn phí thứ hai không cần thông tin xác thực, gộp chung vào danh mục, dùng chung cơ chế định tuyến, chuyển đổi khi bị giới hạn và bảng thăm dò với làn miễn phí.
- **Lớp giao thức được kiểm chứng thực tế**: đúc phiên, cổng dấu vân tay, ba giao thức đường dây (chat / responses / messages), ba lớp dự phòng khi đứt luồng/trống/kết thúc sớm — tất cả đều được đo theo hành vi client thật.
- **Thăm dò từng mô hình**: mỗi mô hình có thể được thăm dò riêng về độ khả dụng và độ trễ ký tự đầu; lịch sử thăm dò được lưu và xem lại được sau khi khởi động lại.

## Tính năng nổi bật

| | |
|---|---|
| 🧭 **Định tuyến đa phương thức thông minh** | Khi yêu cầu có hình ảnh / âm thanh / tệp, chuyển đổi dự phòng chỉ rơi vào mô hình hỗ trợ đúng phương thức đó; năng lực mô hình được gắn nhãn ba lớp (bảng tuyển chọn tích hợp → AI gắn nhãn → đo thực tế) và hiển thị huy hiệu ngay trên trang mô hình |
| 🎯 **Chiến lược định tuyến** | Công tắc định tuyến thông minh bật/tắt bất cứ lúc nào; chuỗi ứng viên dự phòng sắp xếp theo "thứ tự danh mục / độ trễ ký tự đầu"; khi làn miễn phí cạn hoàn toàn có thể lùi một chiều về API tuỳ chỉnh của bạn (mặc định tắt, vì sẽ tốn Key của bạn) |
| 🔄 **Tự động chuyển đổi khi bị giới hạn** | Mô hình bị giới hạn tốc độ sẽ tự chuyển sang mô hình khả dụng tiếp theo, header phản hồi ghi rõ mô hình thực tế đã trả lời |
| 🧯 **Giới hạn theo exit, không theo mô hình** | Chuỗi 429 bắt đầu 90 giây, gấp đôi đến trần 15 phút; chỉ park dài khi ba lần 429 cách xa **và** một lần thăm dò độc lập xác nhận. Governor `exitTokensPerMinute` (mặc định 300k/phút mỗi exit, `0` = tắt) giữ nhịp token, thử exit khác khi cạn và chỉ chờ tối đa 20 giây trước khi trả 429 kèm `Retry-After` — chi tiết ở [docs/EGRESS-POLICY.md](docs/EGRESS-POLICY.md) |
| 🫆 **Cổng fingerprint có công tắc và A/B** | `fingerprintMode` (`auto`/`off`/`force`) và `gateprobe` (`-n`, `-via direct\|pool\|warp`) kiểm tra upstream còn bắt buộc định nghĩa `read`/`shell` hay không; mỗi lần chạy in hash của profile, header `x-loimo-exit` cho biết đúng exit đã phục vụ — xem [docs/GATE-AB.md](docs/GATE-AB.md) |
| 📢 **Hệ thống thông báo** | Sửa `announcements.json` ở thư mục gốc là phát thông báo tới mọi người dùng (phân cấp info/warn/critical, khung giờ hiệu lực, theo dõi đã đọc); trang tổng quan đồng bộ mỗi 6 giờ |
| 🔌 **API tuỳ chỉnh** | 14 preset nhà cung cấp miễn phí điền sẵn một chạm + tự truy vấn danh sách mô hình; Key của bạn do bạn điền, mức dùng vẫn ghi sổ đầy đủ |
| 📖 **Siêu dữ liệu năng lực trong giao diện chuẩn** | `GET /v1/models` kèm theo mỗi mô hình các trường `context_window`, `max_output_tokens`, `input_modalities`, `reasoning`, `supported_reasoning_levels`, `capability_source` — kết luận đo thực tế đọc được từ Agent/SDK/script bất kỳ mà không cần mở bảng quản lý |
| 🌐 **Xoay vòng đăng ký proxy** | Dán đăng ký proxy (vless / vmess / ss / trojan / hy2 / tuic), sidecar sing-box tích hợp chuyển thành pool socks cục bộ, **mỗi yêu cầu xoay một đầu ra khoẻ** — hạn mức làn miễn phí tính theo IP đầu ra nên nhiều nút là nhiều phần hạn mức; có thăm dò sức khoẻ nút và tự bỏ qua nút quay số thất bại |
| 🧭 **Xoay vòng danh sách proxy tĩnh** | Cơ chế `proxies` của opencode2api, không cần sing-box: khai báo `proxyPool` (http(s)://, socks5(h)://, `direct`) rồi chọn `proxyMode: "pool"`. Mỗi hội thoại bám một đầu ra để giữ cache tiền tố; đầu ra bị 429/403 chỉ bị phạt cho đúng cặp (đầu ra, mô hình) và yêu cầu tự thử đầu ra khác. Sửa danh sách trong Cài đặt hoặc `config.json`, áp dụng nóng |
| 💾 **Bộ nhớ đệm tiền tố (prefix cache)** | Thống kê tỉ lệ trúng cache hiển thị ở trang mức dùng, danh sách yêu cầu gần đây và xuất CSV |
| 📊 **Ước tính hạn mức** | Không có API số dư chính thức vẫn ước tính được: theo dõi thời điểm bị giới hạn + dự đoán thời gian khôi phục + thanh tiến độ hạn mức ngày |
| 🌡 **Bản đồ nhiệt kiểu GitHub** | Bản đồ nhiệt mức dùng 365 ngày + biểu đồ xu hướng nhiều mô hình, xem theo ngày / tuần / tích luỹ |
| 🖥 **Thường trú ở khay hệ thống** | Đóng cửa sổ là thu vào khay/menubar, tự khởi động cùng hệ thống, thông báo hệ thống, đi theo proxy của hệ thống |
| 🛡 **Chỉ nghe trên máy cục bộ** | Cổng mặc định chỉ gắn 127.0.0.1; trong cài đặt có thể mở cho LAN dùng chung (API vẫn bắt buộc có API Key, trang quản trị luôn chỉ chấp nhận máy cục bộ), sửa cấu hình luôn sao lưu trước |

## Bắt đầu nhanh

**Windows** — Tải `loimo.exe` từ trang [Releases của bản fork](https://github.com/chipmax/loimo/releases/latest) và chạy trực tiếp (biểu tượng xuất hiện ở khay). Nếu máy chưa có WebView2 Runtime, chương trình tự chuyển sang mở trang quản lý bằng trình duyệt mặc định, chức năng cổng không bị ảnh hưởng. Biến môi trường `LOIMO_NO_WEBVIEW2=1` ép dùng chế độ trình duyệt (để chẩn đoán lỗi).

**macOS** — `tools/build-macos.sh` tạo ra `dist/Lối Mở.app`:

```bash
tools/build-macos.sh
cp -R "dist/Lối Mở.app" /Applications/
open "/Applications/Lối Mở.app"
```

**Linux** — `tools/build-linux.sh` tạo AppImage tự chứa `dist/Loi_Mo-x86_64.AppImage` (GTK3 + appindicator đóng gói sẵn, không cần cài phụ thuộc):

```bash
tools/build-linux.sh 1.2.1
chmod +x dist/Loi_Mo-x86_64.AppImage
./dist/Loi_Mo-x86_64.AppImage
```

Bản Linux không có cửa sổ nhúng: trang quản lý mở trong trình duyệt mặc định, chương trình thường trú ở khay.

**Debian / Ubuntu / Deepin** — `tools/build-deb.sh` tạo gói `dist/loimo_*.deb` (phụ thuộc `libgtk-3-0` và `libayatana-appindicator3-1` của hệ thống, dung lượng nhỏ hơn):

```bash
tools/build-deb.sh 1.2.1
sudo apt install ./dist/loimo_1.2.1-linux_amd64.deb
loimo
```

Ở mọi nền tảng, cách dùng đều như nhau: vào trang "Thích ứng Agent", bật công tắc của Agent bạn đã cài → khởi động lại Agent đó → mô hình miễn phí xuất hiện trong bộ chọn mô hình.

### Windows và WSL: hai bản tách riêng

| | Windows | WSL |
|---|---|---|
| Giao diện | GUI + khay hệ thống | headless, mở bằng trình duyệt |
| Dữ liệu | `%APPDATA%\loimo` | `$XDG_CONFIG_HOME/loimo` (mặc định `~/.config/loimo`) |
| Agent | cấu hình nằm trong Windows | cấu hình nằm trong Linux; thêm mục **omp (Windows)** khi tìm thấy `C:\Users\<tên>\.omp\agent\models.yml` qua `/mnt/c` |
| Cổng | 127.0.0.1:8787 | 127.0.0.1:8787 |

Chạy một bản tại một thời điểm trên cùng một cổng; hai bản không chia sẻ dữ liệu hay danh sách Agent.

#### Cài trong WSL (Debian/Ubuntu, systemd)

```bash
git clone https://github.com/chipmax/loimo.git
cd loimo
CGO_ENABLED=0 go build -tags headless -trimpath -o loimo ./cmd/loimo
sudo install -m755 loimo /usr/local/bin/loimo

sudo tee /etc/systemd/system/loimo.service >/dev/null <<'EOF'
[Unit]
Description=Lối Mở (headless)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=YOUR_USER
Environment=LOIMO_HEADLESS=1
ExecStart=/usr/local/bin/loimo
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

sudo sed -i "s/YOUR_USER/$USER/" /etc/systemd/system/loimo.service
sudo systemctl daemon-reload
sudo systemctl enable --now loimo
```

Mở `http://127.0.0.1:8787` từ trình duyệt trên Windows (WSL2 chuyển tiếp localhost). Log: `journalctl -u loimo -f`.

## Build từ mã nguồn

Windows:

```bash
go build -trimpath -ldflags "-H=windowsgui -X github.com/chipmax/loimo/internal/gateway.Version=1.2.1 -X github.com/chipmax/loimo/internal/update.Current=1.2.1" -o dist/loimo.exe ./cmd/loimo
```

macOS (cần cgo và công cụ dòng lệnh Xcode):

```bash
tools/build-macos.sh 1.2.1
```

Linux (cần cgo, gcc, `libgtk-3-dev` và `libayatana-appindicator3-dev`):

```bash
tools/build-linux.sh 1.2.1
```

## Docker và triển khai headless (không giao diện)

Mã nguồn có thẻ build `headless`: cửa sổ và khay hệ thống được biên dịch loại bỏ hoàn toàn, binary không còn liên kết GTK / WebView2 / appindicator, nên `CGO_ENABLED=0` cũng biên dịch chéo được và chạy thẳng trong container / NAS:

```bash
docker compose up -d        # hoặc docker build -t loimo .
```

- Cổng `8787`, dữ liệu nằm ở volume `/data` (`LOIMO_HOME`); `LOIMO_HEADLESS=1` tương đương cờ `-no-tray`
- Image có sẵn `HEALTHCHECK` gọi `/healthz`
- Trang quản lý vẫn **chỉ** nghe ở loopback: trong container hãy dùng tunnel SSH hoặc reverse proxy cùng máy để truy cập; cổng được map ra ngoài là tầng endpoint OpenAI / Anthropic `/v1`, không phải trang quản trị

Không dùng Docker, chỉ cần một binary không giao diện:

```bash
CGO_ENABLED=0 go build -tags headless -trimpath -o loimo ./cmd/loimo
```

Ma trận triển khai Windows (GUI/tray), WSL (systemd, cổng 8788) và VPS (Caddy
TLS, loopback 8787) cùng quy tắc `agentScope`/`egressModes`/`relayPool`: xem
[docs/DEPLOY.md](docs/DEPLOY.md). Giao thức và triển khai relay:
[docs/RELAY.md](docs/RELAY.md). Chính sách egress/exit, cooldown, governor và
cách đọc bản ghi: [docs/EGRESS-POLICY.md](docs/EGRESS-POLICY.md); A/B cổng
fingerprint và transport: [docs/GATE-AB.md](docs/GATE-AB.md).

## Cập nhật một chạm

Ứng dụng kiểm tra Releases của kho fork (`chipmax/loimo`) mỗi 6 giờ. Khi có phiên bản mới, trang tổng quan hiện nút "Cập nhật một chạm" — tự tải, thay thế và khởi động lại trong khoảng 10 giây. Tính năng này chỉ bật trên Windows (nơi bản phát hành là một file `.exe` đổi tên được khi đang chạy).

Lịch sử thay đổi của fork: [CHANGELOG.md](CHANGELOG.md).

## Giấy phép

**GNU GPL-3.0-only** — SPDX: `GPL-3.0-only`. Toàn văn tại [LICENSE](LICENSE) và bản sao [COPYING](COPYING).

- Mã nguồn gốc: [LAGcomcom/zen-gate](https://github.com/LAGcomcom/zen-gate) — Copyright (C) 2026 LAGcomcom.
- Bản fork tiếng Việt: Copyright (C) 2026 chipmax.
- Khi phân phối lại (kể cả binary), phải kèm giấy phép, giữ nguyên thông báo bản quyền và cung cấp mã nguồn tương ứng theo GPL-3.0.
- Các thành phần bên thứ ba và giấy phép tương ứng: xem [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Phần WARP có nguồn từ opencode2api (GPL-3.0), chi tiết tại `internal/warp/NOTICE` và `WARP-INTEGRATION.md`.
