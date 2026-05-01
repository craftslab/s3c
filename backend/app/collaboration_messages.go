package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	markdownTokenPattern = regexp.MustCompile(`[*_~` + "`" + `>#-]`)
	deviceNamePattern    = regexp.MustCompile(`[^a-z0-9._-]+`)
)

const (
	maxCollaborationSummaryLength       = 120
	truncatedCollaborationSummaryLength = 117
)

func collaborationActorFromUser(user User) CollaborationActor {
	return CollaborationActor{
		Username: user.Username,
		Admin:    user.IsAdmin(),
	}
}

func collaborationSessionView(session CollaborationSession, username string) CollaborationSession {
	view := sanitizeCollaborationSession(session)
	view.Messages = visibleCollaborationMessages(view.Messages, username)
	return view
}

func visibleCollaborationMessages(messages []CollaborationMessage, username string) []CollaborationMessage {
	if len(messages) == 0 {
		return nil
	}
	visible := make([]CollaborationMessage, 0, len(messages))
	for _, message := range messages {
		if collaborationMessageHiddenFor(message, username) {
			continue
		}
		visible = append(visible, sanitizeCollaborationMessage(message))
	}
	return visible
}

func collaborationMessageHiddenFor(message CollaborationMessage, username string) bool {
	if message.Status == CollaborationMessageStatusRecalled {
		return true
	}
	return collaborationMessageDeletedFor(message, username)
}

func collaborationMessageDeletedFor(message CollaborationMessage, username string) bool {
	for _, current := range message.DeletedFor {
		if sameUsername(current, username) {
			return true
		}
	}
	return false
}

func sanitizeCollaborationMessage(message CollaborationMessage) CollaborationMessage {
	message.Mentions = append([]string(nil), message.Mentions...)
	if message.ReplyTo != nil {
		reply := *message.ReplyTo
		message.ReplyTo = &reply
	}
	if len(message.Reactions) > 0 {
		reactions := make([]CollaborationReaction, len(message.Reactions))
		for i, reaction := range message.Reactions {
			reactions[i] = reaction
			reactions[i].Users = append([]string(nil), reaction.Users...)
		}
		message.Reactions = reactions
	}
	message.ReadBy = append([]CollaborationMessageRead(nil), message.ReadBy...)
	message.DeletedFor = append([]string(nil), message.DeletedFor...)
	message.ExportMetadata = cloneStringMap(message.ExportMetadata)
	return message
}

func (s *Service) addCollaborationMessage(token string, actor CollaborationActor, input CollaborationMessageInput) (CollaborationMessage, error) {
	now := time.Now().UTC()
	var created CollaborationMessage
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessibleActor(*session, actor, now); err != nil {
			return err
		}
		messageType := input.Type
		if messageType == "" {
			if strings.TrimSpace(input.QuickReply) != "" {
				messageType = CollaborationMessageTypeQuickReply
			} else {
				messageType = CollaborationMessageTypeMarkdown
			}
		}
		content := sanitizeCollaborationMarkdown(input.Content)
		quickReply := strings.TrimSpace(input.QuickReply)
		if content == "" && quickReply == "" {
			return ErrCollaborationMessageEmpty
		}
		mentions, err := normalizeCollaborationMentions(*session, actor.Username, input.MentionedUsers, content)
		if err != nil {
			return err
		}
		replyTo, err := buildCollaborationReplyRef(session.Messages, input.ReplyToID, actor.Username)
		if err != nil {
			return err
		}
		created = CollaborationMessage{
			ID:         newID("message"),
			Type:       messageType,
			Status:     CollaborationMessageStatusSent,
			Author:     actor.Username,
			Content:    content,
			Summary:    summarizeCollaborationContent(content, quickReply),
			Mentions:   mentions,
			ReplyTo:    replyTo,
			QuickReply: quickReply,
			CreatedAt:  now,
			UpdatedAt:  now,
			ExportMetadata: map[string]string{
				"source": "chat",
			},
		}
		session.Messages = append(session.Messages, created)
		if len(session.Messages) > maxCollaborationMessages {
			session.Messages = session.Messages[len(session.Messages)-maxCollaborationMessages:]
			pruneCollaborationReadStates(session)
		}
		updateCollaborationReadState(session, actor.Username, created.ID, now)
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationMessage{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{
		Type:      "message.created",
		Payload:   sanitizeCollaborationMessage(created),
		CreatedAt: now,
	})
	return sanitizeCollaborationMessage(created), nil
}

func (s *Service) MarkCollaborationRead(token string, user User, messageID string) (CollaborationReadEvent, error) {
	return s.markCollaborationRead(token, collaborationActorFromUser(user), messageID)
}

func (s *Service) markCollaborationRead(token string, actor CollaborationActor, messageID string) (CollaborationReadEvent, error) {
	now := time.Now().UTC()
	event := CollaborationReadEvent{Username: actor.Username, ReadAt: now}
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessibleActor(*session, actor, now); err != nil {
			return err
		}
		targetID := strings.TrimSpace(messageID)
		if targetID == "" {
			targetID = latestVisibleMessageID(session.Messages, actor.Username)
		}
		if targetID != "" && findCollaborationMessageIndex(session.Messages, targetID) < 0 {
			return ErrCollaborationMessageNotFound
		}
		updateCollaborationReadState(session, actor.Username, targetID, now)
		session.UpdatedAt = now
		event.LastReadMessageID = targetID
		event.UnreadCount = collaborationUnreadCount(*session, actor.Username)
		return nil
	}); err != nil {
		return CollaborationReadEvent{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "read.updated", Payload: event, CreatedAt: now})
	return event, nil
}

func (s *Service) ToggleCollaborationReaction(token string, user User, messageID, emoji string) (CollaborationMessage, error) {
	return s.toggleCollaborationReaction(token, collaborationActorFromUser(user), messageID, emoji)
}

func (s *Service) toggleCollaborationReaction(token string, actor CollaborationActor, messageID, emoji string) (CollaborationMessage, error) {
	emoji = strings.TrimSpace(emoji)
	if emoji == "" {
		return CollaborationMessage{}, ErrInvalidCollaborationReaction
	}
	now := time.Now().UTC()
	var updated CollaborationMessage
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessibleActor(*session, actor, now); err != nil {
			return err
		}
		messageIndex := findCollaborationMessageIndex(session.Messages, messageID)
		if messageIndex < 0 {
			return ErrCollaborationMessageNotFound
		}
		message := &session.Messages[messageIndex]
		if collaborationMessageHiddenFor(*message, actor.Username) {
			return ErrCollaborationMessageNotFound
		}
		if message.Status == CollaborationMessageStatusRecalled {
			return ErrCollaborationMessageAuthorOnly
		}
		toggleCollaborationReactionUsers(message, emoji, actor.Username)
		message.UpdatedAt = now
		session.UpdatedAt = now
		updated = sanitizeCollaborationMessage(*message)
		return nil
	}); err != nil {
		return CollaborationMessage{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "reaction.changed", Payload: updated, CreatedAt: now})
	return updated, nil
}

func (s *Service) RecallCollaborationMessage(token string, user User, messageID string) (CollaborationMessage, error) {
	return s.recallCollaborationMessage(token, collaborationActorFromUser(user), messageID)
}

func (s *Service) recallCollaborationMessage(token string, actor CollaborationActor, messageID string) (CollaborationMessage, error) {
	now := time.Now().UTC()
	var updated CollaborationMessage
	event := CollaborationDeletionEvent{
		MessageID:     strings.TrimSpace(messageID),
		Username:      actor.Username,
		DeletedForAll: true,
	}
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessibleActor(*session, actor, now); err != nil {
			return err
		}
		messageIndex := findCollaborationMessageIndex(session.Messages, messageID)
		if messageIndex < 0 {
			return ErrCollaborationMessageNotFound
		}
		message := &session.Messages[messageIndex]
		if !sameUsername(message.Author, actor.Username) {
			return ErrCollaborationMessageAuthorOnly
		}
		updated = sanitizeCollaborationMessage(*message)
		updated.Status = CollaborationMessageStatusRecalled
		updated.Content = ""
		updated.Summary = "Message recalled"
		updated.Mentions = nil
		updated.Reactions = nil
		updated.QuickReply = ""
		updated.RecalledAt = &now
		updated.RecalledBy = actor.Username
		updated.UpdatedAt = now
		session.Messages = append(session.Messages[:messageIndex], session.Messages[messageIndex+1:]...)
		// Remove stale last-read pointers that may still reference the recalled message.
		pruneCollaborationReadStates(session)
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationMessage{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "message.deleted", Payload: event, CreatedAt: now})
	return updated, nil
}

func (s *Service) DeleteCollaborationMessage(token string, user User, messageID string) (CollaborationDeletionEvent, error) {
	return s.deleteCollaborationMessage(token, collaborationActorFromUser(user), messageID)
}

func (s *Service) deleteCollaborationMessage(token string, actor CollaborationActor, messageID string) (CollaborationDeletionEvent, error) {
	now := time.Now().UTC()
	event := CollaborationDeletionEvent{MessageID: strings.TrimSpace(messageID), Username: actor.Username}
	if err := s.store.update(func(state *State) error {
		index, err := findCollaborationSessionIndex(state.CollaborationSessions, token)
		if err != nil {
			return err
		}
		session := &state.CollaborationSessions[index]
		if err := ensureCollaborationAccessibleActor(*session, actor, now); err != nil {
			return err
		}
		messageIndex := findCollaborationMessageIndex(session.Messages, messageID)
		if messageIndex < 0 {
			return ErrCollaborationMessageNotFound
		}
		message := &session.Messages[messageIndex]
		if !sameUsername(message.Author, actor.Username) {
			return ErrCollaborationMessageAuthorOnly
		}
		if !collaborationMessageDeletedFor(*message, actor.Username) {
			message.DeletedFor = append(message.DeletedFor, actor.Username)
			sort.Strings(message.DeletedFor)
		}
		message.UpdatedAt = now
		session.UpdatedAt = now
		return nil
	}); err != nil {
		return CollaborationDeletionEvent{}, err
	}
	s.hub.publish(token, CollaborationRealtimeEvent{Type: "message.deleted", Payload: event, CreatedAt: now})
	return event, nil
}

func (s *Service) ExportCollaborationTranscript(token string, user User, format string) (string, string, []byte, error) {
	return s.exportCollaborationTranscript(token, collaborationActorFromUser(user), format)
}

func (s *Service) exportCollaborationTranscript(token string, actor CollaborationActor, format string) (string, string, []byte, error) {
	state := s.store.snapshot()
	now := time.Now().UTC()
	session, err := findCollaborationSession(state.CollaborationSessions, token)
	if err != nil {
		return "", "", nil, err
	}
	if err := ensureCollaborationAccessibleActor(session, actor, now); err != nil {
		return "", "", nil, err
	}
	snapshot := CollaborationExportSnapshot{
		Session:     collaborationSessionView(session, actor.Username),
		Messages:    visibleCollaborationMessages(session.Messages, actor.Username),
		Attachments: append([]CollaborationAttachment(nil), session.Attachments...),
		SharedFiles: append([]CollaborationFileRef(nil), session.SharedFiles...),
		ExportedAt:  now,
		ExportedBy:  actor.Username,
	}
	baseName := sanitizeExportFileName(session.Title)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		data, err := json.MarshalIndent(snapshot, "", "  ")
		return baseName + ".json", "application/json", data, err
	case "txt":
		data := buildCollaborationTXT(snapshot)
		return baseName + ".txt", "text/plain; charset=utf-8", data, nil
	case "pdf":
		data := buildCollaborationPDF(snapshot)
		return baseName + ".pdf", "application/pdf", data, nil
	default:
		return "", "", nil, ErrInvalidCollaborationExport
	}
}

func normalizeCollaborationMentions(session CollaborationSession, username string, explicit []string, content string) ([]string, error) {
	allowed := make(map[string]string)
	for _, item := range CollaborationMentionableUsers(session) {
		allowed[normalizedUsernameKey(item)] = item
	}
	selected := map[string]string{}
	for _, item := range explicit {
		key := normalizedUsernameKey(item)
		if key == "" {
			continue
		}
		value, ok := allowed[key]
		if !ok && !sameUsername(item, username) {
			return nil, ErrInvalidCollaborationMention
		}
		if sameUsername(item, username) {
			value = username
		}
		selected[key] = value
	}
	for _, match := range mentionPattern.FindAllStringSubmatch(content, -1) {
		key := normalizedUsernameKey(match[2])
		if key == "" {
			continue
		}
		value, ok := allowed[key]
		if !ok && !sameUsername(match[2], username) {
			return nil, ErrInvalidCollaborationMention
		}
		if sameUsername(match[2], username) {
			value = username
		}
		selected[key] = value
	}
	if len(selected) == 0 {
		return nil, nil
	}
	mentions := make([]string, 0, len(selected))
	for _, value := range selected {
		mentions = append(mentions, value)
	}
	sort.Slice(mentions, func(i, j int) bool {
		return normalizedUsernameKey(mentions[i]) < normalizedUsernameKey(mentions[j])
	})
	return mentions, nil
}

func buildCollaborationReplyRef(messages []CollaborationMessage, replyToID, username string) (*CollaborationMessageRef, error) {
	replyToID = strings.TrimSpace(replyToID)
	if replyToID == "" {
		return nil, nil
	}
	index := findCollaborationMessageIndex(messages, replyToID)
	if index < 0 {
		return nil, ErrCollaborationMessageNotFound
	}
	message := messages[index]
	if collaborationMessageHiddenFor(message, username) {
		return nil, ErrCollaborationMessageNotFound
	}
	return &CollaborationMessageRef{
		ID:        message.ID,
		Author:    message.Author,
		Summary:   message.Summary,
		CreatedAt: message.CreatedAt,
	}, nil
}

func summarizeCollaborationContent(content, quickReply string) string {
	summary := strings.TrimSpace(content)
	if summary == "" {
		summary = quickReply
	}
	summary = markdownTokenPattern.ReplaceAllString(summary, "")
	summary = strings.Join(strings.Fields(summary), " ")
	runes := []rune(summary)
	if len(runes) > maxCollaborationSummaryLength {
		summary = string(runes[:truncatedCollaborationSummaryLength]) + "..."
	}
	return summary
}

func sanitizeCollaborationMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSpace(html.EscapeString(value))
}

func CollaborationMentionableUsers(session CollaborationSession) []string {
	seen := map[string]string{}
	for _, item := range append([]string{session.Creator}, session.AllowedUsers...) {
		if key := normalizedUsernameKey(item); key != "" {
			seen[key] = item
		}
	}
	for _, message := range session.Messages {
		if key := normalizedUsernameKey(message.Author); key != "" {
			seen[key] = message.Author
		}
	}
	users := make([]string, 0, len(seen))
	for _, value := range seen {
		users = append(users, value)
	}
	sort.Slice(users, func(i, j int) bool {
		return normalizedUsernameKey(users[i]) < normalizedUsernameKey(users[j])
	})
	return users
}

func findCollaborationMessageIndex(messages []CollaborationMessage, messageID string) int {
	for i, message := range messages {
		if message.ID == strings.TrimSpace(messageID) {
			return i
		}
	}
	return -1
}

func latestVisibleMessageID(messages []CollaborationMessage, username string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if !collaborationMessageHiddenFor(messages[i], username) {
			return messages[i].ID
		}
	}
	return ""
}

func updateCollaborationReadState(session *CollaborationSession, username, messageID string, readAt time.Time) {
	if username == "" {
		return
	}
	for i := range session.ReadStates {
		if sameUsername(session.ReadStates[i].Username, username) {
			session.ReadStates[i].Username = username
			session.ReadStates[i].LastReadMessageID = messageID
			session.ReadStates[i].LastReadAt = readAt
			markMessageReads(session.Messages, username, messageID, readAt)
			return
		}
	}
	session.ReadStates = append(session.ReadStates, CollaborationReadState{
		Username:          username,
		LastReadMessageID: messageID,
		LastReadAt:        readAt,
	})
	markMessageReads(session.Messages, username, messageID, readAt)
}

func markMessageReads(messages []CollaborationMessage, username, messageID string, readAt time.Time) {
	if messageID == "" {
		return
	}
	for i := range messages {
		updateSingleMessageRead(&messages[i], username, readAt)
		if messages[i].ID == messageID {
			return
		}
	}
}

func updateSingleMessageRead(message *CollaborationMessage, username string, readAt time.Time) {
	for i := range message.ReadBy {
		if sameUsername(message.ReadBy[i].Username, username) {
			message.ReadBy[i].Username = username
			message.ReadBy[i].ReadAt = readAt
			return
		}
	}
	message.ReadBy = append(message.ReadBy, CollaborationMessageRead{Username: username, ReadAt: readAt})
	sort.Slice(message.ReadBy, func(i, j int) bool {
		return normalizedUsernameKey(message.ReadBy[i].Username) < normalizedUsernameKey(message.ReadBy[j].Username)
	})
}

func collaborationUnreadCount(session CollaborationSession, username string) int {
	count, _ := CollaborationUnreadState(session, username)
	return count
}

func CollaborationUnreadState(session CollaborationSession, username string) (int, string) {
	lastRead := ""
	for _, state := range session.ReadStates {
		if sameUsername(state.Username, username) {
			lastRead = state.LastReadMessageID
			break
		}
	}
	if lastRead == "" {
		count := 0
		for _, message := range session.Messages {
			if !collaborationMessageHiddenFor(message, username) && !sameUsername(message.Author, username) {
				count++
			}
		}
		return count, lastRead
	}
	count := 0
	seenLast := false
	for _, message := range session.Messages {
		if message.ID == lastRead {
			seenLast = true
			continue
		}
		if !seenLast || collaborationMessageHiddenFor(message, username) || sameUsername(message.Author, username) {
			continue
		}
		count++
	}
	return count, lastRead
}

func pruneCollaborationReadStates(session *CollaborationSession) {
	if len(session.ReadStates) == 0 || len(session.Messages) == 0 {
		return
	}
	validIDs := make(map[string]struct{}, len(session.Messages))
	oldestID := session.Messages[0].ID
	for _, message := range session.Messages {
		validIDs[message.ID] = struct{}{}
	}
	for i := range session.ReadStates {
		lastReadID := session.ReadStates[i].LastReadMessageID
		if lastReadID == "" {
			continue
		}
		if _, ok := validIDs[lastReadID]; !ok {
			session.ReadStates[i].LastReadMessageID = oldestID
		}
	}
}

func toggleCollaborationReactionUsers(message *CollaborationMessage, emoji, username string) {
	for i := range message.Reactions {
		if message.Reactions[i].Emoji != emoji {
			continue
		}
		for j, current := range message.Reactions[i].Users {
			if sameUsername(current, username) {
				message.Reactions[i].Users = append(message.Reactions[i].Users[:j], message.Reactions[i].Users[j+1:]...)
				if len(message.Reactions[i].Users) == 0 {
					message.Reactions = append(message.Reactions[:i], message.Reactions[i+1:]...)
				}
				return
			}
		}
		message.Reactions[i].Users = append(message.Reactions[i].Users, username)
		sort.Strings(message.Reactions[i].Users)
		return
	}
	message.Reactions = append(message.Reactions, CollaborationReaction{Emoji: emoji, Users: []string{username}})
	sort.Slice(message.Reactions, func(i, j int) bool {
		return message.Reactions[i].Emoji < message.Reactions[j].Emoji
	})
}

func ensureCollaborationAccessibleActor(session CollaborationSession, actor CollaborationActor, now time.Time) error {
	if session.Status != CollaborationSessionActive {
		return ErrCollaborationSessionClosed
	}
	if session.ExpiresAt != nil && !session.ExpiresAt.After(now) {
		return ErrCollaborationSessionExpired
	}
	if canManageCollaborationActor(session, actor) || actor.BypassAllowList {
		return nil
	}
	for _, allowed := range session.AllowedUsers {
		if sameUsername(allowed, actor.Username) {
			return nil
		}
	}
	return ErrCollaborationAccessDenied
}

func ensureCollaborationManageableActor(session CollaborationSession, actor CollaborationActor, now time.Time) error {
	if session.ExpiresAt != nil && !session.ExpiresAt.After(now) {
		return ErrCollaborationSessionExpired
	}
	if session.Status != CollaborationSessionActive {
		return ErrCollaborationSessionClosed
	}
	if canManageCollaborationActor(session, actor) {
		return nil
	}
	return ErrCollaborationManageDenied
}

func canManageCollaborationActor(session CollaborationSession, actor CollaborationActor) bool {
	return actor.Admin || sameUsername(session.Creator, actor.Username)
}

func sanitizeExportFileName(value string) string {
	value = normalizedUsernameKey(strings.ReplaceAll(value, " ", "-"))
	if value == "" {
		return "collaboration-transcript"
	}
	return value
}

func buildCollaborationTXT(snapshot CollaborationExportSnapshot) []byte {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Session: %s\n", snapshot.Session.Title))
	builder.WriteString(fmt.Sprintf("Bucket: %s\n", snapshot.Session.Bucket))
	if snapshot.Session.Prefix != "" {
		builder.WriteString(fmt.Sprintf("Prefix: %s\n", snapshot.Session.Prefix))
	}
	builder.WriteString(fmt.Sprintf("Exported by: %s\n", snapshot.ExportedBy))
	builder.WriteString(fmt.Sprintf("Exported at: %s\n\n", snapshot.ExportedAt.Format(time.RFC3339)))
	for _, message := range snapshot.Messages {
		builder.WriteString(fmt.Sprintf("[%s] %s", message.CreatedAt.Format(time.RFC3339), message.Author))
		if len(message.Mentions) > 0 {
			builder.WriteString(fmt.Sprintf(" mentions %s", strings.Join(message.Mentions, ", ")))
		}
		builder.WriteString("\n")
		if message.ReplyTo != nil {
			builder.WriteString(fmt.Sprintf("> reply to %s: %s\n", message.ReplyTo.Author, message.ReplyTo.Summary))
		}
		if message.Status == CollaborationMessageStatusRecalled {
			builder.WriteString("(message recalled)\n\n")
			continue
		}
		if message.QuickReply != "" {
			builder.WriteString(fmt.Sprintf("[quick reply] %s\n", message.QuickReply))
		}
		builder.WriteString(message.Content)
		builder.WriteString("\n")
		if len(message.Reactions) > 0 {
			parts := make([]string, 0, len(message.Reactions))
			for _, reaction := range message.Reactions {
				parts = append(parts, fmt.Sprintf("%s(%d)", reaction.Emoji, len(reaction.Users)))
			}
			builder.WriteString(fmt.Sprintf("Reactions: %s\n", strings.Join(parts, ", ")))
		}
		builder.WriteString("\n")
	}
	if len(snapshot.Attachments) > 0 {
		builder.WriteString("Attachments:\n")
		for _, item := range snapshot.Attachments {
			builder.WriteString(fmt.Sprintf("- %s (%s, %d bytes)\n", item.Name, item.UploadedBy, item.Size))
		}
		builder.WriteString("\n")
	}
	if len(snapshot.SharedFiles) > 0 {
		builder.WriteString("Shared files:\n")
		for _, item := range snapshot.SharedFiles {
			builder.WriteString(fmt.Sprintf("- %s (%s/%s)\n", item.Name, item.Bucket, item.Key))
		}
	}
	return []byte(builder.String())
}

func buildCollaborationPDF(snapshot CollaborationExportSnapshot) []byte {
	text := asciiPDFText(string(buildCollaborationTXT(snapshot)))
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		lines = []string{"Collaboration transcript"}
	}
	var content strings.Builder
	y := 800
	for _, line := range lines {
		if y < 40 {
			break
		}
		content.WriteString(fmt.Sprintf("BT /F1 10 Tf 40 %d Td (%s) Tj ET\n", y, escapePDFString(line)))
		y -= 14
	}
	stream := content.String()
	var buffer bytes.Buffer
	offsets := []int{0}
	writeObject := func(id int, body string) {
		offsets = append(offsets, buffer.Len())
		buffer.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, body))
	}
	buffer.WriteString("%PDF-1.4\n")
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	writeObject(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	writeObject(5, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	xrefOffset := buffer.Len()
	buffer.WriteString(fmt.Sprintf("xref\n0 %d\n", len(offsets)))
	buffer.WriteString("0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		buffer.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	buffer.WriteString(fmt.Sprintf("trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets), xrefOffset))
	return buffer.Bytes()
}

func escapePDFString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "(", `\(`)
	value = strings.ReplaceAll(value, ")", `\)`)
	return value
}

func asciiPDFText(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			builder.WriteRune(r)
		case r >= 32 && r <= 126:
			builder.WriteRune(r)
		default:
			builder.WriteRune('?')
		}
	}
	return builder.String()
}

func mobileCollaborationActor(result MobileAppValidationResult) CollaborationActor {
	return CollaborationActor{
		Username:        mobileCollaborationUsername(result.Installation),
		BypassAllowList: true,
	}
}

func mobileCollaborationUsername(installation MobileAppInstallation) string {
	label := normalizedUsernameKey(strings.TrimSpace(installation.DeviceName))
	if label == "" {
		label = normalizedUsernameKey(installation.DeviceID)
	}
	label = deviceNamePattern.ReplaceAllString(label, "-")
	label = strings.Trim(label, "-")
	if label == "" {
		label = "device"
	}
	if len(label) > 24 {
		label = label[:21] + "..."
	}
	return "mobile-" + label
}

func (s *Service) mobileCollaborationSession(activationToken, deviceID string) (CollaborationActor, CollaborationSession, error) {
	result, err := s.ValidateMobileAppInstallation(strings.TrimSpace(activationToken), strings.TrimSpace(deviceID))
	if err != nil && !shouldReturnMobileValidationForCollaboration(err) {
		return CollaborationActor{}, CollaborationSession{}, err
	}
	if err != nil {
		return CollaborationActor{}, CollaborationSession{}, err
	}
	if strings.TrimSpace(result.CollaborationToken) == "" {
		return CollaborationActor{}, CollaborationSession{}, ErrCollaborationSessionNotFound
	}
	state := s.store.snapshot()
	session, findErr := findCollaborationSession(state.CollaborationSessions, result.CollaborationToken)
	if findErr != nil {
		return CollaborationActor{}, CollaborationSession{}, findErr
	}
	actor := mobileCollaborationActor(result)
	if accessErr := ensureCollaborationAccessibleActor(session, actor, time.Now().UTC()); accessErr != nil {
		return CollaborationActor{}, CollaborationSession{}, accessErr
	}
	return actor, collaborationSessionView(session, actor.Username), nil
}

func shouldReturnMobileValidationForCollaboration(err error) bool {
	return err == nil ||
		errors.Is(err, ErrMobileAppReleaseExpired) ||
		errors.Is(err, ErrMobileAppReleaseRevoked) ||
		errors.Is(err, ErrMobileAppInstallationRevoked) ||
		errors.Is(err, ErrCollaborationSessionClosed) ||
		errors.Is(err, ErrCollaborationSessionExpired)
}

func (s *Service) GetMobileCollaborationSession(activationToken, deviceID string) (CollaborationSession, CollaborationActor, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return CollaborationSession{}, CollaborationActor{}, err
	}
	return session, actor, nil
}

func (s *Service) AddMobileCollaborationMessage(activationToken, deviceID string, input CollaborationMessageInput) (CollaborationMessage, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return CollaborationMessage{}, err
	}
	return s.addCollaborationMessage(session.Token, actor, input)
}

func (s *Service) MarkMobileCollaborationRead(activationToken, deviceID, messageID string) (CollaborationReadEvent, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return CollaborationReadEvent{}, err
	}
	return s.markCollaborationRead(session.Token, actor, messageID)
}

func (s *Service) ToggleMobileCollaborationReaction(activationToken, deviceID, messageID, emoji string) (CollaborationMessage, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return CollaborationMessage{}, err
	}
	return s.toggleCollaborationReaction(session.Token, actor, messageID, emoji)
}

func (s *Service) RecallMobileCollaborationMessage(activationToken, deviceID, messageID string) (CollaborationMessage, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return CollaborationMessage{}, err
	}
	return s.recallCollaborationMessage(session.Token, actor, messageID)
}

func (s *Service) DeleteMobileCollaborationMessage(activationToken, deviceID, messageID string) (CollaborationDeletionEvent, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return CollaborationDeletionEvent{}, err
	}
	return s.deleteCollaborationMessage(session.Token, actor, messageID)
}

func (s *Service) ExportMobileCollaborationTranscript(activationToken, deviceID, format string) (string, string, []byte, error) {
	actor, session, err := s.mobileCollaborationSession(activationToken, deviceID)
	if err != nil {
		return "", "", nil, err
	}
	return s.exportCollaborationTranscript(session.Token, actor, format)
}
