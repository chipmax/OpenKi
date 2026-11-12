# Triển khai loimo trên Windows (GUI + tray)

Bản Windows là ứng dụng **GUI + tray** chạy trong phiên đăng nhập của người
dùng. Nó **không phải** Windows service và không có giao diện SCM: không dùng
`sc create`, `New-Service` hay bất kỳ trình quản lý service nào.

## Vì sao không dùng Windows service

- Ứng dụng cần phiên người dùng để tạo tray/WebView2 và để tiêm cấu hình vào
  Agent trong profile người dùng (`%USERPROFILE%`, `%APPDATA%`).
- Service chạy ở session 0, không có desktop/tray; injection sẽ ghi nhầm
  profile hoặc thất bại.
- Cơ chế single-instance và autostart hiện có được thiết kế cho phiên đăng nhập.

Nếu chỉ cần gateway không giao diện trên Windows Server, hãy dùng bản
**headless** trong WSL/Linux (xem `deploy/wsl/loimo.service`), không cài bản
GUI làm service.

## Cài đặt

1. Đặt `loimo.exe` ở thư mục ổn định, ví dụ:
   `%LOCALAPPDATA%\Programs\LoiMo\loimo.exe`.
2. Chạy một lần, mở `http://127.0.0.1:8787`, đặt `mainKey` và chọn Agent cần
   tiêm.
3. Dữ liệu mặc định: `%APPDATA%\loimo` (`config.json`, log, backup Agent).
   Có thể override bằng biến môi trường `LOIMO_HOME` (chỉ nên dùng khi chạy
   instance phụ/kiểm thử).
4. Giữ `allowLan: false`; trang quản trị chỉ nghe `127.0.0.1:8787`.

Khởi động từ terminal (tuỳ chọn):

```powershell
& "$env:LOCALAPPDATA\Programs\LoiMo\loimo.exe"
```

Chỉ chạy gateway, không đụng cấu hình Agent:

```powershell
& "$env:LOCALAPPDATA\Programs\LoiMo\loimo.exe" --no-inject
```

Chỉ tiêm phía Windows:

```powershell
& "$env:LOCALAPPDATA\Programs\LoiMo\loimo.exe" --inject-scope=windows
```

`--inject-scope` nhận `host|windows|wsl|all|none`; `--no-inject` thắng mọi giá
trị scope. Mặc định `agentScope` rỗng = `all`.

## Khởi động cùng đăng nhập

Ưu tiên cơ chế autostart/tray có sẵn trong ứng dụng. Nếu cần Task Scheduler,
dùng trigger **At logon** (không phải At startup) và chạy với quyền user hiện
tại:

```powershell
$exe = "$env:LOCALAPPDATA\Programs\LoiMo\loimo.exe"
$action  = New-ScheduledTaskAction -Execute $exe
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable `
  -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName "Lối Mở" `
  -Description "Lối Mở gateway (GUI/tray) tại đăng nhập" `
  -Action $action -Trigger $trigger -Settings $settings `
  -RunLevel Limited
```

Gỡ:

```powershell
Unregister-ScheduledTask -TaskName "Lối Mở" -Confirm:$false
```

Không đặt task ở mức `Highest` và không chọn "Run whether user is logged on or
not" — hai lựa chọn đó đưa tiến trình ra khỏi phiên tương tác, mất tray và mất
khả năng tiêm cấu hình đúng profile.

## Kiểm tra

```powershell
Invoke-RestMethod http://127.0.0.1:8787/healthz
Get-NetTCPConnection -LocalPort 8787 -State Listen |
  Select-Object LocalAddress,LocalPort
```

- `/healthz` trả `ok: true`.
- `LocalAddress` phải là `127.0.0.1`/`::1`; nếu là `0.0.0.0` nghĩa là
  `allowLan` đang bật — tắt ngay nếu không thật sự cần.
- Không mở cổng `8787` trên Windows Firewall cho mạng ngoài.

## Cập nhật

Dùng nút "Cập nhật một chạm" trong dashboard (chỉ Windows) hoặc tải bản mới và
thay `loimo.exe` khi ứng dụng đã thoát. Sao lưu `%APPDATA%\loimo` trước
khi nâng cấp.

## Liên quan

- `../../docs/DEPLOY.md` — ma trận Windows/WSL/VPS.
- `../config.example.json` — mẫu cấu hình không chứa secret.
