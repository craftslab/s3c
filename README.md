# kipup – Efficient S3 Storage & Download

[English](README.md) | [中文](README_cn.md)

**kipup** — a variation of “keep up” — is a lightweight **Go + Vue 3** web application for efficient S3 storage, downloads, and interactive collaboration scenarios across text, voice, video, and more.

## Features

- 📁 Browse buckets and folders in a clean file-browser UI
- ⬆️ **Batch upload** of files and folders with per-item progress, resumable multipart transfer, and task tracking
- ⬇️ **Streaming download** of large files – objects are piped from S3 straight to the browser
- 🔗 **Presigned URLs** – generate time-limited download or upload links shareable without credentials (configurable expiry, default 24 h, max 7 days)
- ➕ Create / 🗑️ delete buckets
- 🗑️ Delete individual files or entire folders (recursive)
- 🧰 Batch download, move, rename, and delete for files/folders
- 🔎 Search objects by name, size, prefix, and modification time
- 🧾 Built-in task center, operation history, cleanup policies, and webhook
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
git clone https://github.com/craftslab/kipup.git
cd kipup

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
| Kipup Web UI | http://localhost:3000 |
| Go API | http://localhost:8080 |
| MinIO Console | http://localhost:9001 |

Default MinIO credentials: `minioadmin` / `minioadmin`

Default Kipup admin credentials: `admin` / `admin`

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
| `ADMIN_USERNAME` | `admin` | Bootstrap admin username used for sign-in |
| `ADMIN_PASSWORD` | `admin` | Bootstrap admin password used for sign-in |
| `S3_ENDPOINT` | `localhost:9000` | S3/MinIO endpoint (host:port) |
| `S3_PUBLIC_URL` | *(empty)* | Public base URL used in returned presigned links (e.g. `https://s3.example.com`) |
| `PUBLIC_BASE_URL` | *(empty)* | Public base URL of the Kipup web entry used to build shareable proxy download links (e.g. `https://kipup.example.com`) |
| `S3_ACCESS_KEY` | `minioadmin` | S3 access key |
| `S3_SECRET_KEY` | `minioadmin` | S3 secret key |
| `S3_USE_SSL` | `false` | Use HTTPS for S3 connection |
| `S3_REGION` | `us-east-1` | S3 region |
| `DATA_FILE` | `./data/state.json` | JSON file used to persist tasks, history, cleanup policies, and webhooks |
| `CLEANUP_INTERVAL_SECONDS` | `3600` | Background interval for running enabled cleanup policies |

## API Reference

All `/api/v1/*` routes except `/api/v1/auth/sign-up` and `/api/v1/auth/sign-in` require a `Bearer` token. Sign-up creates a normal user account with default permissions for `upload`, `download`, `search`, and `presign`. Admin users can change user roles and permissions in the UI.

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/auth/sign-up` | Create a normal user account `{"username":"…","password":"…"}` |
| POST | `/api/v1/auth/sign-in` | Sign in and receive a bearer token |
| GET | `/api/v1/auth/me` | Get current signed-in user |
| POST | `/api/v1/auth/sign-out` | Sign out current session |
| GET | `/api/v1/users` | List users (admin only) |
| PUT | `/api/v1/users/:username` | Update a user's role and permissions (admin only) |
| DELETE | `/api/v1/users/:username` | Delete a user (admin only) |
| GET | `/api/v1/buckets` | List buckets |
| POST | `/api/v1/buckets` | Create bucket `{"name":"…","region":"…"}` |
| DELETE | `/api/v1/buckets/:bucket` | Delete bucket |
| GET | `/api/v1/objects/:bucket?prefix=` | List objects / folders |
| GET | `/api/v1/objects/:bucket/*key` | Download object (streaming) |
| POST | `/api/v1/objects/:bucket?prefix=` | Upload files (multipart streaming) |
| POST | `/api/v1/uploads/:bucket/resumable/init?prefix=` | Initialize a resumable multipart upload |
| GET | `/api/v1/uploads/:bucket/resumable/status?prefix=&key=&uploadId=` | Query uploaded parts for resume |
| PUT | `/api/v1/uploads/:bucket/resumable/part?prefix=&key=&uploadId=&partNumber=` | Upload a resumable chunk |
| POST | `/api/v1/uploads/:bucket/resumable/complete?prefix=` | Complete a resumable multipart upload |
| DELETE | `/api/v1/uploads/:bucket/resumable?prefix=&key=&uploadId=` | Abort a resumable multipart upload |
| DELETE | `/api/v1/objects/:bucket/*key` | Delete object or folder (recursive) |
| GET | `/api/v1/search/:bucket` | Search objects by prefix/name/size/time filters |
| POST | `/api/v1/operations/:bucket/download` | Download selected files/folders as a ZIP |
| POST | `/api/v1/operations/:bucket/delete` | Batch delete files/folders |
| POST | `/api/v1/operations/:bucket/move` | Batch move files/folders to a prefix |
| POST | `/api/v1/operations/:bucket/rename` | Batch rename files/folders |
| GET | `/api/v1/tasks` | List recent tasks and progress |
| GET | `/api/v1/history` | List operation history |
| GET/POST/PUT/DELETE | `/api/v1/cleanup-policies` | Manage cleanup policies |
| POST | `/api/v1/cleanup-policies/:id/run` | Run a cleanup policy immediately |
| GET/POST/PUT/DELETE | `/api/v1/webhooks` | Manage webhook subscriptions |
| GET | `/api/v1/webhook-deliveries` | List recent webhook deliveries |
| GET | `/api/v1/presign/download/:bucket/*key` | Generate presigned download URL |
| GET | `/api/v1/presign/upload/:bucket/*key` | Generate presigned upload URL |

### Role and permission model

- `admin` users have full access to every operation.
- `user` accounts can be granted these permissions individually: `upload`, `download`, `create`, `delete`, `move`, `rename`, `search`, `cleanup`, `webhook`, `presign`.
- Shared `/upload` and `/download` proxy routes continue to work with presigned links and do not require sign-in.

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

In the web UI, **Generate Download Link** now creates a shared page that can both download the current file and upload a replacement using the same selected expiry.

## License

[Apache 2.0](LICENSE)
