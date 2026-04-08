# Cliplink 开发过程复盘

从零到 GitHub Release 的完整开发记录——设计决策、踩过的坑、学到的知识。

---

## 目录

1. [需求演进](#1-需求演进)
2. [架构设计决策](#2-架构设计决策)
3. [关键代码解析](#3-关键代码解析)
4. [开发过程中的 Bug 和修复](#4-开发过程中的-bug-和修复)
5. [测试策略](#5-测试策略)
6. [跨平台工程问题](#6-跨平台工程问题)
7. [部署和运维](#7-部署和运维)
8. [Codex 对抗性 Review 发现](#8-codex-对抗性-review-发现)
9. [知识点索引](#9-知识点索引)

---

## 1. 需求演进

### 起点

两台电脑（Mac + Windows）通过 Tailscale 组内网，想在两台之间快速复制粘贴文字。

### 调研阶段

搜索了 GitHub 上的现有方案：

| 工具 | 评价 |
|------|------|
| **Telltail** | 专为 Tailscale 设计，43 stars，只支持文本 |
| **Deskflow** (Synergy fork) | 功能全（键鼠+剪贴板），但太重 |
| **Uniclip** | 409 stars 但 2023 后不活跃 |

**决策：自己造**——既是实际需求，也是 Go 练手项目。

### 需求收敛

通过 brainstorm 明确了核心问题：

- **单向 vs 双向？** → 双向
- **只传文本 vs 也传图片？** → 文本 + 剪贴板图片（截图）
- **文件传输？** → 不做，Tailscale Taildrop 已覆盖
- **自动同步 vs 手动推送？** → 手动推送（按热键触发）

**关键洞察**：剪贴板图片 ≠ 文件。剪贴板里的"图片"是 PNG 像素数据（没有文件名），"复制文件"存的是文件路径引用。它们需要完全不同的传输逻辑。

---

## 2. 架构设计决策

### 决策 1: 对称设计

```
每台机器跑相同的程序：
  cliplink daemon  → HTTP server（接收端）
  cliplink send    → HTTP client（发送端）
```

**为什么不用 P2P 或 WebSocket？**

HTTP request-response 是最简单的模型。send 是一次性操作（不需要保持连接），daemon 只需要一个标准的 HTTP server。用 Go 的 `net/http` 标准库就够了，零外部依赖。

### 决策 2: Board 接口抽象

```go
type Board interface {
    ReadText() ([]byte, error)
    ReadImage() ([]byte, error)
    WriteText(data []byte) error
    WriteImage(data []byte) error
}
```

**为什么要抽象？** 不是为了"以后可能换剪贴板库"这种 YAGNI 理由，而是为了**测试**。剪贴板操作依赖系统环境（GUI session），无法在 CI 或 headless 环境中测试。Board 接口让 server 和 client 的所有网络逻辑都可以用 MockBoard 测试，不需要真正的剪贴板。

**效果**：30+ 个自动化测试，全部不依赖系统剪贴板。

### 决策 3: Config 不验证 peer

```go
// LoadConfig 只验证 port 和 max_size
// 不验证 peer —— daemon 不需要 peer
func LoadConfig(path string) (Config, error) { ... }

// 各命令按需验证
func requirePeer(cfg Config) error { ... }
```

**为什么？** 这是 Codex 第一次 review 发现的 blocker。原始设计在 LoadConfig 里验证 peer，但 daemon 命令不需要 peer（它只需要知道自己监听什么端口）。如果强制要求 peer，用户必须在 config 里写上对方 IP 才能启动 daemon——即使 daemon 根本不用这个字段。

**原则**：验证应该在使用点，不在解析点。Parse, don't validate 的反面教训。

### 决策 4: CLI flag 覆盖顺序

```
config 文件 → 解析 → 应用 CLI flag 覆盖 → 验证
```

**不是**：
```
config 文件 → 解析 → 验证 → 应用 CLI flag 覆盖  ← 错误！覆盖永远不生效
```

这也是 Codex review 发现的 blocker。如果先验证再覆盖，那么 config 文件缺少 peer 时，即使你用 `--peer` flag 提供了，也会在验证阶段就报错。

**教训**：config 覆盖链（文件 → 环境变量 → CLI flag）是 CLI 工具中最容易写错的逻辑。

### 决策 5: Tailscale IP 自动检测

```go
func ResolveBind(bind string) string {
    if bind != "" { return bind }  // 用户显式指定了，尊重它
    // 扫描网卡，找 Tailscale CGNAT 范围
    // 100.64.0.0/10 = 100.64.x.x ~ 100.127.x.x
    for _, iface := range interfaces {
        if isTailscaleIP(ip) { return ip.String() }
    }
    return "127.0.0.1"  // 找不到就只绑本地
}
```

**为什么不绑 0.0.0.0？** 绑 0.0.0.0 意味着所有网络接口都能访问 daemon。如果你在咖啡馆连 WiFi，同一网络的人可以往你的剪贴板写东西。绑定 Tailscale IP 后，只有 tailnet 内的设备能连。

**Fallback 为什么是 127.0.0.1 而不是 0.0.0.0？** Fail safe > fail open。如果检测不到 Tailscale，宁可让 daemon 不可用（安全），也不要暴露给所有网络（危险）。

### 决策 6: 热键不写在 Go 里

```
Go 只负责: cliplink send（CLI 命令）
热键绑定: Hammerspoon (Mac) / AutoHotkey (Windows)
```

**为什么？** 全局热键需要平台特定的 API（macOS 的 CGEvent、Windows 的 RegisterHotKey）。在 Go 里做需要大量 CGO 代码或 syscall。分离后：
- Go 代码保持跨平台，无平台 #ifdef
- 热键工具各平台有成熟方案，不需要重新造
- 用户可以自定义热键组合，不被硬编码限制

---

## 3. 关键代码解析

### HTTP 协议设计

```
POST /clip
Content-Type: text/plain; charset=utf-8  或  image/png
Body: 原始剪贴板数据（文本字节 或 PNG 字节）
```

**为什么不用 JSON 包装？**

选项 A: JSON `{ "type": "image", "data": "base64..." }`
选项 B: 直接发二进制，Content-Type 区分

选了 B。因为 base64 会给图片数据增加 33% 的体积。虽然在 LAN 上影响不大，但直接发二进制更简洁，也更符合 HTTP 语义（Content-Type 本来就是干这个的）。

### Server 双层大小限制

```go
func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
    // 第一层: Content-Length 头检查（快速拒绝，不读 body）
    if r.ContentLength > s.maxSize {
        http.Error(w, "too large", 413)
        return
    }

    // 第二层: 实际 body 读取限制（防止没有 Content-Length 的请求）
    limited := io.LimitReader(r.Body, s.maxSize+1)
    data, _ := io.ReadAll(limited)
    if int64(len(data)) > s.maxSize {
        http.Error(w, "too large", 413)
        return
    }
}
```

**为什么两层？** 攻击者可以不发 Content-Length 头，或发一个虚假的小值。第一层是快速优化（避免读取大 body），第二层是真正的安全保障。`LimitReader(body, maxSize+1)` 中的 `+1` 技巧：读 maxSize+1 字节，如果真的读到了这么多，说明超限了。

### Client 文本优先策略

```go
func (c *Client) readClipboard() ([]byte, string, error) {
    text, textErr := c.board.ReadText()
    if textErr == nil && len(text) > 0 {
        return text, "text/plain; charset=utf-8", nil
    }

    img, imgErr := c.board.ReadImage()
    if imgErr == nil && len(img) > 0 {
        return img, "image/png", nil
    }

    // 区分"空剪贴板"和"读取失败"
    if textErr != nil && imgErr != nil {
        return nil, "", fmt.Errorf("clipboard read failed (text: %v; image: %v)", textErr, imgErr)
    }
    return nil, "", fmt.Errorf("clipboard is empty")
}
```

**为什么文本优先？** 复制网页内容时，剪贴板可能同时包含文本格式和图片格式。用户大多数时候想传的是文字，不是网页截图。

**错误区分的重要性**：最初版本把所有失败都报 "clipboard is empty"。Codex review 指出这吞掉了真实错误（比如权限被拒绝）。修复后区分了两种情况：
- 两个格式都读取成功但为空 → "clipboard is empty"
- 两个格式都读取失败 → "clipboard read failed: text: ...; image: ..."

### 连接超时 vs 总超时

```go
http: &http.Client{
    Timeout: 15 * time.Second,  // 总超时（允许大图片传输）
    Transport: &http.Transport{
        Proxy: nil,
        DialContext: (&net.Dialer{
            Timeout: 2 * time.Second,  // 连接超时（LAN 应该 <10ms）
        }).DialContext,
    },
}
```

**原始问题**：总超时设为 10 秒，Windows 端没跑 daemon 时要等整整 10 秒才报错。用户体验极差。

**修复思路**：把"连不上"和"传输慢"分开。TCP 握手（连接阶段）在 LAN 上应该 <10ms，2 秒绰绰有余。一旦连上了，传输大图片可能需要更长时间，所以总超时给 15 秒。

**效果**：失败从 10 秒降到 2.4 秒，成功仍然 ~20ms。

### http.Server 超时配置

```go
s.httpSrv = &http.Server{
    Addr:         fmt.Sprintf("%s:%d", bind, port),
    Handler:      s,
    ReadTimeout:  30 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

**为什么不用 `http.ListenAndServe`？** 那是个便利函数，内部创建一个零值 `http.Server`——没有任何超时。意味着：
- 慢客户端可以永远占着连接（Slowloris 攻击）
- 没有 Shutdown 方法（无法优雅关闭）

生产代码应该始终自己构造 `http.Server`。

### 优雅关闭

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

go func() {
    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    srv.Shutdown(shutdownCtx)
}()

if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
    return fmt.Errorf("server: %w", err)
}
```

**流程**：
1. `signal.NotifyContext` 监听 SIGINT/SIGTERM
2. 收到信号后，`srv.Shutdown` 会：停止接受新连接 → 等待正在处理的请求完成 → 关闭
3. `ListenAndServe` 返回 `http.ErrServerClosed`——这是预期行为，不应报错
4. 5 秒超时：如果有请求卡住，不会无限等待

**原始版本的问题**：直接 `os.Exit(0)`。这会立刻终止进程，正在传输的剪贴板数据会丢失。

---

## 4. 开发过程中的 Bug 和修复

### Bug 1: HTTP 代理劫持（最意外的 bug）

**现象**：`cliplink send` 返回 502 Bad Gateway，`cliplink status` 也是 502。

**排查过程**：
```bash
curl -v http://100.85.255.70:8275/health
# 发现：请求被转发到了 127.0.0.1:7890（Clash 代理）！
```

**根因**：机器上有 `http_proxy=http://127.0.0.1:7890` 环境变量（Clash 代理）。Go 的 `http.Client` 默认读取这个环境变量，把所有 HTTP 请求都发给代理。代理当然连不上 Tailscale 内网 IP。

**修复**：`Transport: &http.Transport{Proxy: nil}`——显式告诉 HTTP client 不走代理。

**教训**：
- 单元测试用 `httptest` 不会触发代理（因为连的是 localhost）
- 只有真实网络请求才会暴露这个问题
- 面向内网的工具应该始终禁用代理

### Bug 2: PowerShell UTF-8 BOM

**现象**：Windows 上 `cliplink daemon` 报 `invalid character '茂' looking for beginning of value`。

**根因**：PowerShell 5 的 `Out-File -Encoding utf8` 写入 UTF-8 **with BOM**（3 字节 `EF BB BF`）。Go 的 `json.Unmarshal` 严格遵循 JSON spec（RFC 8259），不允许 BOM。

**修复**：
```powershell
# 错误：带 BOM
$config | Out-File -Encoding utf8 $CONFIG_FILE

# 正确：无 BOM
[System.IO.File]::WriteAllText($CONFIG_FILE, $config, [System.Text.UTF8Encoding]::new($false))
```

**教训**：PowerShell 5 和 7 的 `-Encoding utf8` 行为不同（5 带 BOM，7 不带）。跨平台工具生成的配置文件必须注意编码。

### Bug 3: Makefile 缺 mkdir

**现象**：clean checkout 后 `make build` 失败——`dist/` 目录不存在。

**修复**：每个 build target 前加 `@mkdir -p dist`。

**教训**：Makefile 应该是幂等的——从任何状态运行都应该成功。

---

## 5. 测试策略

### 分层测试

```
┌─────────────────────────────┐
│ 集成测试（手动 / 验证 agent）│  真实网络 + 真实剪贴板
├─────────────────────────────┤
│ Server 测试 (httptest)      │  MockBoard + 真实 HTTP 处理
├─────────────────────────────┤
│ Client 测试 (httptest)      │  MockBoard + mock HTTP server
├─────────────────────────────┤
│ Config 测试                 │  纯逻辑，无外部依赖
├─────────────────────────────┤
│ CGNAT 范围测试 (table test) │  纯函数，边界值
└─────────────────────────────┘
```

### MockBoard 的设计

```go
type MockBoard struct {
    text     []byte   // 预设的文本数据
    image    []byte   // 预设的图片数据
    readErr  error    // 注入读取错误
    writeErr error    // 注入写入错误
    written  []byte   // 记录最后写入的数据
}
```

四个控制旋钮：预设数据、注入错误。一个观察点：记录写入。这足以覆盖所有 server/client 的边界情况。

### Table-Driven Test 模式

```go
func TestIsTailscaleIP(t *testing.T) {
    tests := []struct {
        ip   string
        want bool
    }{
        {"100.64.0.0", true},       // 范围起始
        {"100.127.255.255", true},  // 范围结束
        {"100.63.255.255", false},  // 刚好低于范围
        {"100.128.0.0", false},     // 刚好高于范围
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.ip, func(t *testing.T) {
            got := isTailscaleIP(net.ParseIP(tt.ip))
            if got != tt.want { t.Errorf(...) }
        })
    }
}
```

这是 Go 的标准测试模式。关键是**边界值**：不只测"中间的正常值"，更要测"边界上的值"和"刚好超出边界的值"。

### 最终测试统计

40 个测试（含子测试），全部通过。覆盖：
- Config 加载：6 个（含无效 JSON、缺字段、默认值）
- Tailscale IP 检测：9 个边界值 + 2 个 ResolveBind + 1 个 ConfigPath
- Server HTTP 处理：8 个（含超大 payload、空 body、错误 method、写入失败）
- Client 发送：8 个（含空剪贴板、peer 拒绝、不可达、读取错误）
- CLI 集成：5 个（config 覆盖、peer 验证、daemon 不需要 peer）

---

## 6. 跨平台工程问题

### Go 交叉编译

```makefile
build-mac:
    GOOS=darwin GOARCH=arm64 go build -o dist/cliplink-mac-arm64 .

build-windows:
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dist/cliplink-windows-amd64.exe .
```

**关键**：`golang.design/x/clipboard` 在 macOS 上需要 CGO（Objective-C bridge），在 Windows 上不需要（用 syscall）。所以：
- 在 Mac 上编译 Mac 版本：CGO 自动启用（Xcode 工具链）
- 在 Mac 上交叉编译 Windows 版本：`CGO_ENABLED=0` 即可，因为 Windows 的 clipboard 实现不用 CGO

Go 的交叉编译之所以强大，是因为编译器可以根据 `GOOS` 选择不同的 `_darwin.go` / `_windows.go` 源文件。

### Config 路径

```go
func ConfigPath() string {
    if dir, err := os.UserConfigDir(); err == nil {
        return filepath.Join(dir, "cliplink", "config.json")
    }
    // fallback
}
```

`os.UserConfigDir()` 返回：
- macOS: `~/Library/Application Support`
- Windows: `%APPDATA%` (通常是 `C:\Users\xxx\AppData\Roaming`)
- Linux: `$XDG_CONFIG_HOME` 或 `~/.config`

**不要手写 `if runtime.GOOS == "windows"`**——标准库已经处理了。

### Windows 剪贴板 Session 限制

Windows Service 运行在 Session 0（隔离会话），无法访问用户的剪贴板。这是 cliplink 在 Windows 上不能注册为 Windows Service 的根本原因，只能作为用户进程在 Startup 文件夹启动。

---

## 7. 部署和运维

### Mac: launchd

```xml
<key>KeepAlive</key>
<true/>
```

这一行让 launchd 在 daemon 崩溃后自动重启。比 Windows Startup 文件夹更可靠。

### Windows: VBS 静默启动

```vbs
WshShell.Run """C:\path\cliplink.exe"" daemon", 0, False
```

参数 `0` = 隐藏窗口。如果用 `.bat` 文件，会闪出黑色命令行窗口再消失。VBS 的 `WshShell.Run` 可以完全静默。

### Hammerspoon Toast 通知

用 `hs.canvas` 自绘通知而不是 `hs.alert`，因为：
- `hs.alert` 在屏幕正中间显示巨大文字，很突兀
- `hs.canvas` 可以控制位置（右上角）、大小、颜色、样式
- 用状态点（绿色/红色）直观显示成功/失败

---

## 8. Codex 对抗性 Review 发现

Codex 以攻击者视角审查了整个代码库，发现 3 个 blocker：

1. **无认证**：任何能连到 daemon 端口的人都能写剪贴板。v0.1.0 通过绑定 Tailscale IP 缓解（只有 tailnet 内可达），v0.2.0 应加 bearer token。

2. **剪贴板写入无法验证**：`clipboard.Write()` 不返回 error。daemon 返回 200 但数据可能没有真正写入剪贴板。

3. **PNG 解压炸弹**：一个 10MB 的 PNG 文件可以解压成 GB 级内存。需要在写入前解码 PNG header 检查实际尺寸。

**完整列表**：见 GitHub Issue weishh/cliplink#1

---

## 9. 知识点索引

方便日后查阅：

| 知识点 | 在哪里用到 | 章节 |
|--------|-----------|------|
| Go Board 接口抽象 + Mock 测试 | board.go, board_mock_test.go | §2 决策 2, §5 |
| HTTP LimitReader 防大 payload | server.go handleClipboard | §3 双层限制 |
| Go 交叉编译 + CGO 条件 | Makefile | §6 |
| TCP 连接超时 vs 总超时 | client.go NewClient | §3 连接超时 |
| HTTP 代理环境变量陷阱 | client.go Transport{Proxy:nil} | §4 Bug 1 |
| PowerShell UTF-8 BOM | setup-windows.ps1 | §4 Bug 2 |
| macOS launchd 服务管理 | setup-mac.sh, plist | §7 |
| Windows Startup 文件夹 vs Service | setup-windows.ps1 | §7 |
| Tailscale CGNAT 100.64.0.0/10 | config.go isTailscaleIP | §2 决策 5 |
| http.Server 超时 vs http.ListenAndServe | server.go NewServer | §3 Server 超时 |
| signal.NotifyContext 优雅关闭 | main.go cmdDaemon | §3 优雅关闭 |
| Go table-driven test 模式 | config_test.go TestIsTailscaleIP | §5 |
| macOS Accessibility 权限 | Hammerspoon 配置 | §7 |
| os.UserConfigDir 跨平台路径 | config.go ConfigPath | §6 |
| Hammerspoon hs.canvas 自绘 UI | ~/.hammerspoon/init.lua | §7 |
| Config 覆盖顺序（文件→flag→验证） | main.go loadConfig | §2 决策 4 |
