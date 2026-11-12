# OpenKi

Cổng mô hình miễn phí OpenCode Zen, bản fork tiếng Việt của
[LAGcomcom/zen-gate](https://github.com/LAGcomcom/zen-gate). OpenKi chạy như một
gateway cục bộ tương thích OpenAI/Anthropic, tự inject cấu hình vào các AI Agent
đã cài trên máy. **Linux/WSL là nền tảng chính**; Windows và macOS dùng chung
mã nguồn.

[![release](https://img.shields.io/github/v/release/chipmax/OpenKi?style=flat-square&label=phiên%20bản)](https://github.com/chipmax/OpenKi/releases/latest)
[![go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![license](https://img.shields.io/badge/license-GPL--3.0-blue?style=flat-square)](LICENSE)

## Tính năng

| | |
|---|---|
| **Làn miễn phí** | Đóng gói OpenCode Zen thành `/v1/chat/completions`, `/v1/responses`, `/v1/messages`; tự chuyển mô hình khi bị giới hạn. |
| **Tự inject Agent** | OpenCode, Codex, Claude Code, DeepSeek Harness, omp (Windows và WSL là hai mục riêng); sao lưu trước khi sửa, tắt là khôi phục. |
| **Egress nhiều peer** | `relay`, `singbox`, `warp`, `proton` là các peer ngang hàng; mỗi request chọn một peer, lỗi transport hoặc 429/403 tự chuyển peer kế tiếp. |
| **Proton VPN** | Dùng listener SOCKS5 do client Proton/pVPN mở sẵn; OpenKi không giữ mật khẩu Proton và không tự quản lý tunnel. |
| **Pool tĩnh / xoay đăng ký** | `proxyPool` cho http(s)/socks5(h)/direct; hoặc sidecar sing-box biến subscription thành pool socks và xoay mỗi request. |
| **Hạn mức theo exit** | 429 bắt đầu 90 giây, tăng dần đến 15 phút; governor token mỗi exit (mặc định 300k/phút, `0` = tắt). |
| **Cache & thống kê** | Prefix cache, ước tính quota ngày, heatmap 365 ngày, xuất CSV. |
| **LAN có kiểm soát** | API có thể mở cho LAN nhưng luôn cần API key; dashboard/admin chỉ chấp nhận loopback. |
| **Cập nhật** | Kiểm tra GitHub Releases; tự thay binary trên Windows, Linux hiển thị lệnh build/cài. |

## Cài đặt

### WSL / Debian / Ubuntu (khuyến nghị)

Yêu cầu: Go ≥ 1.26, systemd trong WSL, user có quyền `sudo`.

```bash
git clone https://github.com/chipmax/OpenKi.git
cd OpenKi
CGO_ENABLED=0 go build -tags headless -trimpath -o openki ./cmd/openki
sudo install -m755 openki /usr/local/bin/openki
sudo install -m644 deploy/wsl/openki.service /etc/systemd/system/openki.service
sudo sed -i "s/User=admin/User=$USER/; s/Group=admin/Group=$USER/; s#/home/admin#$HOME#g" /etc/systemd/system/openki.service
sudo systemctl daemon-reload
sudo systemctl enable --now openki
```

Mở `http://127.0.0.1:8787` từ Windows (WSL2 mirrored networking) hoặc từ Linux.
Log: `journalctl -u openki -f`. Dữ liệu: `$OPENKI_HOME` (mặc định
`~/.config/openki`).

### Linux desktop

```bash
tools/build-linux.sh 2.0.0       # AppImage
tools/build-deb.sh 2.0.0         # Debian/Ubuntu .deb
```

Bản desktop có tray; bản headless không cần GTK/WebView2.

### Windows / macOS

Tải release tại [chipmax/OpenKi/releases](https://github.com/chipmax/OpenKi/releases/latest)
hoặc build theo `deploy/windows/README.md`. Chạy một bản OpenKi trên một cổng;
không chạy đồng thời bản Windows và WSL nếu dùng chung cổng.

## Proton VPN

Proton không có API gateway cho mô hình; cách tích hợp an toàn là dùng SOCKS5 mà
client Proton (hoặc wrapper pVPN) đã mở:

```bash
# Kiểm tra listener trước khi bật trong OpenKi
curl --proxy socks5h://127.0.0.1:1080 https://www.cloudflare.com/cdn-cgi/trace
```

Trong **Mạng → Đầu ra & Proton VPN**, tích `Proton VPN`, nhập URL
`socks5h://127.0.0.1:1080` (hoặc địa chỉ tunnel của pVPN), bấm **Kiểm tra
Proton**, rồi **Lưu & bật Proton**. Hoặc sửa `config.json`:

```json
{
  "egressModes": ["proton"],
  "protonSocks5": "socks5h://127.0.0.1:1080"
}
```

Có thể ghép `"proton"` với `"warp"`, `"relay"` hoặc `"singbox"` để dự phòng.
Listener phải bind loopback hoặc địa chỉ tunnel riêng (pVPN có thể nghe trên
`192.168.110.88:1080` thay vì loopback); không mở SOCKS5 không xác thực ra LAN.
Kiểm tra bằng `ss -ltnp` và chặn cổng đó bằng firewall nếu máy có interface
khác.

Chi tiết triển khai: [docs/OPERATIONS.md](docs/OPERATIONS.md) và
[docs/DEPLOY.md](docs/DEPLOY.md).

## Docker / headless

```bash
docker compose up -d
```

Cổng `8787`, dữ liệu trong volume `/data`, healthcheck `/healthz`. Trang quản trị
luôn chỉ loopback; để truy cập từ xa dùng SSH tunnel hoặc reverse proxy cùng máy.
Cổng public chỉ nên là `/v1/*`, không phải `/admin/*`.

## Build từ mã nguồn

```bash
CGO_ENABLED=0 go build -tags headless -trimpath -o openki ./cmd/openki
go test ./...
```

Kiểm tra fingerprint upstream: `openki -gate-check` (xem
[docs/GATE-AB.md](docs/GATE-AB.md)). Kiểm tra Proton trên dashboard bằng nút
**Kiểm tra Proton**.

## Cập nhật

OpenKi kiểm tra feed GitHub Releases mỗi 6 giờ. Bản Windows có nút cập nhật một
chạm; bản Linux hiển thị commit đang chạy và lệnh `git pull` + build + restart.
Lịch sử thay đổi: [CHANGELOG.md](CHANGELOG.md).

## Giấy phép

**GNU GPL-3.0-only** — SPDX: `GPL-3.0-only`. Toàn văn tại [LICENSE](LICENSE).

- Mã nguồn gốc: [LAGcomcom/zen-gate](https://github.com/LAGcomcom/zen-gate),
  Copyright (C) 2026 LAGcomcom.
- Bản fork: Copyright (C) 2026 chipmax.
- Khi phân phối lại phải kèm giấy phép, giữ thông báo bản quyền và cung cấp mã
  nguồn tương ứng.
- Thành phần bên thứ ba: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md);
  phần WARP có nguồn gốc GPL-3.0, xem `internal/warp/NOTICE`.
