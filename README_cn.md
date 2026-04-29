# s3c – S3 云浏览器

[English](README.md) | [中文](README_cn.md)

一个轻量级的 **Go + Vue 3** Web 应用，用于浏览、上传和下载存储在任意兼容 S3 的对象存储（如 MinIO、AWS S3 等）中的文件。

## 功能特性

- 📁 以简洁的文件浏览器界面浏览桶和文件夹
- ⬆️ **流式上传**大文件——文件直接从浏览器传输到 S3，无需先缓冲到磁盘
- ⬇️ **流式下载**大文件——对象直接从 S3 传输到浏览器
- 🔗 **预签名 URL**——生成无需凭证即可分享的限时下载或上传链接（过期时间可配置，默认 24 小时，最长 7 天）
- ➕ 创建 / 🗑️ 删除桶
- 🗑️ 删除单个文件或整个文件夹（递归）
- 🧰 批量下载、移动、重命名和删除文件/文件夹
- 🔎 按名称、大小、前缀和修改时间搜索对象
- 🧾 内置任务中心、操作历史、清理策略和 Webhook 投递日志
- 🐳 一条命令完成 **Docker Compose** 部署（MinIO + backend + frontend）

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
git clone https://github.com/craftslab/s3c.git
cd s3c

# 2. （可选）自定义凭证
cp .env.example .env
$EDITOR .env

# 3. 构建并启动
docker compose up --build

# 4. 打开浏览器
open http://localhost:3000
```

| 服务 | URL |
|---|---|
| S3C Web UI | http://localhost:3000 |
| Go API | http://localhost:8080 |
| MinIO Console | http://localhost:9001 |

默认 MinIO 凭证：`minioadmin` / `minioadmin`

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
| `S3_ENDPOINT` | `localhost:9000` | S3/MinIO 端点（host:port） |
| `S3_PUBLIC_URL` | *(空)* | 返回的预签名链接中使用的 S3 公网基础 URL（例如 `https://s3.example.com`） |
| `PUBLIC_BASE_URL` | *(空)* | 用于构建可分享代理下载链接的 S3C Web 入口公网基础 URL（例如 `https://s3c.example.com`） |
| `S3_ACCESS_KEY` | `minioadmin` | S3 访问密钥 |
| `S3_SECRET_KEY` | `minioadmin` | S3 密钥 |
| `S3_USE_SSL` | `false` | 是否使用 HTTPS 连接 S3 |
| `S3_REGION` | `us-east-1` | S3 区域 |
| `DATA_FILE` | `./data/state.json` | 用于持久化任务、历史记录、清理策略和 Webhook 的 JSON 文件 |
| `CLEANUP_INTERVAL_SECONDS` | `3600` | 后台执行已启用清理策略的时间间隔 |

## API 参考

| 方法 | 路径 | 说明 |
|---|---|---|
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

### 预签名 URL 接口

两个预签名接口都支持可选的 `expiry` 查询参数（单位：秒）。

| 参数 | 默认值 | 最大值 | 说明 |
|---|---|---|---|
| `expiry` | `86400`（24 小时） | `604800`（7 天） | 链接有效期（秒） |

**下载链接**——返回一个预签名 `GET` URL，任何人都可以在无需凭证的情况下使用它下载对象：

```
GET /api/v1/presign/download/:bucket/*key?expiry=3600
```

```json
{
  "url": "https://…/bucket/key?X-Amz-Expires=3600&…",
  "expires_in": 3600
}
```

**上传链接**——返回一个预签名 `PUT` URL，允许在无需凭证的情况下将内容上传到指定 key：

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

## 许可证

[Apache 2.0](LICENSE)
