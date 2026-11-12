# Bảng thuật ngữ Trung → Việt — Giao diện Lối Mở (bản Việt hoá)

Áp dụng cho `internal/gateway/web/index.html`. Mọi bản dịch giao diện về sau phải dùng đúng cột "Tiếng Việt" để giữ nhất quán. Tên riêng (Lối Mở, OpenCode, Kilo, Codex, Claude Code, WARP, sing-box, ZCode, Qoder, WorkBuddy...) và tên thương hiệu Trung Quốc (智谱, 讯飞, 商汤, 有道, 魔搭, 华为云, 字节) giữ nguyên, không dịch.

| # | Tiếng Trung | Tiếng Việt | Ghi chú |
|---|---|---|---|
| 1 | 模型 | Mô hình | |
| 2 | 渠道 | Kênh | Trang providers có nhãn điều hướng rút gọn là "Kênh" |
| 3 | 订阅（一般） | Gói đăng ký | |
| 4 | 订阅（proxy/代理订阅） | Đăng ký proxy | Theo ngữ cảnh đăng ký proxy |
| 5 | 订阅轮询 | Luân phiên đăng ký proxy | |
| 6 | 设置 | Cài đặt | |
| 7 | 总览 | Tổng quan | |
| 8 | 首字（延迟） | token đầu tiên (độ trễ token đầu tiên) | |
| 9 | 限流 | bị giới hạn tốc độ | Danh từ: "giới hạn tốc độ" |
| 10 | 额度 | hạn mức | |
| 11 | 网关 | gateway | Giữ nguyên "gateway" khi là khái niệm sản phẩm |
| 12 | 托盘 | khay hệ thống | |
| 13 | 开机自启 | tự khởi động cùng hệ thống | |
| 14 | 自定义 API | API tuỳ chỉnh | |
| 15 | 体检 | kiểm tra tình trạng | Cho model/kênh |
| 16 | 车道 | lane | Giữ nguyên "lane" (thuật ngữ sản phẩm: lane miễn phí...) |
| 17 | 免费 | miễn phí | |
| 18 | 账号 | tài khoản | |
| 19 | 账号池 | nhóm tài khoản | |
| 20 | 密钥 | key | Giữ nguyên "key" (主 Key = Key chính) |
| 21 | 令牌 | token | |
| 22 | 凭据 | thông tin xác thực | |
| 23 | 供应商 | nhà cung cấp | |
| 24 | 公告 | thông báo | |
| 25 | 面板 | bảng điều khiển | Dashboard |
| 26 | 探测 | dò kiểm tra | probe; danh từ: "lần dò kiểm tra" |
| 27 | 可用 | khả dụng | 不可用 = không khả dụng |
| 28 | 状态 | trạng thái | |
| 29 | 默认 | mặc định | |
| 30 | 保存 | lưu | |
| 31 | 取消 | huỷ | |
| 32 | 删除 | xoá | |
| 33 | 添加 | thêm | |
| 34 | 更新 | cập nhật | 一键更新 = cập nhật một chạm |
| 35 | 刷新 | làm mới | |
| 36 | 复制 | sao chép | |
| 37 | 导入 / 导出 | nhập / xuất | |
| 38 | 搜索 | tìm kiếm | |
| 39 | 筛选 | lọc | |
| 40 | 用量 | mức sử dụng | |
| 41 | 请求 | yêu cầu | request |
| 42 | 响应 | phản hồi | |
| 43 | 延迟 | độ trễ | |
| 44 | 节点 | node | Giữ nguyên "node" cho node proxy |
| 45 | 代理 | proxy | Giữ nguyên "proxy" |
| 46 | 授权 | uỷ quyền | 登录授权 = uỷ quyền đăng nhập |
| 47 | 登录 | đăng nhập | |
| 48 | 失败 | thất bại | |
| 49 | 成功 | thành công | |
| 50 | 启用 / 停用 | bật / tắt (đã bật / đã tắt) | Cho công tắc kênh/tính năng |
| 51 | 主用 | dùng chính | 设为主用 = đặt làm mô hình dùng chính |
| 52 | 故障转移/回落 | chuyển dự phòng / quay lui dự phòng | failover / fallback |
| 53 | 冷却 | thời gian chờ (cooldown) | 冷却中 = đang trong thời gian chờ |
| 54 | 恢复 | khôi phục | |
| 55 | 接入 | kết nối | Trang access |
| 56 | 概览（服务/Agent 概览） | tổng quan | 同 #7 |
| 57 | 徽标/徽章 | huy hiệu | badge |
| 58 | 指标 | chỉ số | |
| 59 | 健康（节点健康） | khoẻ / tình trạng tốt | Tuỳ câu: "node khoẻ" hoặc "tình trạng" |

## Quy tắc giữ nguyên (không dịch)

- Tên sản phẩm/dịch vụ: Lối Mở, OpenCode, Kilo, Codex, Claude Code, ZCode, Qoder, WorkBuddy, CodeArts, TRAE, NVIDIA, Gemini, Groq, Cerebras, OpenRouter, Mistral, HuggingFace, ModelScope, WARP, sing-box, Tailwind.
- Tên thương hiệu Trung Quốc (chỉ giữ phần tên, mô tả xung quanh vẫn dịch): 智谱 (Zhipu), 讯飞 (iFlytek), 讯飞星火 (iFlytek Spark), 星火 (Spark), 商汤 (SenseTime), 有道 (Youdao), 魔搭 (ModelScope), 华为云 (Huawei Cloud), 字节 (ByteDance).
- Mã kỹ thuật: tên biến/hàm/lớp CSS, đường dẫn, số và đơn vị (px, ms, %, 127.0.0.1...), mã locale đã đổi `zh-CN` → `vi-VN`.
