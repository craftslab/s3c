package app

import "time"

type TaskStatus string
type Role string
type Permission string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
)

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

const (
	PermissionUpload   Permission = "upload"
	PermissionDownload Permission = "download"
	PermissionCreate   Permission = "create"
	PermissionDelete   Permission = "delete"
	PermissionMove     Permission = "move"
	PermissionRename   Permission = "rename"
	PermissionSearch   Permission = "search"
	PermissionCleanup  Permission = "cleanup"
	PermissionWebhook  Permission = "webhook"
	PermissionPresign  Permission = "presign"
)

var AllPermissions = []Permission{
	PermissionUpload,
	PermissionDownload,
	PermissionCreate,
	PermissionDelete,
	PermissionMove,
	PermissionRename,
	PermissionSearch,
	PermissionCleanup,
	PermissionWebhook,
	PermissionPresign,
}

type TaskItem struct {
	SourceKey string `json:"sourceKey"`
	TargetKey string `json:"targetKey,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type Task struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Status         TaskStatus        `json:"status"`
	Bucket         string            `json:"bucket,omitempty"`
	Prefix         string            `json:"prefix,omitempty"`
	Actor          string            `json:"actor"`
	TotalItems     int               `json:"totalItems"`
	CompletedItems int               `json:"completedItems"`
	CurrentKey     string            `json:"currentKey,omitempty"`
	Message        string            `json:"message,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Items          []TaskItem        `json:"items,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	FinishedAt     *time.Time        `json:"finishedAt,omitempty"`
}

type HistoryEntry struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Bucket    string            `json:"bucket,omitempty"`
	Actor     string            `json:"actor"`
	Keys      []string          `json:"keys,omitempty"`
	Status    string            `json:"status"`
	Message   string            `json:"message,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

type CleanupPolicy struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Bucket        string     `json:"bucket"`
	Prefix        string     `json:"prefix,omitempty"`
	NameContains  string     `json:"nameContains,omitempty"`
	MinSize       int64      `json:"minSize,omitempty"`
	MaxSize       int64      `json:"maxSize,omitempty"`
	OlderThanDays int        `json:"olderThanDays,omitempty"`
	KeepLatest    int        `json:"keepLatest,omitempty"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	LastRunAt     *time.Time `json:"lastRunAt,omitempty"`
}

type Webhook struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Secret    string    `json:"secret,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type WebhookDelivery struct {
	ID         string                 `json:"id"`
	WebhookID  string                 `json:"webhookId"`
	Webhook    string                 `json:"webhook"`
	Event      string                 `json:"event"`
	Status     string                 `json:"status"`
	StatusCode int                    `json:"statusCode,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Payload    map[string]interface{} `json:"payload"`
	Attempted  time.Time              `json:"attemptedAt"`
}

type Event struct {
	Type      string            `json:"type"`
	Bucket    string            `json:"bucket,omitempty"`
	Actor     string            `json:"actor"`
	Keys      []string          `json:"keys,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

type User struct {
	ID           string       `json:"id"`
	Username     string       `json:"username"`
	Role         Role         `json:"role"`
	Permissions  []Permission `json:"permissions,omitempty"`
	PasswordHash string       `json:"-"`
	Builtin      bool         `json:"builtin,omitempty"`
	Temporary    bool         `json:"temporary,omitempty"`
	ExpiresAt    *time.Time   `json:"expiresAt,omitempty"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type State struct {
	Tasks      []Task            `json:"tasks"`
	History    []HistoryEntry    `json:"history"`
	Policies   []CleanupPolicy   `json:"policies"`
	Webhooks   []Webhook         `json:"webhooks"`
	Deliveries []WebhookDelivery `json:"deliveries"`
	Users      []User            `json:"users,omitempty"`
	Sessions   []Session         `json:"sessions,omitempty"`
}
