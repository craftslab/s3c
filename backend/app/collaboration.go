package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrCollaborationSessionNotFound    = errors.New("collaboration session not found")
	ErrCollaborationAccessDenied       = errors.New("collaboration access denied")
	ErrCollaborationSessionClosed      = errors.New("collaboration session is closed")
	ErrCollaborationSessionExpired     = errors.New("collaboration session has expired")
	ErrInvalidCollaborationExpiry      = errors.New("collaboration session expiry must be in the future")
	ErrInvalidCollaborationTitle       = errors.New("collaboration title is required")
	ErrInvalidCollaborationBucket      = errors.New("collaboration bucket is required")
	ErrCollaborationMessageEmpty       = errors.New("message content is required")
	ErrCollaborationMessageNotFound    = errors.New("collaboration message not found")
	ErrCollaborationAttachmentNotFound = errors.New("collaboration attachment not found")
	ErrCollaborationFileNotFound       = errors.New("collaboration shared file not found")
	ErrCollaborationManageDenied       = errors.New("collaboration session can only be managed by the creator or an admin")
	ErrInvalidCollaborationSignal      = errors.New("collaboration signal payload is required")
	ErrInvalidCollaborationMention     = errors.New("mentioned user is not part of the collaboration session")
	ErrInvalidCollaborationReaction    = errors.New("reaction emoji is required")
	ErrInvalidCollaborationExport      = errors.New("collaboration export format must be json, txt, or pdf")
	ErrCollaborationMessageAuthorOnly  = errors.New("only the original sender can update this message state")
)

const (
	CollaborationSessionActive CollaborationSessionStatus = "active"
	CollaborationSessionClosed CollaborationSessionStatus = "closed"
	maxCollaborationMessages   int                        = 500
)

type CollaborationSessionStatus string
type CollaborationMessageType string
type CollaborationMessageStatus string

const (
	CollaborationMessageTypeMarkdown   CollaborationMessageType   = "markdown"
	CollaborationMessageTypeQuickReply CollaborationMessageType   = "quick_reply"
	CollaborationMessageStatusSent     CollaborationMessageStatus = "sent"
	CollaborationMessageStatusRecalled CollaborationMessageStatus = "recalled"
)

type CollaborationSession struct {
	ID               string                     `json:"id"`
	Token            string                     `json:"token"`
	Title            string                     `json:"title"`
	Creator          string                     `json:"creator"`
	Bucket           string                     `json:"bucket"`
	Prefix           string                     `json:"prefix,omitempty"`
	AttachmentBucket string                     `json:"attachmentBucket"`
	AttachmentPrefix string                     `json:"attachmentPrefix"`
	AllowedUsers     []string                   `json:"allowedUsers,omitempty"`
	Status           CollaborationSessionStatus `json:"status"`
	Messages         []CollaborationMessage     `json:"messages,omitempty"`
	ReadStates       []CollaborationReadState   `json:"readStates,omitempty"`
	Attachments      []CollaborationAttachment  `json:"attachments,omitempty"`
	SharedFiles      []CollaborationFileRef     `json:"sharedFiles,omitempty"`
	CreatedAt        time.Time                  `json:"createdAt"`
	UpdatedAt        time.Time                  `json:"updatedAt"`
	ExpiresAt        *time.Time                 `json:"expiresAt,omitempty"`
	ClosedAt         *time.Time                 `json:"closedAt,omitempty"`
}

type CollaborationMessage struct {
	ID             string                     `json:"id"`
	Type           CollaborationMessageType   `json:"type"`
	Status         CollaborationMessageStatus `json:"status"`
	Author         string                     `json:"author"`
	Content        string                     `json:"content"`
	Summary        string                     `json:"summary"`
	Mentions       []string                   `json:"mentions,omitempty"`
	ReplyTo        *CollaborationMessageRef   `json:"replyTo,omitempty"`
	QuickReply     string                     `json:"quickReply,omitempty"`
	Reactions      []CollaborationReaction    `json:"reactions,omitempty"`
	ReadBy         []CollaborationMessageRead `json:"readBy,omitempty"`
	DeletedFor     []string                   `json:"-"`
	CreatedAt      time.Time                  `json:"createdAt"`
	UpdatedAt      time.Time                  `json:"updatedAt"`
	RecalledAt     *time.Time                 `json:"recalledAt,omitempty"`
	RecalledBy     string                     `json:"recalledBy,omitempty"`
	ExportMetadata map[string]string          `json:"exportMetadata,omitempty"`
}

type CollaborationMessageRef struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type CollaborationReaction struct {
	Emoji string   `json:"emoji"`
	Users []string `json:"users,omitempty"`
}

type CollaborationMessageRead struct {
	Username string    `json:"username"`
	ReadAt   time.Time `json:"readAt"`
}

type CollaborationReadState struct {
	Username          string    `json:"username"`
	LastReadMessageID string    `json:"lastReadMessageId,omitempty"`
	LastReadAt        time.Time `json:"lastReadAt"`
}

type CollaborationMessageInput struct {
	Content        string
	ReplyToID      string
	QuickReply     string
	MentionedUsers []string
	Type           CollaborationMessageType
}

type CollaborationActor struct {
	Username        string
	Admin           bool
	BypassAllowList bool
}

type CollaborationAttachment struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Bucket      string    `json:"bucket"`
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	UploadedBy  string    `json:"uploadedBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CollaborationFileRef struct {
	ID        string    `json:"id"`
	Bucket    string    `json:"bucket"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	AddedBy   string    `json:"addedBy"`
	CreatedAt time.Time `json:"createdAt"`
}

type CollaborationRealtimeEvent struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
}

type CollaborationReadEvent struct {
	Username          string    `json:"username"`
	LastReadMessageID string    `json:"lastReadMessageId,omitempty"`
	UnreadCount       int       `json:"unreadCount"`
	ReadAt            time.Time `json:"readAt"`
}

type CollaborationDeletionEvent struct {
	MessageID string `json:"messageId"`
	Username  string `json:"username"`
}

type CollaborationExportSnapshot struct {
	Session     CollaborationSession  `json:"session"`
	Messages    []CollaborationMessage `json:"messages"`
	Attachments []CollaborationAttachment `json:"attachments"`
	SharedFiles []CollaborationFileRef `json:"sharedFiles"`
	ExportedAt  time.Time `json:"exportedAt"`
	ExportedBy  string `json:"exportedBy"`
}

var mentionPattern = regexp.MustCompile(`(^|[\s\(\[\{>])@([A-Za-z0-9._-]{3,64})`)

type streamAccessToken struct {
	SessionToken string
	Username     string
	ExpiresAt    time.Time
}

type collaborationHub struct {
	mu           sync.Mutex
	subscribers  map[string]map[string]chan CollaborationRealtimeEvent
	onlineCounts map[string]map[string]int
	streamTokens map[string]streamAccessToken
}

func newCollaborationHub() *collaborationHub {
	return &collaborationHub{
		subscribers:  make(map[string]map[string]chan CollaborationRealtimeEvent),
		onlineCounts: make(map[string]map[string]int),
		streamTokens: make(map[string]streamAccessToken),
	}
}

func (h *collaborationHub) issueStreamToken(sessionToken, username string) (string, error) {
	token, err := randomString(40, "abcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.streamTokens[token] = streamAccessToken{
		SessionToken: sessionToken,
		Username:     username,
		ExpiresAt:    time.Now().UTC().Add(15 * time.Minute),
	}
	return token, nil
}

func (h *collaborationHub) validateStreamToken(sessionToken, token string) (string, error) {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, item := range h.streamTokens {
		if !item.ExpiresAt.After(now) {
			delete(h.streamTokens, key)
		}
	}
	item, ok := h.streamTokens[token]
	if !ok || item.SessionToken != sessionToken || !item.ExpiresAt.After(now) {
		return "", ErrUnauthorized
	}
	return item.Username, nil
}

func (h *collaborationHub) subscribe(sessionToken, username string) (<-chan CollaborationRealtimeEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[sessionToken]; !ok {
		h.subscribers[sessionToken] = make(map[string]chan CollaborationRealtimeEvent)
	}
	if _, ok := h.onlineCounts[sessionToken]; !ok {
		h.onlineCounts[sessionToken] = make(map[string]int)
	}
	subscriberID := newID("collab-sub")
	ch := make(chan CollaborationRealtimeEvent, 32)
	h.subscribers[sessionToken][subscriberID] = ch
	h.onlineCounts[sessionToken][username]++
	online := sortedKeys(h.onlineCounts[sessionToken])
	h.publishLocked(sessionToken, CollaborationRealtimeEvent{
		Type:      "presence",
		Payload:   map[string]interface{}{"onlineUsers": online},
		CreatedAt: time.Now().UTC(),
	})
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.subscribers[sessionToken]; ok {
			if current, exists := subs[subscriberID]; exists {
				delete(subs, subscriberID)
				close(current)
			}
			if len(subs) == 0 {
				delete(h.subscribers, sessionToken)
			}
		}
		if counts, ok := h.onlineCounts[sessionToken]; ok {
			if counts[username] <= 1 {
				delete(counts, username)
			} else {
				counts[username]--
			}
			online := sortedKeys(counts)
			h.publishLocked(sessionToken, CollaborationRealtimeEvent{
				Type:      "presence",
				Payload:   map[string]interface{}{"onlineUsers": online},
				CreatedAt: time.Now().UTC(),
			})
			if len(counts) == 0 {
				delete(h.onlineCounts, sessionToken)
			}
		}
	}
}

func (h *collaborationHub) onlineUsers(sessionToken string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), sortedKeys(h.onlineCounts[sessionToken])...)
}

func (h *collaborationHub) publish(sessionToken string, event CollaborationRealtimeEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publishLocked(sessionToken, event)
}

func (h *collaborationHub) publishLocked(sessionToken string, event CollaborationRealtimeEvent) {
	subs := h.subscribers[sessionToken]
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func sortedKeys(values map[string]int) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type CollaborationSessionUpdate struct {
	Title        string
	AllowedUsers []string
	ExpiresAt    *time.Time
}

func (s *Service) ListCollaborationSessions(user User) []CollaborationSession {
	state := s.store.snapshot()
	now := time.Now().UTC()
	actor := collaborationActorFromUser(user)
	sessions := make([]CollaborationSession, 0, len(state.CollaborationSessions))
	for _, session := range state.CollaborationSessions {
		if err := ensureCollaborationAccessibleActor(session, actor, now); err != nil {
			continue
		}
		sessions = append(sessions, collaborationSessionView(session, actor.Username))
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions
}

func (s *Service) CreateCollaborationSession(user User, title, bucket, prefix string, allowedUsers []string, expiresAt *time.Time) (CollaborationSession, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return CollaborationSession{}, ErrInvalidCollaborationTitle
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return CollaborationSession{}, ErrInvalidCollaborationBucket
	}
	cleanPrefix, err := normalizeCollaborationPrefix(prefix)
	if err != nil {
		return CollaborationSession{}, err
	}
	now := time.Now().UTC()
	var normalizedExpiry *time.Time
	if expiresAt != nil {
		expires := expiresAt.UTC()
		if !expires.After(now) {
			return CollaborationSession{}, ErrInvalidCollaborationExpiry
		}
		normalizedExpiry = &expires
	}
	token, err := randomString(24, "abcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		return CollaborationSession{}, err
	}
	session := CollaborationSession{
		ID:               newID("collaboration"),
		Token:            token,
		Title:            title,
		Creator:          user.Username,
		Bucket:           bucket,
		Prefix:           cleanPrefix,
		AttachmentBucket: bucket,
		AttachmentPrefix: fmt.Sprintf(".kipup/collaboration/%s/attachments/", token),
		AllowedUsers:     normalizeCollaborationUsers(allowedUsers, user.Username),
		Status:           CollaborationSessionActive,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        normalizedExpiry,
	}
	if err := s.store.update(func(state *State) error {
		pruneExpiredAccess(state, now)
		state.CollaborationSessions = append([]CollaborationSession{session}, state.CollaborationSessions...)
		return nil
	}); err != nil {
		return CollaborationSession{}, err
	}
	return sanitizeCollaborationSession(session), nil
}

func (s *Service) GetCollaborationSession(token string, user User) (CollaborationSession, []string, error) {
	state := s.store.snapshot()
	now := time.Now().UTC()
	actor := collaborationActorFromUser(user)
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return CollaborationSession{}, nil, err
	}
	if err := ensureCollaborationAccessibleActor(session, actor, now); err != nil {
		return CollaborationSession{}, nil, err
	}
	return collaborationSessionView(session, actor.Username), s.hub.onlineUsers(session.Token), nil
}

func (s *Service) UpdateCollaborationSession(token string, user User, update CollaborationSessionUpdate) (CollaborationSession, error) {
	now := time.Now().UTC()
	allowed := normalizeCollaborationUsers(update.AllowedUsers, user.Username)
	updatedTitle := strings.TrimSpace(update.Title)
	if updatedTitle == "" {
		return CollaborationSession{}, ErrInvalidCollaborationTitle
	}
	var normalizedExpiry *time.Time
	if update.ExpiresAt != nil {
		expires := update.ExpiresAt.UTC()
		if !expires.After(now) {
			return CollaborationSession{}, ErrInvalidCollaborationExpiry
		}
		normalizedExpiry = &expires
	}
	var updated CollaborationSession
	if err := s.store.update(func(state *State) error {
		pruneExpiredAccess(state, now)
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationManageable(*session, user, now); err != nil {
			return err
		}
		session.Title = updatedTitle
		session.AllowedUsers = allowed
		session.ExpiresAt = normalizedExpiry
		session.UpdatedAt = now
		updated = sanitizeCollaborationSession(*session)
		return nil
	}); err != nil {
		return CollaborationSession{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "session.updated", Payload: updated, CreatedAt: now})
	return updated, nil
}

func (s *Service) CloseCollaborationSession(token string, user User) (CollaborationSession, error) {
	now := time.Now().UTC()
	var updated CollaborationSession
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationManageable(*session, user, now); err != nil {
			return err
		}
		session.Status = CollaborationSessionClosed
		session.ClosedAt = &now
		session.UpdatedAt = now
		updated = sanitizeCollaborationSession(*session)
		return nil
	}); err != nil {
		return CollaborationSession{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "session.closed", Payload: updated, CreatedAt: now})
	return updated, nil
}

func (s *Service) DeleteCollaborationSession(token string, user User) (CollaborationSession, error) {
	now := time.Now().UTC()
	var deleted CollaborationSession
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := state.CollaborationSessions[index]
		if err := ensureCollaborationManageable(session, user, now); err != nil {
			return err
		}
		deleted = sanitizeCollaborationSession(session)
		state.CollaborationSessions = append(state.CollaborationSessions[:index], state.CollaborationSessions[index+1:]...)
		return nil
	}); err != nil {
		return CollaborationSession{}, err
	}
	if s.client != nil && deleted.AttachmentBucket != "" && deleted.AttachmentPrefix != "" {
		_ = s.client.RemoveObjectsWithPrefix(context.Background(), deleted.AttachmentBucket, deleted.AttachmentPrefix)
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "session.deleted", Payload: deleted, CreatedAt: now})
	return deleted, nil
}

func (s *Service) AddCollaborationMessage(token string, user User, input CollaborationMessageInput) (CollaborationMessage, error) {
	return s.addCollaborationMessage(token, collaborationActorFromUser(user), input)
}

func (s *Service) RegisterCollaborationAttachment(token string, user User, attachment CollaborationAttachment) (CollaborationAttachment, error) {
	now := time.Now().UTC()
	attachment.ID = newID("attachment")
	attachment.UploadedBy = user.Username
	attachment.CreatedAt = now
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessible(*session, user, now); err != nil {
			return err
		}
		session.Attachments = append([]CollaborationAttachment{attachment}, session.Attachments...)
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationAttachment{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "attachment.created", Payload: attachment, CreatedAt: now})
	return attachment, nil
}

func (s *Service) DeleteCollaborationAttachment(token, attachmentID string, user User) (CollaborationAttachment, error) {
	now := time.Now().UTC()
	var removed CollaborationAttachment
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessible(*session, user, now); err != nil {
			return err
		}
		attachmentIndex := -1
		for i, attachment := range session.Attachments {
			if attachment.ID == attachmentID {
				attachmentIndex = i
				removed = attachment
				break
			}
		}
		if attachmentIndex < 0 {
			return ErrCollaborationAttachmentNotFound
		}
		session.Attachments = append(session.Attachments[:attachmentIndex], session.Attachments[attachmentIndex+1:]...)
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationAttachment{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "attachment.deleted", Payload: removed, CreatedAt: now})
	return removed, nil
}

func (s *Service) GetCollaborationAttachment(token, attachmentID string, user User) (CollaborationAttachment, error) {
	state := s.store.snapshot()
	now := time.Now().UTC()
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return CollaborationAttachment{}, err
	}
	if err := ensureCollaborationAccessible(session, user, now); err != nil {
		return CollaborationAttachment{}, err
	}
	for _, attachment := range session.Attachments {
		if attachment.ID == attachmentID {
			return attachment, nil
		}
	}
	return CollaborationAttachment{}, ErrCollaborationAttachmentNotFound
}

func (s *Service) AddCollaborationFileRef(token string, user User, ref CollaborationFileRef) (CollaborationFileRef, error) {
	now := time.Now().UTC()
	ref.ID = newID("shared-file")
	ref.AddedBy = user.Username
	ref.CreatedAt = now
	if ref.Name == "" {
		ref.Name = path.Base(ref.Key)
	}
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationManageable(*session, user, now); err != nil {
			return err
		}
		session.SharedFiles = append([]CollaborationFileRef{ref}, session.SharedFiles...)
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationFileRef{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "shared-file.created", Payload: ref, CreatedAt: now})
	return ref, nil
}

func (s *Service) DeleteCollaborationFileRef(token, fileID string, user User) (CollaborationFileRef, error) {
	now := time.Now().UTC()
	var removed CollaborationFileRef
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationManageable(*session, user, now); err != nil {
			return err
		}
		fileIndex := -1
		for i, item := range session.SharedFiles {
			if item.ID == fileID {
				fileIndex = i
				removed = item
				break
			}
		}
		if fileIndex < 0 {
			return ErrCollaborationFileNotFound
		}
		session.SharedFiles = append(session.SharedFiles[:fileIndex], session.SharedFiles[fileIndex+1:]...)
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationFileRef{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "shared-file.deleted", Payload: removed, CreatedAt: now})
	return removed, nil
}

func (s *Service) GetCollaborationFileRef(token, fileID string, user User) (CollaborationFileRef, error) {
	state := s.store.snapshot()
	now := time.Now().UTC()
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return CollaborationFileRef{}, err
	}
	if err := ensureCollaborationAccessible(session, user, now); err != nil {
		return CollaborationFileRef{}, err
	}
	for _, item := range session.SharedFiles {
		if item.ID == fileID {
			return item, nil
		}
	}
	return CollaborationFileRef{}, ErrCollaborationFileNotFound
}

func (s *Service) IssueCollaborationStreamToken(token string, user User) (string, error) {
	state := s.store.snapshot()
	now := time.Now().UTC()
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return "", err
	}
	if err := ensureCollaborationAccessibleActor(session, collaborationActorFromUser(user), now); err != nil {
		return "", err
	}
	return s.hub.issueStreamToken(token, user.Username)
}

func (s *Service) SubscribeCollaboration(token, streamToken string) (<-chan CollaborationRealtimeEvent, func(), []string, error) {
	username, err := s.hub.validateStreamToken(token, streamToken)
	if err != nil {
		return nil, nil, nil, err
	}
	state := s.store.snapshot()
	user, ok := findActiveUser(state.Users, username, time.Now().UTC())
	if !ok {
		return nil, nil, nil, ErrUnauthorized
	}
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := ensureCollaborationAccessibleActor(session, collaborationActorFromUser(user), time.Now().UTC()); err != nil {
		return nil, nil, nil, err
	}
	ch, unsubscribe := s.hub.subscribe(token, username)
	return ch, unsubscribe, s.hub.onlineUsers(token), nil
}

func (s *Service) PublishCollaborationSignal(token string, user User, payload map[string]interface{}) error {
	if len(payload) == 0 {
		return ErrInvalidCollaborationSignal
	}
	state := s.store.snapshot()
	now := time.Now().UTC()
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return err
	}
	if err := ensureCollaborationAccessibleActor(session, collaborationActorFromUser(user), now); err != nil {
		return err
	}
	message := make(map[string]interface{}, len(payload)+1)
	for key, value := range payload {
		message[key] = value
	}
	message["from"] = user.Username
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "signal", Payload: message, CreatedAt: now})
	return nil
}

func (s *Service) AttachmentObjectKey(session CollaborationSession, filename string) (string, error) {
	name := strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	name = path.Base(name)
	if name == "." || name == "/" || name == "" {
		return "", errors.New("attachment filename is required")
	}
	suffix, err := randomString(8, "abcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d-%s-%s", session.AttachmentPrefix, time.Now().UTC().UnixNano(), suffix, name), nil
}

func findActiveUser(users []User, username string, now time.Time) (User, bool) {
	index := findUserIndex(users, username)
	if index < 0 {
		return User{}, false
	}
	user := users[index]
	if isUserExpired(user, now) {
		return User{}, false
	}
	return sanitizeUser(user), true
}

func findCollaborationSession(sessions []CollaborationSession, token string) (CollaborationSession, error) {
	index, err := findCollaborationSessionIndex(sessions, token)
	if err != nil {
		return CollaborationSession{}, err
	}
	return sessions[index], nil
}

func findCollaborationSessionIndex(sessions []CollaborationSession, token string) (int, error) {
	token = strings.TrimSpace(token)
	for i, session := range sessions {
		if session.Token == token {
			return i, nil
		}
	}
	return -1, ErrCollaborationSessionNotFound
}

func ensureCollaborationAccessible(session CollaborationSession, user User, now time.Time) error {
	return ensureCollaborationAccessibleActor(session, collaborationActorFromUser(user), now)
}

func ensureCollaborationManageable(session CollaborationSession, user User, now time.Time) error {
	return ensureCollaborationManageableActor(session, collaborationActorFromUser(user), now)
}

func canManageCollaboration(session CollaborationSession, user User) bool {
	return canManageCollaborationActor(session, collaborationActorFromUser(user))
}

func sanitizeCollaborationSession(session CollaborationSession) CollaborationSession {
	session.AllowedUsers = append([]string(nil), session.AllowedUsers...)
	if len(session.Messages) > 0 {
		messages := make([]CollaborationMessage, len(session.Messages))
		for i, message := range session.Messages {
			messages[i] = sanitizeCollaborationMessage(message)
		}
		session.Messages = messages
	}
	session.ReadStates = append([]CollaborationReadState(nil), session.ReadStates...)
	session.Attachments = append([]CollaborationAttachment(nil), session.Attachments...)
	session.SharedFiles = append([]CollaborationFileRef(nil), session.SharedFiles...)
	return session
}

func normalizeCollaborationUsers(input []string, creator string) []string {
	seen := map[string]struct{}{}
	users := make([]string, 0, len(input)+1)
	for _, value := range input {
		normalized := normalizedUsernameKey(value)
		if normalized == "" || sameUsername(normalized, creator) {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		users = append(users, normalized)
	}
	sort.Strings(users)
	return users
}

func normalizeCollaborationPrefix(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "", nil
	}
	segments := strings.Split(value, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." {
			continue
		}
		if segment == ".." {
			return "", errors.New("collaboration prefix cannot contain '..'")
		}
		cleaned = append(cleaned, segment)
	}
	if len(cleaned) == 0 {
		return "", nil
	}
	return strings.Join(cleaned, "/") + "/", nil
}
