package app

import (
	"errors"
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

	message, err := service.AddCollaborationMessage(session.Token, member, "hello team")
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
