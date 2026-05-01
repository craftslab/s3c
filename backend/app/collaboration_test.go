package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCollaborationSessionAccessAndMessaging(t *testing.T) {
	service, _ := newAuthTestService(t)

	creator, err := service.SignUp("creator", "secret1")
	if err != nil {
		t.Fatalf("SignUp creator error = %v", err)
	}
	member, err := service.SignUp("member", "secret1")
	if err != nil {
		t.Fatalf("SignUp member error = %v", err)
	}
	outsider, err := service.SignUp("outsider", "secret1")
	if err != nil {
		t.Fatalf("SignUp outsider error = %v", err)
	}

	session, err := service.CreateCollaborationSession(creator, "Design review", "team-bucket", "notes", []string{member.Username}, nil)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}
	if session.AttachmentPrefix == "" {
		t.Fatal("expected attachment prefix to be set")
	}

	if _, _, err := service.GetCollaborationSession(session.Token, member); err != nil {
		t.Fatalf("GetCollaborationSession(member) error = %v", err)
	}
	if _, _, err := service.GetCollaborationSession(session.Token, outsider); !errors.Is(err, ErrCollaborationAccessDenied) {
		t.Fatalf("GetCollaborationSession(outsider) error = %v, want %v", err, ErrCollaborationAccessDenied)
	}

	message, err := service.AddCollaborationMessage(session.Token, member, CollaborationMessageInput{Content: "hello team"})
	if err != nil {
		t.Fatalf("AddCollaborationMessage() error = %v", err)
	}
	if message.Author != member.Username {
		t.Fatalf("expected author %q, got %q", member.Username, message.Author)
	}

	stored, _, err := service.GetCollaborationSession(session.Token, creator)
	if err != nil {
		t.Fatalf("GetCollaborationSession(creator) error = %v", err)
	}
	if len(stored.Messages) != 1 || stored.Messages[0].Content != "hello team" {
		t.Fatalf("unexpected messages: %#v", stored.Messages)
	}
}

func TestCollaborationMessageLifecycle(t *testing.T) {
	service, _ := newAuthTestService(t)

	creator, _ := service.SignUp("creator3", "secret1")
	member, _ := service.SignUp("member3", "secret1")

	session, err := service.CreateCollaborationSession(creator, "Chat", "team-bucket", "", []string{member.Username}, nil)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}

	root, err := service.AddCollaborationMessage(session.Token, creator, CollaborationMessageInput{
		Content:        "**hello** @member3",
		MentionedUsers: []string{"member3"},
	})
	if err != nil {
		t.Fatalf("AddCollaborationMessage(root) error = %v", err)
	}
	if len(root.Mentions) != 1 || root.Mentions[0] != member.Username {
		t.Fatalf("unexpected mentions: %#v", root.Mentions)
	}

	reply, err := service.AddCollaborationMessage(session.Token, member, CollaborationMessageInput{
		Content:    "On it",
		ReplyToID:  root.ID,
		QuickReply: "👍 Received",
	})
	if err != nil {
		t.Fatalf("AddCollaborationMessage(reply) error = %v", err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.ID != root.ID {
		t.Fatalf("expected reply reference to %q, got %#v", root.ID, reply.ReplyTo)
	}
	if reply.Type != CollaborationMessageTypeQuickReply {
		t.Fatalf("expected quick reply type, got %q", reply.Type)
	}

	if _, err := service.MarkCollaborationRead(session.Token, member, root.ID); err != nil {
		t.Fatalf("MarkCollaborationRead() error = %v", err)
	}
	reacted, err := service.ToggleCollaborationReaction(session.Token, creator, reply.ID, "🔥")
	if err != nil {
		t.Fatalf("ToggleCollaborationReaction() error = %v", err)
	}
	if len(reacted.Reactions) != 1 || reacted.Reactions[0].Emoji != "🔥" {
		t.Fatalf("unexpected reactions: %#v", reacted.Reactions)
	}

	recalled, err := service.RecallCollaborationMessage(session.Token, member, reply.ID)
	if err != nil {
		t.Fatalf("RecallCollaborationMessage() error = %v", err)
	}
	if recalled.Status != CollaborationMessageStatusRecalled {
		t.Fatalf("expected recalled status, got %q", recalled.Status)
	}

	deletion, err := service.DeleteCollaborationMessage(session.Token, creator, root.ID)
	if err != nil {
		t.Fatalf("DeleteCollaborationMessage() error = %v", err)
	}
	if deletion.MessageID != root.ID {
		t.Fatalf("expected deletion for %q, got %#v", root.ID, deletion)
	}

	creatorView, _, err := service.GetCollaborationSession(session.Token, creator)
	if err != nil {
		t.Fatalf("GetCollaborationSession(creator) error = %v", err)
	}
	if len(creatorView.Messages) != 1 || creatorView.Messages[0].ID != reply.ID {
		t.Fatalf("creator view should hide deleted message, got %#v", creatorView.Messages)
	}

	memberView, _, err := service.GetCollaborationSession(session.Token, member)
	if err != nil {
		t.Fatalf("GetCollaborationSession(member) error = %v", err)
	}
	if len(memberView.Messages) != 2 {
		t.Fatalf("member view should retain both messages, got %#v", memberView.Messages)
	}
	if memberView.Messages[1].Status != CollaborationMessageStatusRecalled {
		t.Fatalf("expected recalled message in member view, got %#v", memberView.Messages[1])
	}
}

func TestCollaborationTranscriptExport(t *testing.T) {
	service, _ := newAuthTestService(t)

	creator, _ := service.SignUp("creator4", "secret1")
	member, _ := service.SignUp("member4", "secret1")

	session, err := service.CreateCollaborationSession(creator, "Export Room", "team-bucket", "", []string{member.Username}, nil)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}
	if _, err := service.AddCollaborationMessage(session.Token, creator, CollaborationMessageInput{Content: "First line"}); err != nil {
		t.Fatalf("AddCollaborationMessage() error = %v", err)
	}
	if _, err := service.AddCollaborationMessage(session.Token, member, CollaborationMessageInput{Content: "Second line"}); err != nil {
		t.Fatalf("AddCollaborationMessage() error = %v", err)
	}

	_, jsonType, jsonData, err := service.ExportCollaborationTranscript(session.Token, creator, "json")
	if err != nil {
		t.Fatalf("ExportCollaborationTranscript(json) error = %v", err)
	}
	if !strings.Contains(jsonType, "application/json") {
		t.Fatalf("unexpected json content type %q", jsonType)
	}
	var snapshot CollaborationExportSnapshot
	if err := json.Unmarshal(jsonData, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(snapshot.Messages) != 2 {
		t.Fatalf("expected 2 exported messages, got %d", len(snapshot.Messages))
	}

	_, txtType, txtData, err := service.ExportCollaborationTranscript(session.Token, creator, "txt")
	if err != nil {
		t.Fatalf("ExportCollaborationTranscript(txt) error = %v", err)
	}
	if !strings.Contains(txtType, "text/plain") || !strings.Contains(string(txtData), "First line") {
		t.Fatalf("unexpected txt export: %q / %q", txtType, string(txtData))
	}

	_, pdfType, pdfData, err := service.ExportCollaborationTranscript(session.Token, creator, "pdf")
	if err != nil {
		t.Fatalf("ExportCollaborationTranscript(pdf) error = %v", err)
	}
	if pdfType != "application/pdf" || !strings.HasPrefix(string(pdfData), "%PDF-1.4") {
		t.Fatalf("unexpected pdf export: %q / %q", pdfType, string(pdfData[:8]))
	}
}

func TestCollaborationSessionExpiryAndClosure(t *testing.T) {
	service, _ := newAuthTestService(t)

	creator, err := service.SignUp("creator2", "secret1")
	if err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	session, err := service.CreateCollaborationSession(creator, "War room", "team-bucket", "", nil, &expiresAt)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}
	if _, err := service.CloseCollaborationSession(session.Token, creator); err != nil {
		t.Fatalf("CloseCollaborationSession() error = %v", err)
	}
	if _, _, err := service.GetCollaborationSession(session.Token, creator); !errors.Is(err, ErrCollaborationSessionClosed) {
		t.Fatalf("GetCollaborationSession() error = %v, want %v", err, ErrCollaborationSessionClosed)
	}

	activeUntil := time.Now().UTC().Add(30 * time.Minute)
	expiredAt := time.Now().UTC().Add(-time.Minute)
	expiredSession, err := service.CreateCollaborationSession(creator, "Expired", "team-bucket", "", nil, &activeUntil)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}
	if err := service.store.update(func(state *State) error {
		index, findErr := findCollaborationSessionIndex(state.CollaborationSessions, expiredSession.Token)
		if findErr != nil {
			return findErr
		}
		state.CollaborationSessions[index].ExpiresAt = &expiredAt
		return nil
	}); err != nil {
		t.Fatalf("expire session error = %v", err)
	}
	if _, _, err := service.GetCollaborationSession(expiredSession.Token, creator); !errors.Is(err, ErrCollaborationSessionExpired) {
		t.Fatalf("GetCollaborationSession(expired) error = %v, want %v", err, ErrCollaborationSessionExpired)
	}
}
