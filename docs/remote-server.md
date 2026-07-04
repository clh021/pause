# 远程控制服务与 Token 认证

最后更新：2026-07-04

本文档只描述当前实现。

## 1. 概览

Pause 内建一个**远程 HTTP 控制服务**（`internal/remoteserver/`），允许在局域网内通过浏览器访问和控制 Pause 实例。该服务默认启用，监听 `0.0.0.0:18680`，并使用 **token 认证**保护所有 API 端点。

## 2. 配置文件 `remote_server.json`

所有远程服务设置保存在 `remote_server.json` 中，位于**应用配置目录**下（见下文第 3 节）。

示例：

```json
{
  "enabled": true,
  "bindAddress": "0.0.0.0",
  "port": 18680,
  "token": "a1b2c3d4e5f6789012345678abcdef0123456789abcdef",
  "triggerCooldownSec": 60
}
```

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 是否启动远程服务 |
| `bindAddress` | string | `"0.0.0.0"` | 监听地址 |
| `port` | int | `18680` | 监听端口（1–65535） |
| `token` | string | 自动生成 | 访问令牌，为空时服务视为未启用认证 |
| `triggerCooldownSec` | int | `60` | 两次手动触发休息的最小间隔（秒） |

### 2.1 Token 自动生成

首次启动时，如果同时满足以下条件：

- `enabled = true`
- `token` 为空或不存在

服务会自动生成一个 **48 字符 hex 随机 token**（`crypto/rand` 生成 24 字节 → hex 编码），并写回配置文件。

如果 `enabled = false` 或 `token` 已存在，则不会覆盖已有 token。

### 2.2 Token 手动配置

用户可以手动在 `remote_server.json` 中设置任意字符串作为 token：

```json
{
  "enabled": true,
  "token": "my-custom-token"
}
```

配置加载时 token 两端空格会被自动 trim。

### 2.3 Token 验证规则

服务启用时（`enabled = true`），token **不能为空**——校验失败会导致服务启动报错。

如果 `enabled = true` 但 `token` 为空（例如因某种原因 token 字段被清空），服务会自动生成新 token 并保存。

## 3. 配置文件位置（各平台）

配置目录由 `os.UserConfigDir()` 决定，Pause 在其下创建 `Pause/` 子目录。

| 平台 | 配置文件路径 |
|---|---|
| **Linux** | `$XDG_CONFIG_HOME/Pause/remote_server.json`（通常为 `~/.config/Pause/remote_server.json`） |
| **macOS** | `~/Library/Application Support/Pause/remote_server.json` |
| **Windows** | `%AppData%\Pause\remote_server.json`（通常为 `C:\Users\<用户名>\AppData\Roaming\Pause\remote_server.json`） |

**兜底策略**：如果 `os.UserConfigDir()` 不可用（极少见），回退到 `~/.pause/remote_server.json`。

**日志目录**独立于配置目录：

| 平台 | 日志目录 |
|---|---|
| **macOS** | `~/Library/Logs/Pause/` |
| **Linux / Windows** | `<CacheDir>/Pause/logs/`（Linux：通常 `~/.cache/Pause/logs/`；Windows：`%LocalAppData%\Pause\logs\`） |

### 3.1 实用命令

使用 `--print-remote-info` 参数可直接打印当前配置文件路径和 token：

```bash
# Linux / macOS
./Pause --print-remote-info

# Windows
Pause.exe --print-remote-info
```

输出示例：

```
version=0.9.3
branch=main
commit=abc1234
build_time=2026-07-03T12:00:00Z
vcs_modified=false
config=/home/user/.config/Pause/remote_server.json
local_url=http://127.0.0.1:18680
token=a1b2c3d4e5f6789012345678abcdef0123456789abcdef
bind=0.0.0.0:18680
enabled=true
```

## 4. Token 认证流程

### 4.1 架构

```
浏览器 / 前端               远程 HTTP 服务
     │                           │
     │  POST /api/auth/session   │
     │  { "token": "..." }       │
     │ ───────────────────────>  │
     │                           ├─ tokensEqual() ← subtle.ConstantTimeCompare
     │                           │
     │  Set-Cookie:              │
     │  pause_remote_auth=...    │
     │ <───────────────────────  │
     │                           │
     │  GET /api/...             │
     │  Cookie: pause_remote_auth│
     │ ───────────────────────>  │
     │                           ├─ requireAuth 中间件
     │                           │   └─ isAuthorized()
     │                           │       └─ requestToken()
     │                           │
     │  200 OK                   │
     │ <───────────────────────  │
```

### 4.2 Token 提取优先级

`requestToken()` 按以下顺序尝试提取 token：

1. **`Authorization: Bearer <token>`** — HTTP 请求头
2. **`pause_remote_auth` Cookie** — 登录成功后设置的会话 cookie
3. **`access_token`** — URL query 参数（主要用于桌面内嵌前端直连，用 `GetRemoteServerInfo()` 获取明文 token 后附加在请求 URL 中）

找到第一个非空 token 即返回。

### 4.3 认证中间件

所有 `/api/*` 路由均经过 `requireAuth` 中间件：

- 如果 `cfg.Token == ""`（token 为空），中间件直接放行（等价于不启用认证）
- 如果请求方法为 `OPTIONS`（CORS 预检），中间件直接放行
- 否则调用 `isAuthorized()` 校验 token
- 校验失败返回 HTTP `401 Unauthorized`

### 4.4 登录会话

- **POST `/api/auth/session`** — 客户端提交 `{ "token": "..." }`，服务端对比 token。匹配则设置 HttpOnly cookie：
  - 名称：`pause_remote_auth`
  - 有效期：30 天（`MaxAge = 2592000`）
  - `SameSite=Strict`，`HttpOnly=true`
  - 后续请求自动携带该 cookie
- **DELETE `/api/auth/session`** — 清除 `pause_remote_auth` cookie，登出

### 4.5 常量时间比较

token 对比使用 `crypto/subtle.ConstantTimeCompare`，防止基于响应时间的侧信道攻击。两个输入均先 trim 空格，任一为空则直接返回 false。

### 4.6 桌面内嵌前端

桌面端（Wails 壳）内嵌的前端通过 `GetRemoteServerInfo()` 调用获取明文 `accessToken`，直接通过 `access_token` query 参数附带在请求中，无需走 cookie 登录流程。

## 5. CLI 相关参数

| 参数 | 说明 |
|---|---|
| `--headless` | 仅启动后台服务，不打开桌面窗口。**Windows 打包版默认以此模式启动** |
| `--windowed` / `--gui` | 显式打开桌面窗口 |
| `--print-remote-info` | 打印版本信息、配置文件路径、本机访问地址和当前 token |
| 环境变量 `PAUSE_HEADLESS=1` | 等同于 `--headless` |

Windows 打包版的行为：

- 默认无参数时走 **headless** 模式（仅后台服务 + 系统托盘）
- 用户双击 `Pause.exe` 后可在系统托盘看到图标，浏览器访问 `http://localhost:18680` 进行控制
- 可通过 `Pause.exe --windowed` 打开桌面窗口

## 6. 安全说明

- Token 使用 `crypto/rand` 生成，**不可预测**
- Token 比对使用 **常量时间比较**，抗时序攻击
- 登录 Cookie 设置 **HttpOnly**（JavaScript 不可读取）和 **SameSite=Strict**（防 CSRF）
- 所有 `/api/*` 端点均受认证保护，包括截图、活动记录等敏感接口
- 配置文件权限：写入时使用 `0o600`（仅所有者可读写）
- `0.0.0.0` 绑定意味着局域网所有设备均可访问——请将 token 视为敏感凭据，不要在不信任的网络中使用

## 7. 关键代码文件

| 文件 | 职责 |
|---|---|
| `internal/remoteserver/config.go` | 配置定义、加载、自动生成 token、持久化 |
| `internal/remoteserver/auth.go` | token 认证中间件、登录/登出、token 提取与比对 |
| `internal/remoteserver/server.go` | HTTP 服务启动、路由注册 |
| `internal/remoteserver/handler.go` | API 处理函数 |
| `internal/paths/paths.go` | 配置目录路径解析（各平台差异） |
| `internal/entry/desktop/cli.go` | CLI 参数解析、`--print-remote-info` 实现 |
| `internal/app/app_api_remote.go` | Wails 绑定，向前端暴露 `GetRemoteServerInfo()` |
| `frontend/src/api.ts` | 前端 token 登录、远程请求封装 |
| `internal/remoteserver/config_test.go` | 配置与 token 行为测试 |
| `internal/remoteserver/handler_test.go` | 认证 handler 测试 |
