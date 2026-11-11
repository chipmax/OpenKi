# Thông báo bên thứ ba — zen-gate-vi

Dự án này phát hành theo **GNU GPL-3.0-only** (xem [LICENSE](LICENSE) và
[COPYING](COPYING)). Các thành phần dưới đây được giữ nguyên giấy phép gốc;
chúng tương thích với GPL-3.0 và không làm thay đổi nghĩa vụ GPL của bản phân
phối tổng thể.

## Tổng quan

| Thành phần | Nguồn gốc | Giấy phép | Cách phân phối |
|---|---|---|---|
| `internal/lane` (một phần giao thức làn) | dsh-our-free-model (zouyuxuan122) | MIT | Mã nguồn trong repo |
| `internal/qoderhub` | qodercan-gateway (Tiancheng Lu và cộng sự) | MIT | Mã nguồn trong repo, kèm `internal/qoderhub/LICENSE` |
| `internal/warp` | opencode2api (jasonxu114514), gốc là ai-route (sudogeeker) | GPL-3.0 (opencode2api); phần gốc ai-route: MIT | Mã nguồn trong repo, kèm `internal/warp/NOTICE` |
| sing-box sidecar | SagerNet/sing-box | GPL-3.0 | **Không** nhúng trong repo; người dùng tải và chạy riêng |
| `relay-server/` | Mã nguồn trong repo này | GPL-3.0-only | Mã nguồn trong repo |

## GPL-3.0

### `internal/warp`

Package WARP userspace được sao chép gần như nguyên trạng từ
[opencode2api](https://github.com/jasonxu114514/opencode2api) (GPL-3.0). Phần
gốc của package này trong [ai-route](https://github.com/sudogeeker/ai-route)
theo MIT. Chi tiết thay đổi và phạm vi tích hợp: `internal/warp/NOTICE` và
`WARP-INTEGRATION.md`.

Nghĩa vụ khi phân phối lại: bản phân phối có chứa `internal/warp` phải tuân
theo GPL-3.0 (mã nguồn tương ứng, thông báo bản quyền, không bổ sung hạn chế
ngoài GPL). Vì toàn bộ zen-gate-vi đã phát hành theo GPL-3.0, nghĩa vụ này được
đáp ứng ở cấp dự án.

### sing-box sidecar

zen-gate **không** nhúng hoặc phát hành lại sing-box. Khi người dùng cấu hình
`singbox` trong `egressModes` (hoặc đặt `singBoxPath`), zen-gate chỉ gọi một
binary sing-box do người dùng tự tải và cài. Binary đó chịu GPL-3.0 của
SagerNet/sing-box; việc tải, nâng cấp và tuân thủ giấy phép binary là trách
nhiệm của người vận hành.

## MIT

Hai thành phần sau theo MIT và giữ nguyên thông báo gốc:

- `internal/lane` — phần giao thức làn có nguồn từ **dsh-our-free-model** của
  zouyuxuan122 (MIT).
- `internal/qoderhub` — gateway QoderCN nhúng, có nguồn từ
  **qodercan-gateway** của Tiancheng Lu và cộng sự (MIT); cây mã vendored dùng
  tên `qodercn-gateway` và giữ nguyên `internal/qoderhub/LICENSE`.

Văn bản MIT áp dụng cho các phần gốc:

```text
MIT License

Copyright (c) dsh-our-free-model contributors (zouyuxuan122)
Copyright (c) 2026 Tiancheng Lu and contributors (qodercan-gateway)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## `relay-server/`

Mẫu relay server trong repo (khi có) phát hành theo MIT; xem `relay-server/`
và tài liệu triển khai tại [docs/RELAY.md](docs/RELAY.md). Việc triển khai
relay trên Vercel/VPS không làm thay đổi giấy phép của zen-gate.

## Giấy phép của mã phụ thuộc Go

Các module Go trong `go.mod` giữ giấy phép riêng của chúng. Khi phân phối lại
binary, cần kèm thông báo bản quyền/giấy phép tương ứng của các module được
liên kết tĩnh. Danh sách đầy đủ có thể sinh bằng `go-licenses` hoặc đọc
`go.sum`; tệp này chỉ ghi các thành phần mã nguồn được sao chép trực tiếp vào
repo.
