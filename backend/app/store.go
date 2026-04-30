package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu   sync.RWMutex
	path string
	data State
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, data: State{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, &s.data)
}

func (s *Store) snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.data)
}

func (s *Store) update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func cloneState(in State) State {
	out := State{
		Tasks:                 cloneTasks(in.Tasks),
		History:               cloneHistory(in.History),
		Policies:              append([]CleanupPolicy(nil), in.Policies...),
		Webhooks:              cloneWebhooks(in.Webhooks),
		Deliveries:            cloneDeliveries(in.Deliveries),
		Users:                 cloneUsers(in.Users),
		Sessions:              append([]Session(nil), in.Sessions...),
		CollaborationSessions: cloneCollaborationSessions(in.CollaborationSessions),
	}
	return out
}

func cloneTasks(in []Task) []Task {
	if len(in) == 0 {
		return nil
	}
	out := make([]Task, len(in))
	for i, task := range in {
		out[i] = task
		out[i].Metadata = cloneStringMap(task.Metadata)
		out[i].Items = append([]TaskItem(nil), task.Items...)
	}
	return out
}

func cloneHistory(in []HistoryEntry) []HistoryEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]HistoryEntry, len(in))
	for i, entry := range in {
		out[i] = entry
		out[i].Keys = append([]string(nil), entry.Keys...)
		out[i].Metadata = cloneStringMap(entry.Metadata)
	}
	return out
}

func cloneWebhooks(in []Webhook) []Webhook {
	if len(in) == 0 {
		return nil
	}
	out := make([]Webhook, len(in))
	for i, hook := range in {
		out[i] = hook
		out[i].Events = append([]string(nil), hook.Events...)
	}
	return out
}

func cloneDeliveries(in []WebhookDelivery) []WebhookDelivery {
	if len(in) == 0 {
		return nil
	}
	out := make([]WebhookDelivery, len(in))
	for i, delivery := range in {
		out[i] = delivery
		out[i].Payload = cloneAnyMap(delivery.Payload)
	}
	return out
}

func cloneUsers(in []User) []User {
	if len(in) == 0 {
		return nil
	}
	out := make([]User, len(in))
	for i, user := range in {
		out[i] = user
		out[i].Permissions = clonePermissions(user.Permissions)
	}
	return out
}

func cloneCollaborationSessions(in []CollaborationSession) []CollaborationSession {
	if len(in) == 0 {
		return nil
	}
	out := make([]CollaborationSession, len(in))
	for i, session := range in {
		out[i] = session
		out[i].AllowedUsers = append([]string(nil), session.AllowedUsers...)
		out[i].Messages = append([]CollaborationMessage(nil), session.Messages...)
		out[i].Attachments = append([]CollaborationAttachment(nil), session.Attachments...)
		out[i].SharedFiles = append([]CollaborationFileRef(nil), session.SharedFiles...)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
