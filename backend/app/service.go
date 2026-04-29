package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/craftslab/s3c/backend/storage"
	"github.com/minio/minio-go/v7"
)

type SearchRequest struct {
	Bucket         string
	Prefix         string
	Name           string
	MinSize        *int64
	MaxSize        *int64
	ModifiedAfter  *time.Time
	ModifiedBefore *time.Time
}

type BatchDeleteRequest struct {
	TaskID string   `json:"taskId"`
	Keys   []string `json:"keys"`
}

type BatchMoveItem struct {
	SourceKey string `json:"sourceKey"`
	TargetKey string `json:"targetKey"`
}

type BatchMoveRequest struct {
	TaskID string          `json:"taskId"`
	Items  []BatchMoveItem `json:"items"`
}

type BatchRenameItem struct {
	SourceKey string `json:"sourceKey"`
	NewName   string `json:"newName"`
}

type BatchRenameRequest struct {
	TaskID string            `json:"taskId"`
	Items  []BatchRenameItem `json:"items"`
}

type BatchDownloadRequest struct {
	Keys []string `json:"keys"`
}

type Service struct {
	client     *storage.Client
	store      *Store
	httpClient *http.Client
}

func NewService(client *storage.Client, store *Store) *Service {
	return &Service{
		client: client,
		store:  store,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *Service) ListTasks() []Task {
	state := s.store.snapshot()
	return state.Tasks
}

func (s *Service) ListHistory() []HistoryEntry {
	state := s.store.snapshot()
	return state.History
}

func (s *Service) ListPolicies() []CleanupPolicy {
	state := s.store.snapshot()
	return state.Policies
}

func (s *Service) ListWebhooks() []Webhook {
	state := s.store.snapshot()
	return state.Webhooks
}

func (s *Service) ListDeliveries() []WebhookDelivery {
	state := s.store.snapshot()
	return state.Deliveries
}

func (s *Service) UpsertTask(id, taskType, bucket, prefix, actor string, total int, metadata map[string]string) string {
	if strings.TrimSpace(id) == "" {
		id = newID(taskType)
	}
	now := time.Now().UTC()
	_ = s.store.update(func(state *State) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == id {
				state.Tasks[i].Type = taskType
				state.Tasks[i].Bucket = bucket
				state.Tasks[i].Prefix = prefix
				state.Tasks[i].Actor = actor
				state.Tasks[i].TotalItems = total
				state.Tasks[i].Metadata = metadata
				state.Tasks[i].Status = TaskRunning
				state.Tasks[i].UpdatedAt = now
				return nil
			}
		}
		state.Tasks = append([]Task{{
			ID:         id,
			Type:       taskType,
			Status:     TaskRunning,
			Bucket:     bucket,
			Prefix:     prefix,
			Actor:      actor,
			TotalItems: total,
			Metadata:   metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}}, state.Tasks...)
		if len(state.Tasks) > 200 {
			state.Tasks = state.Tasks[:200]
		}
		return nil
	})
	return id
}

func (s *Service) UpdateTaskProgress(id, currentKey string, completed int, item TaskItem) {
	if strings.TrimSpace(id) == "" {
		return
	}
	now := time.Now().UTC()
	_ = s.store.update(func(state *State) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID != id {
				continue
			}
			state.Tasks[i].Status = TaskRunning
			state.Tasks[i].CurrentKey = currentKey
			state.Tasks[i].CompletedItems = completed
			state.Tasks[i].UpdatedAt = now
			if item.SourceKey != "" || item.TargetKey != "" || item.Status != "" {
				state.Tasks[i].Items = append(state.Tasks[i].Items, item)
			}
			return nil
		}
		return nil
	})
}

func (s *Service) FinishTask(id string, status TaskStatus, message string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	now := time.Now().UTC()
	_ = s.store.update(func(state *State) error {
		for i := range state.Tasks {
			if state.Tasks[i].ID == id {
				state.Tasks[i].Status = status
				state.Tasks[i].Message = message
				state.Tasks[i].UpdatedAt = now
				state.Tasks[i].FinishedAt = &now
				return nil
			}
		}
		return nil
	})
}

func (s *Service) RecordHistory(eventType, bucket, actor, status, message string, keys []string, metadata map[string]string) {
	entry := HistoryEntry{
		ID:        newID("history"),
		Type:      eventType,
		Bucket:    bucket,
		Actor:     actor,
		Keys:      append([]string(nil), keys...),
		Status:    status,
		Message:   message,
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	}
	_ = s.store.update(func(state *State) error {
		state.History = append([]HistoryEntry{entry}, state.History...)
		if len(state.History) > 500 {
			state.History = state.History[:500]
		}
		return nil
	})
}

func (s *Service) EmitEvent(evt Event) {
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	state := s.store.snapshot()
	for _, hook := range state.Webhooks {
		if !hook.Enabled || !contains(hook.Events, evt.Type) {
			continue
		}
		go s.deliverWebhook(hook, evt)
	}
}

func (s *Service) deliverWebhook(hook Webhook, evt Event) {
	payload := map[string]interface{}{
		"type":      evt.Type,
		"bucket":    evt.Bucket,
		"actor":     evt.Actor,
		"keys":      evt.Keys,
		"metadata":  evt.Metadata,
		"createdAt": evt.CreatedAt,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, hook.URL, bytes.NewReader(body))
	status := "success"
	statusCode := 0
	message := ""
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		if hook.Secret != "" {
			mac := hmac.New(sha256.New, []byte(hook.Secret))
			_, _ = mac.Write(body)
			req.Header.Set("X-S3C-Signature", hex.EncodeToString(mac.Sum(nil)))
		}
		resp, reqErr := s.httpClient.Do(req)
		if reqErr != nil {
			status = "failed"
			message = reqErr.Error()
		} else {
			statusCode = resp.StatusCode
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				status = "failed"
				message = resp.Status
			}
			_ = resp.Body.Close()
		}
	} else {
		status = "failed"
		message = err.Error()
	}
	delivery := WebhookDelivery{
		ID:         newID("delivery"),
		WebhookID:  hook.ID,
		Webhook:    hook.Name,
		Event:      evt.Type,
		Status:     status,
		StatusCode: statusCode,
		Error:      message,
		Payload:    payload,
		Attempted:  time.Now().UTC(),
	}
	_ = s.store.update(func(state *State) error {
		state.Deliveries = append([]WebhookDelivery{delivery}, state.Deliveries...)
		if len(state.Deliveries) > 500 {
			state.Deliveries = state.Deliveries[:500]
		}
		return nil
	})
}

func (s *Service) SearchObjects(ctx context.Context, req SearchRequest) ([]minio.ObjectInfo, error) {
	items := s.client.ListObjectsRecursive(ctx, req.Bucket, req.Prefix)
	result := make([]minio.ObjectInfo, 0, len(items))
	for _, obj := range items {
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}
		if req.Name != "" && !strings.Contains(strings.ToLower(path.Base(obj.Key)), strings.ToLower(req.Name)) {
			continue
		}
		if req.MinSize != nil && obj.Size < *req.MinSize {
			continue
		}
		if req.MaxSize != nil && obj.Size > *req.MaxSize {
			continue
		}
		if req.ModifiedAfter != nil && obj.LastModified.Before(*req.ModifiedAfter) {
			continue
		}
		if req.ModifiedBefore != nil && obj.LastModified.After(*req.ModifiedBefore) {
			continue
		}
		result = append(result, obj)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastModified.After(result[j].LastModified)
	})
	return result, nil
}

func (s *Service) BatchDelete(ctx context.Context, bucket, actor string, req BatchDeleteRequest) (string, error) {
	keys := normalizeKeys(req.Keys)
	if len(keys) == 0 {
		return "", fmt.Errorf("at least one key is required")
	}
	taskID := s.UpsertTask(req.TaskID, "batch-delete", bucket, "", actor, len(keys), nil)
	expanded, err := s.expandKeys(ctx, bucket, keys)
	if err != nil {
		s.FinishTask(taskID, TaskFailed, err.Error())
		return taskID, err
	}
	for idx, key := range expanded {
		if err := s.client.RemoveObject(ctx, bucket, key); err != nil {
			s.UpdateTaskProgress(taskID, key, idx, TaskItem{SourceKey: key, Status: "failed", Error: err.Error()})
			s.FinishTask(taskID, TaskFailed, err.Error())
			s.RecordHistory("object.delete", bucket, actor, "failed", err.Error(), expanded, nil)
			return taskID, err
		}
		s.UpdateTaskProgress(taskID, key, idx+1, TaskItem{SourceKey: key, Status: "deleted"})
	}
	s.FinishTask(taskID, TaskCompleted, fmt.Sprintf("deleted %d object(s)", len(expanded)))
	s.RecordHistory("object.delete", bucket, actor, "success", "batch delete completed", expanded, map[string]string{"taskId": taskID})
	s.EmitEvent(Event{Type: "object.deleted", Bucket: bucket, Actor: actor, Keys: expanded, Metadata: map[string]string{"taskId": taskID}})
	return taskID, nil
}

func (s *Service) BatchMove(ctx context.Context, bucket, actor string, req BatchMoveRequest) (string, error) {
	if len(req.Items) == 0 {
		return "", fmt.Errorf("at least one item is required")
	}
	taskID := s.UpsertTask(req.TaskID, "batch-move", bucket, "", actor, len(req.Items), nil)
	processed := make([]string, 0)
	for idx, item := range req.Items {
		if strings.TrimSpace(item.SourceKey) == "" || strings.TrimSpace(item.TargetKey) == "" {
			err := fmt.Errorf("sourceKey and targetKey are required")
			s.FinishTask(taskID, TaskFailed, err.Error())
			return taskID, err
		}
		moved, err := s.moveKey(ctx, bucket, item.SourceKey, item.TargetKey)
		if err != nil {
			s.UpdateTaskProgress(taskID, item.SourceKey, idx, TaskItem{SourceKey: item.SourceKey, TargetKey: item.TargetKey, Status: "failed", Error: err.Error()})
			s.FinishTask(taskID, TaskFailed, err.Error())
			s.RecordHistory("object.move", bucket, actor, "failed", err.Error(), processed, nil)
			return taskID, err
		}
		processed = append(processed, moved...)
		s.UpdateTaskProgress(taskID, item.SourceKey, idx+1, TaskItem{SourceKey: item.SourceKey, TargetKey: item.TargetKey, Status: "moved"})
	}
	s.FinishTask(taskID, TaskCompleted, fmt.Sprintf("moved %d item(s)", len(req.Items)))
	s.RecordHistory("object.move", bucket, actor, "success", "batch move completed", processed, map[string]string{"taskId": taskID})
	s.EmitEvent(Event{Type: "object.moved", Bucket: bucket, Actor: actor, Keys: processed, Metadata: map[string]string{"taskId": taskID}})
	return taskID, nil
}

func (s *Service) BatchRename(ctx context.Context, bucket, actor string, req BatchRenameRequest) (string, error) {
	if len(req.Items) == 0 {
		return "", fmt.Errorf("at least one item is required")
	}
	moveReq := BatchMoveRequest{TaskID: req.TaskID, Items: make([]BatchMoveItem, 0, len(req.Items))}
	for _, item := range req.Items {
		if strings.TrimSpace(item.SourceKey) == "" || strings.TrimSpace(item.NewName) == "" {
			return "", fmt.Errorf("sourceKey and newName are required")
		}
		dir := path.Dir(strings.TrimSuffix(item.SourceKey, "/"))
		if dir == "." {
			dir = ""
		}
		target := item.NewName
		if dir != "" {
			target = dir + "/" + item.NewName
		}
		if strings.HasSuffix(item.SourceKey, "/") {
			target = strings.TrimSuffix(target, "/") + "/"
		}
		moveReq.Items = append(moveReq.Items, BatchMoveItem{SourceKey: item.SourceKey, TargetKey: target})
	}
	taskID, err := s.BatchMove(ctx, bucket, actor, moveReq)
	if err == nil {
		s.RecordHistory("object.rename", bucket, actor, "success", "batch rename completed", nil, map[string]string{"taskId": taskID})
		s.EmitEvent(Event{Type: "object.renamed", Bucket: bucket, Actor: actor, Metadata: map[string]string{"taskId": taskID}})
	}
	return taskID, err
}

func (s *Service) StreamZip(ctx context.Context, bucket string, keys []string, w io.Writer) error {
	keys = normalizeKeys(keys)
	if len(keys) == 0 {
		return fmt.Errorf("at least one key is required")
	}
	expanded, err := s.expandKeys(ctx, bucket, keys)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, key := range expanded {
		obj, err := s.client.GetObject(ctx, bucket, key)
		if err != nil {
			return err
		}
		info, err := obj.Stat()
		if err != nil {
			_ = obj.Close()
			return err
		}
		name := strings.TrimLeft(key, "/")
		if name == "" {
			name = path.Base(key)
		}
		header, err := zip.FileInfoHeader(objectFileInfo{key: path.Base(name), size: info.Size, modified: info.LastModified})
		if err != nil {
			_ = obj.Close()
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = obj.Close()
			return err
		}
		if _, err := io.Copy(writer, obj); err != nil {
			_ = obj.Close()
			return err
		}
		_ = obj.Close()
	}
	return nil
}

func (s *Service) CreatePolicy(policy CleanupPolicy) CleanupPolicy {
	now := time.Now().UTC()
	policy.ID = newID("policy")
	policy.CreatedAt = now
	policy.UpdatedAt = now
	_ = s.store.update(func(state *State) error {
		state.Policies = append([]CleanupPolicy{policy}, state.Policies...)
		return nil
	})
	return policy
}

func (s *Service) UpdatePolicy(id string, policy CleanupPolicy) (CleanupPolicy, error) {
	policy.ID = id
	var out CleanupPolicy
	err := s.store.update(func(state *State) error {
		for i := range state.Policies {
			if state.Policies[i].ID == id {
				policy.CreatedAt = state.Policies[i].CreatedAt
				policy.UpdatedAt = time.Now().UTC()
				policy.LastRunAt = state.Policies[i].LastRunAt
				state.Policies[i] = policy
				out = policy
				return nil
			}
		}
		return fmt.Errorf("policy not found")
	})
	return out, err
}

func (s *Service) DeletePolicy(id string) error {
	return s.store.update(func(state *State) error {
		for i := range state.Policies {
			if state.Policies[i].ID == id {
				state.Policies = append(state.Policies[:i], state.Policies[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("policy not found")
	})
}

func (s *Service) RunPolicy(ctx context.Context, id, actor string) ([]string, error) {
	state := s.store.snapshot()
	var policy *CleanupPolicy
	for i := range state.Policies {
		if state.Policies[i].ID == id {
			p := state.Policies[i]
			policy = &p
			break
		}
	}
	if policy == nil {
		return nil, fmt.Errorf("policy not found")
	}
	objects, err := s.SearchObjects(ctx, SearchRequest{Bucket: policy.Bucket, Prefix: policy.Prefix, Name: policy.NameContains})
	if err != nil {
		return nil, err
	}
	filtered := make([]minio.ObjectInfo, 0, len(objects))
	for _, obj := range objects {
		if policy.MinSize > 0 && obj.Size < policy.MinSize {
			continue
		}
		if policy.MaxSize > 0 && obj.Size > policy.MaxSize {
			continue
		}
		if policy.OlderThanDays > 0 && obj.LastModified.After(time.Now().Add(-time.Duration(policy.OlderThanDays)*24*time.Hour)) {
			continue
		}
		filtered = append(filtered, obj)
	}
	if policy.KeepLatest > 0 && len(filtered) > policy.KeepLatest {
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].LastModified.After(filtered[j].LastModified) })
		filtered = filtered[policy.KeepLatest:]
	}
	deleted := make([]string, 0, len(filtered))
	for _, obj := range filtered {
		if err := s.client.RemoveObject(ctx, policy.Bucket, obj.Key); err != nil {
			return deleted, err
		}
		deleted = append(deleted, obj.Key)
	}
	now := time.Now().UTC()
	_ = s.store.update(func(state *State) error {
		for i := range state.Policies {
			if state.Policies[i].ID == id {
				state.Policies[i].LastRunAt = &now
				state.Policies[i].UpdatedAt = now
				return nil
			}
		}
		return nil
	})
	s.RecordHistory("cleanup.run", policy.Bucket, actor, "success", fmt.Sprintf("deleted %d object(s)", len(deleted)), deleted, map[string]string{"policyId": id})
	s.EmitEvent(Event{Type: "cleanup.completed", Bucket: policy.Bucket, Actor: actor, Keys: deleted, Metadata: map[string]string{"policyId": id}})
	return deleted, nil
}

func (s *Service) StartCleanupScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := s.store.snapshot()
				for _, policy := range state.Policies {
					if !policy.Enabled {
						continue
					}
					_, _ = s.RunPolicy(context.Background(), policy.ID, "system")
				}
			}
		}
	}()
}

func (s *Service) CreateWebhook(hook Webhook) Webhook {
	now := time.Now().UTC()
	hook.ID = newID("webhook")
	hook.CreatedAt = now
	hook.UpdatedAt = now
	_ = s.store.update(func(state *State) error {
		state.Webhooks = append([]Webhook{hook}, state.Webhooks...)
		return nil
	})
	return hook
}

func (s *Service) UpdateWebhook(id string, hook Webhook) (Webhook, error) {
	hook.ID = id
	var out Webhook
	err := s.store.update(func(state *State) error {
		for i := range state.Webhooks {
			if state.Webhooks[i].ID == id {
				hook.CreatedAt = state.Webhooks[i].CreatedAt
				hook.UpdatedAt = time.Now().UTC()
				if hook.Secret == "" {
					hook.Secret = state.Webhooks[i].Secret
				}
				state.Webhooks[i] = hook
				out = hook
				return nil
			}
		}
		return fmt.Errorf("webhook not found")
	})
	return out, err
}

func (s *Service) DeleteWebhook(id string) error {
	return s.store.update(func(state *State) error {
		for i := range state.Webhooks {
			if state.Webhooks[i].ID == id {
				state.Webhooks = append(state.Webhooks[:i], state.Webhooks[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("webhook not found")
	})
}

func (s *Service) expandKeys(ctx context.Context, bucket string, keys []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, key := range keys {
		if strings.HasSuffix(key, "/") {
			items := s.client.ListObjectsRecursive(ctx, bucket, key)
			for _, obj := range items {
				if obj.Err != nil || strings.HasSuffix(obj.Key, "/") {
					continue
				}
				if _, ok := seen[obj.Key]; ok {
					continue
				}
				seen[obj.Key] = struct{}{}
				result = append(result, obj.Key)
			}
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result, nil
}

func (s *Service) moveKey(ctx context.Context, bucket, sourceKey, targetKey string) ([]string, error) {
	if sourceKey == targetKey {
		return nil, nil
	}
	if strings.HasSuffix(sourceKey, "/") {
		items := s.client.ListObjectsRecursive(ctx, bucket, sourceKey)
		moved := make([]string, 0, len(items))
		for _, obj := range items {
			if obj.Err != nil || strings.HasSuffix(obj.Key, "/") {
				continue
			}
			rel := strings.TrimPrefix(obj.Key, sourceKey)
			newKey := strings.TrimSuffix(targetKey, "/") + "/" + rel
			if err := s.client.CopyObject(ctx, bucket, obj.Key, newKey); err != nil {
				return moved, err
			}
			if err := s.client.RemoveObject(ctx, bucket, obj.Key); err != nil {
				return moved, err
			}
			moved = append(moved, newKey)
		}
		return moved, nil
	}
	if err := s.client.CopyObject(ctx, bucket, sourceKey, targetKey); err != nil {
		return nil, err
	}
	if err := s.client.RemoveObject(ctx, bucket, sourceKey); err != nil {
		return nil, err
	}
	return []string{targetKey}, nil
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func normalizeKeys(keys []string) []string {
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(strings.TrimPrefix(key, "/"))
		if key == "" {
			continue
		}
		result = append(result, key)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type objectFileInfo struct {
	key      string
	size     int64
	modified time.Time
}

func (o objectFileInfo) Name() string       { return o.key }
func (o objectFileInfo) Size() int64        { return o.size }
func (o objectFileInfo) Mode() os.FileMode  { return 0o644 }
func (o objectFileInfo) ModTime() time.Time { return o.modified }
func (o objectFileInfo) IsDir() bool        { return false }
func (o objectFileInfo) Sys() interface{}   { return nil }
