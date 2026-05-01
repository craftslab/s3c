# kipup – 高效 S3 存储与下载

[English](README.md) | [中文](README_cn.md)

**kipup** 是 **keep up** 的变体，指代面向文字、语音、视频等互动协作场景的高效 S3 存储与下载工具。它是一个轻量级的 **Go + Vue 3** Web 应用，用于浏览、上传和下载存储在任意兼容 S3 的对象存储（如 MinIO、AWS S3 等）中的文件。

## 功能特性

- 📁 以简洁的文件浏览器界面浏览桶和文件夹
- ⬆️ 流式上传大文件——文件直接从浏览器传输到 S3，无需先缓冲到磁盘
- ⬇️ 流式下载大文件——对象直接从 S3 传输到浏览器
- 🔗 预签名 URL——生成无需凭证即可分享的限时下载或上传链接（过期时间可配置，默认 24 小时，最长 7 天）
- 💬 协作会话——创建登录后可访问的在线协作链接，支持用户白名单、实时聊天、附件管理、S3 文件引用、语音输入和 WebRTC 视频信令
- 📱 移动应用分发——登记存储在 S3 中的 Android/iOS 安装包，生成带有效期的下载页，并可撤销已激活设备
- ➕ 创建 / 🗑️ 删除桶
- 🗑️ 删除单个文件或整个文件夹（递归）
- 🧰 批量下载、移动、重命名和删除文件/文件夹
- 🔎 按名称、大小、前缀和修改时间搜索对象
- 🧾 内置任务中心、操作历史、清理策略和 Webhook
- 👤 支持管理员创建随机账号密码的临时用户，并配置有效期与权限范围
- 🐳 一条命令完成 Docker Compose 部署（MinIO + backend + frontend）

## 架构

```
浏览器  ──HTTP──▶  nginx（前端）
                     │  /api/* 代理
                     ▼
                Go 后端（gin + minio-go v7）
                     │  S3 API
                     ▼
                   MinIO
```

## 快速开始（Docker Compose）

```bash
# 1. 克隆仓库
git clone https://github.com/craftslab/kipup.git
cd kipup

# 2.（可选）自定义凭证
cp .env.example .env
$EDITOR .env

# 3. 构建并启动
docker compose up --build

# 4. 打开浏览器
open http://localhost:3000
```

| 服务 | URL |
|---|---|
| Kipup Web UI | http://localhost:3000 |
| Go API | http://localhost:8080 |
| MinIO Console | http://localhost:9001 |

默认 MinIO 凭证：`minioadmin` / `minioadmin`

默认 Kipup 管理员凭证：`admin` / `admin`

## 本地开发

### 后端

```bash
cd backend
go mod tidy

# 导出环境变量（或使用 .env 文件 + direnv）
export S3_ENDPOINT=localhost:9000
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin

go run .
# API 可通过 http://localhost:8080 访问
```

### 前端

```bash
cd frontend
npm install
npm run dev
# 带 HMR 的开发服务器运行于 http://localhost:3000
# API 请求会代理到 http://localhost:8080
```

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | 后端监听地址 |
| `ADMIN_USERNAME` | `admin` | 用于登录的初始化管理员用户名 |
| `ADMIN_PASSWORD` | `admin` | 用于登录的初始化管理员密码 |
| `S3_ENDPOINT` | `localhost:9000` | S3/MinIO 端点（host:port） |
| `S3_PUBLIC_URL` | *(空)* | 返回的预签名链接中使用的 S3 公网基础 URL（例如 `https://s3.example.com`） |
| `PUBLIC_BASE_URL` | *(空)* | 用于构建可分享代理下载链接的 Kipup Web 入口公网基础 URL（例如 `https://kipup.example.com`） |
| `S3_ACCESS_KEY` | `minioadmin` | S3 访问密钥 |
| `S3_SECRET_KEY` | `minioadmin` | S3 密钥 |
| `S3_USE_SSL` | `false` | 是否使用 HTTPS 连接 S3 |
| `S3_REGION` | `us-east-1` | S3 区域 |
| `DATA_FILE` | `./data/state.json` | 用于持久化任务、历史记录、清理策略和 Webhook 的 JSON 文件 |
| `CLEANUP_INTERVAL_SECONDS` | `3600` | 后台执行已启用清理策略的时间间隔 |

## API 参考

除 `/api/v1/auth/sign-up` 和 `/api/v1/auth/sign-in` 外，所有 `/api/v1/*` 接口都需要 `Bearer` token。注册默认会创建普通用户账号，并授予 `upload`、`download`、`search`、`presign` 四项默认权限；管理员可在界面中调整用户角色和权限，也可创建带随机账号密码和过期时间的临时用户。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/auth/sign-up` | 创建普通用户账号 `{"username":"…","password":"…"}` |
| POST | `/api/v1/auth/sign-in` | 登录并获取 bearer token |
| GET | `/api/v1/auth/me` | 获取当前登录用户 |
| POST | `/api/v1/auth/sign-out` | 登出当前会话 |
| GET | `/api/v1/users` | 列出用户（仅管理员） |
| POST | `/api/v1/users/temp` | 创建带随机凭证、过期时间和权限的临时用户（仅管理员） |
| PUT | `/api/v1/users/:username` | 更新用户角色和权限（仅管理员） |
| DELETE | `/api/v1/users/:username` | 删除用户（仅管理员） |
| GET | `/api/v1/buckets` | 列出桶 |
| POST | `/api/v1/buckets` | 创建桶 `{"name":"…","region":"…"}` |
| DELETE | `/api/v1/buckets/:bucket` | 删除桶 |
| GET | `/api/v1/objects/:bucket?prefix=` | 列出对象 / 文件夹 |
| GET | `/api/v1/objects/:bucket/*key` | 下载对象（流式） |
| POST | `/api/v1/objects/:bucket?prefix=` | 上传文件（multipart 流式） |
| DELETE | `/api/v1/objects/:bucket/*key` | 删除对象或文件夹（递归） |
| GET | `/api/v1/search/:bucket` | 按前缀 / 名称 / 大小 / 时间过滤搜索对象 |
| POST | `/api/v1/operations/:bucket/download` | 将选中的文件/文件夹打包为 ZIP 下载 |
| POST | `/api/v1/operations/:bucket/delete` | 批量删除文件/文件夹 |
| POST | `/api/v1/operations/:bucket/move` | 批量移动文件/文件夹到某个前缀 |
| POST | `/api/v1/operations/:bucket/rename` | 批量重命名文件/文件夹 |
| GET | `/api/v1/tasks` | 列出最近任务和进度 |
| GET | `/api/v1/history` | 列出操作历史 |
| GET/POST/PUT/DELETE | `/api/v1/cleanup-policies` | 管理清理策略 |
| POST | `/api/v1/cleanup-policies/:id/run` | 立即执行某个清理策略 |
| GET/POST/PUT/DELETE | `/api/v1/webhooks` | 管理 Webhook 订阅 |
| GET | `/api/v1/webhook-deliveries` | 列出最近的 Webhook 投递记录 |
| GET | `/api/v1/presign/download/:bucket/*key` | 生成预签名下载 URL |
| GET | `/api/v1/presign/upload/:bucket/*key` | 生成预签名上传 URL |
| GET/POST | `/api/v1/collaboration/sessions` | 列出或创建协作会话 |
| GET/PUT/DELETE | `/api/v1/collaboration/sessions/:token` | 获取、更新或删除协作会话 |
| POST | `/api/v1/collaboration/sessions/:token/close` | 关闭协作会话 |
| POST | `/api/v1/collaboration/sessions/:token/messages` | 发送聊天消息 |
| POST | `/api/v1/collaboration/sessions/:token/attachments` | 上传会话附件 |
| GET/DELETE | `/api/v1/collaboration/sessions/:token/attachments/:attachmentId/*` | 下载或删除附件 |
| POST | `/api/v1/collaboration/sessions/:token/files` | 将已存在的 S3 对象加入共享文件列表 |
| GET/DELETE | `/api/v1/collaboration/sessions/:token/files/:fileId/*` | 下载或移除共享文件引用 |
| POST | `/api/v1/collaboration/sessions/:token/stream-token` | 生成 SSE 实时流令牌 |
| GET | `/api/v1/collaboration/sessions/:token/stream?streamToken=` | 通过 SSE 接收实时协作事件 |
| POST | `/api/v1/collaboration/sessions/:token/signal` | 交换 WebRTC 信令数据 |
| GET/POST | `/api/v1/mobile/releases` | 列出或创建移动应用发布记录（仅管理员） |
| POST | `/api/v1/mobile/releases/:id/revoke` | 撤销一个移动应用发布及其已安装客户端（仅管理员） |
| POST | `/api/v1/mobile/releases/:id/download-links` | 生成一个带有效期的移动应用下载页（仅管理员） |
| GET | `/api/v1/mobile/releases/:id/installations` | 列出某个发布下已激活的移动设备（仅管理员） |
| POST | `/api/v1/mobile/installations/:id/revoke` | 撤销单个已激活设备（仅管理员） |
| GET | `/api/v1/mobile/download-links/:token` | 获取公开移动下载页元数据 |
| GET | `/api/v1/mobile/download-links/:token/file` | 下载 Android/iOS 安装包 |
| POST | `/api/v1/mobile/download-links/:token/activate` | 为一个已安装客户端激活启动校验令牌 |
| POST | `/api/v1/mobile/installations/validate` | 校验移动客户端是否仍可启动 |

### 角色与权限模型

- `admin` 拥有全部操作权限。
- `user` 可单独授予这些权限：`upload`、`download`、`create`、`delete`、`move`、`rename`、`search`、`cleanup`、`webhook`、`presign`。
- 临时用户固定使用 `user` 角色，必须设置过期时间，并会在过期后自动失效。
- 基于预签名链接的共享 `/upload` 和 `/download` 代理路由仍可匿名访问。

### 预签名 URL 接口

两个预签名接口都支持可选的 `expiry` 查询参数（单位：秒）。

| 参数 | 默认值 | 最大值 | 说明 |
|---|---|---|---|
| `expiry` | `86400`（24 小时） | `604800`（7 天） | 链接有效期（秒） |

`下载链接`——返回一个预签名 `GET` URL，任何人都可以在无需凭证的情况下使用它下载对象：

```
GET /api/v1/presign/download/:bucket/*key?expiry=3600
```

```json
{
  "url": "https://…/bucket/key?X-Amz-Expires=3600&…",
  "expires_in": 3600
}
```

`上传链接`——返回一个预签名 `PUT` URL，允许在无需凭证的情况下将内容上传到指定 key：

```
GET /api/v1/presign/upload/:bucket/*key?expiry=3600
```

```json
{
  "url": "https://…/bucket/key?X-Amz-Expires=3600&…",
  "key": "path/to/object",
  "expires_in": 3600
}
```

在 Web 界面中，`生成下载链接` 现在会生成一个共享页面，既可下载当前文件，也可在相同的有效期内上传替换文件。

### 协作会话

- 协作链接使用 `/collaboration/:token`，打开后会先进入登录流程，再校验是否在允许访问名单中。
- 会话创建者、管理员以及白名单中的普通用户/临时用户均可进入协作页。
- 会话附件会隔离保存到所选桶下的 `.kipup/collaboration/<token>/attachments/` 前缀中，避免误删业务对象。
- “共享文件”仅保存对现有 S3 对象的引用，不会复制原始文件。
- 协作页提供文字聊天、浏览器语音转文字输入，以及基于 WebRTC 信令的摄像头/语音通话能力。

### 移动应用分发

- 管理员可在 `/mobile-apps` 页面中把已有 S3 对象登记为带过期时间的 Android/iOS 发布记录。
- 每个下载页都会同时提供安装包下载按钮和 Flutter 客户端首启所需的激活码。
- Flutter 移动端脚手架位于 `/mobile_app`，启动时会调用 `/api/v1/mobile/installations/validate` 做超时/撤销校验。
- 当发布过期、被手动撤销，或关联协作会话关闭/过期时，移动端应阻止继续启动并清理本地状态。

## 许可证

[Apache 2.0](LICENSE)
