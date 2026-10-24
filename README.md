# tunmap · 内网工具映射器

> 填一个端口，点启动，拿到公网地址。任何设备都能访问你本机的服务——就这么简单。

tunmap 是一个用 Go 编写的内网穿透平台（对标 ngrok / frp / 花生壳，面向普通用户简化）。
你只需要**一个端口号**，tunmap 就给你一个公网域名，别人在哪儿都能访问你的服务。

```
公网用户 ──HTTP──▶ tunmap-server（公网服务器）──WebSocket──▶ tunmap-client（你本机/手机）──▶ localhost:端口
```

## 对新手有多简单

**电脑 / 手机客户端**：填端口 → 点启动 → 复制公网地址。就三步。

**服务端**：一台公网服务器，一条命令跑起来，密钥可选（不设就完全开放）。

## 快速开始

### 1. 部署服务端（需要一台公网 VPS / 云服务器）

```bash
docker run -d --name tunmap -p 8080:8080 \
  -e TUNMAP_BASE_DOMAIN=<你的域名或服务器IP> \
  ghcr.io/tunmap/server
```

没有 Docker 也可以直接跑二进制：

```bash
wget <release 下载地址>/tunmap-server-linux-amd64
./tunmap-server --addr :8080 --domain example.com        # 可选 --key 你的密钥
```

### 2. 用客户端（三选一）

**安卓手机**（最简单）：
1. 安装 [tunmap-android.apk](#下载)
2. 第一次打开填一次服务器地址（如 `http://123.45.67.89:8080`），以后不用再填
3. 输入要映射的端口（如 5050）→ 点 **启动** → 底下出现绿色公网地址，点一下复制
4. 想停就点 **停止**

**电脑**：

```bash
./tunmap-client --server http://123.45.67.89:8080 --port 5050
# 也可以只填端口，用环境变量指定服务器：
TUNMAP_SERVER=http://123.45.67.89:8080 ./tunmap-client --port 5050
```

客户端会打印：

```
==================================================
  tunnel ready
  public URL : http://a1b2c3.example.com
  forwards   -> localhost:5050
==================================================
```

把 `http://a1b2c3.example.com` 发给任何人，TA 就能访问你本机 5050 的服务。

## 下载

| 文件 | 说明 |
|---|---|
| `tunmap-android.apk` | 安卓客户端（Android 5.0+） |
| `tunmap-server-linux-amd64` / `-arm64` | 服务端（Linux） |
| `tunmap-client-linux-amd64` / `-arm64` | 客户端（Linux） |
| `tunmap-client-darwin-amd64` / `-arm64` | 客户端（macOS） |
| `tunmap-client-windows-amd64.exe` | 客户端（Windows） |

## 服务端参数

| 参数 | 环境变量 | 说明 | 默认 |
|---|---|---|---|
| `--addr` | `TUNMAP_ADDR` | 监听地址 | `:8080` |
| `--domain` | `TUNMAP_BASE_DOMAIN` | 基础域名，隧道是 `<sub>.<domain>` | `localhost` |
| `--key` | `TUNMAP_AUTH_KEY` | 共享密钥（可选，不设则开放） | 空 |

客户端参数：`--server`、`--port`（必填）、`--key`（可选）、`--subdomain`（可选自定义子域名）。

## Web 控制台

打开 `http://服务器地址/` 即可看到实时面板：在线隧道、请求计数、上传/下载流量、实时请求日志（可点开看请求/响应详情、按方法过滤、搜索 path）。

## 架构

```
internal/
├── protocol/   # WebSocket JSON 消息协议
├── server/     # 公网服务端：WebSocket 中枢、HTTP 反代、实时统计/SSE、Web 控制台(go:embed)
└── client/     # 内网客户端：拨号、断线重连、转发到 localhost
mobile/         # gomobile 绑定层（安卓/iOS 共用）
android/        # 安卓 APP（极简单屏 UI）
cmd/server      # 服务端入口
cmd/client      # 客户端入口
```

## 开发

```bash
# 本地跑服务端（无需公网）
go run ./cmd/server --addr :8080 --domain localhost

# 本地跑客户端
go run ./cmd/client --server http://localhost:8080 --port 5050
# 然后用 curl 测试：curl --resolve <子>.localhost:8080:127.0.0.1 http://<子>.localhost:8080/
```

## 构建

```bash
# 服务端
CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/tunmap-server-linux-amd64 ./cmd/server

# 客户端（交叉编译）
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/tunmap-client-darwin-arm64 ./cmd/client
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/tunmap-client-windows-amd64.exe ./cmd/client

# 安卓 APK（需要 Android SDK + gomobile）
cd mobile && gomobile bind -target=android -androidapi=21 -o ../android/tunmap.aar .
cd ../android && gradle assembleDebug
```

## 路线图

- [ ] HTTPS 自动证书（Let's Encrypt）
- [ ] 自定义域名（CName 接入）
- [ ] 流量统计 / 带宽限制
- [ ] TCP/UDP 隧道（数据库、SSH、游戏）
- [ ] 请求重放、访问日志导出

## License

MIT
