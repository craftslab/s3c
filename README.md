# s3c – S3 Cloud Browser

A lightweight **Go + Vue 3** web application for browsing, uploading and downloading files stored in any S3-compatible object store (MinIO, AWS S3, etc.).

## Features

- 📁 Browse buckets and folders in a clean file-browser UI
- ⬆️ **Streaming upload** of large files – files are piped directly from the browser to S3 without buffering to disk
- ⬇️ **Streaming download** of large files – objects are piped from S3 straight to the browser
- 🔗 **Presigned URLs** – generate time-limited download or upload links shareable without credentials (configurable expiry, default 24 h, max 7 days)
- 🪣 Create / delete buckets
- 🗑️ Delete individual files or entire folders (recursive)
- 🐳 One-command **Docker Compose** deployment (MinIO + backend + frontend)

## Architecture

```
Browser  ──HTTP──▶  nginx (frontend)
                       │  /api/* proxy
                       ▼
                  Go backend  (gin + minio-go v7)
                       │  S3 API
                       ▼
                     MinIO
```

## Quick Start (Docker Compose)

```bash
# 1. Clone the repo
git clone https://github.com/craftslab/s3c.git
cd s3c

# 2. (Optional) customise credentials
cp .env.example .env
$EDITOR .env

# 3. Build and launch
docker compose up --build

# 4. Open the browser
open http://localhost:3000
```

| Service | URL |
|---|---|
| S3C Web UI | http://localhost:3000 |
| Go API | http://localhost:8080 |
| MinIO Console | http://localhost:9001 |

Default MinIO credentials: `minioadmin` / `minioadmin`

## Local Development

### Backend

```bash
cd backend
go mod tidy

# Export env vars (or use a .env file + direnv)
export S3_ENDPOINT=localhost:9000
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin

go run .
# API available at http://localhost:8080
```

### Frontend

```bash
cd frontend
npm install
npm run dev
# Dev server with HMR at http://localhost:3000
# API calls proxied to http://localhost:8080
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | Backend listen address |
| `S3_ENDPOINT` | `localhost:9000` | S3/MinIO endpoint (host:port) |
| `S3_ACCESS_KEY` | `minioadmin` | S3 access key |
| `S3_SECRET_KEY` | `minioadmin` | S3 secret key |
| `S3_USE_SSL` | `false` | Use HTTPS for S3 connection |
| `S3_REGION` | `us-east-1` | S3 region |

## API Reference

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/buckets` | List buckets |
| POST | `/api/v1/buckets` | Create bucket `{"name":"…","region":"…"}` |
| DELETE | `/api/v1/buckets/:bucket` | Delete bucket |
| GET | `/api/v1/objects/:bucket?prefix=` | List objects / folders |
| GET | `/api/v1/objects/:bucket/*key` | Download object (streaming) |
| POST | `/api/v1/objects/:bucket?prefix=` | Upload files (multipart streaming) |
| DELETE | `/api/v1/objects/:bucket/*key` | Delete object or folder (recursive) |
| GET | `/api/v1/presign/download/:bucket/*key` | Generate presigned download URL |
| GET | `/api/v1/presign/upload/:bucket/*key` | Generate presigned upload URL |

### Presigned URL endpoints

Both presigned endpoints accept an optional `expiry` query parameter (seconds).

| Parameter | Default | Maximum | Description |
|---|---|---|---|
| `expiry` | `86400` (24 h) | `604800` (7 days) | Link validity period in seconds |

**Download link** – returns a presigned `GET` URL that anyone can use to download the object without credentials:

```
GET /api/v1/presign/download/:bucket/*key?expiry=3600
```

```json
{
  "url": "https://…/bucket/key?X-Amz-Expires=3600&…",
  "expires_in": 3600
}
```

**Upload link** – returns a presigned `PUT` URL that allows uploading to the specified key without credentials:

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

## License

[Apache 2.0](LICENSE)
