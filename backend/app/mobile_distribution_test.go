package app

import (
	"errors"
	"testing"
	"time"
)

func TestMobileAppReleaseLifecycle(t *testing.T) {
	service, _ := newAuthTestService(t)

	creator, err := service.SignUp("mobile-admin", "secret1")
	if err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	sessionExpiry := time.Now().UTC().Add(4 * time.Hour)
	session, err := service.CreateCollaborationSession(creator, "Room", "team-bucket", "", nil, &sessionExpiry)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}
	releaseExpiry := time.Now().UTC().Add(2 * time.Hour)
	release, err := service.CreateMobileAppRelease(creator, MobileAppReleaseInput{
		Title:               "Kipup Mobile",
		Version:             "1.0.0",
		Platform:            "android",
		Bucket:              "team-bucket",
		ObjectKey:           "mobile/kipup.apk",
		CollaborationToken:  session.Token,
		ExpiresAt:           releaseExpiry,
		OfflineGraceSeconds: 1800,
	})
	if err != nil {
		t.Fatalf("CreateMobileAppRelease() error = %v", err)
	}
	if release.CollaborationTitle != session.Title {
		t.Fatalf("expected collaboration title %q, got %q", session.Title, release.CollaborationTitle)
	}

	linkExpiry := time.Now().UTC().Add(time.Hour)
	link, storedRelease, err := service.CreateMobileAppDownloadLink(release.ID, creator, linkExpiry)
	if err != nil {
		t.Fatalf("CreateMobileAppDownloadLink() error = %v", err)
	}
	if storedRelease.ID != release.ID {
		t.Fatalf("expected release %q, got %q", release.ID, storedRelease.ID)
	}

	installation, _, err := service.ActivateMobileAppInstallation(link.Token, MobileAppActivationRequest{
		Platform:   "android",
		DeviceID:   "device-123",
		DeviceName: "QA phone",
		AppVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("ActivateMobileAppInstallation() error = %v", err)
	}
	if installation.ActivationToken == "" {
		t.Fatal("expected activation token")
	}

	result, err := service.ValidateMobileAppInstallation(installation.ActivationToken, installation.DeviceID)
	if err != nil {
		t.Fatalf("ValidateMobileAppInstallation() error = %v", err)
	}
	if !result.Valid || result.Reason != "active" {
		t.Fatalf("unexpected validation result: %#v", result)
	}

	if _, err := service.RevokeMobileAppRelease(release.ID); err != nil {
		t.Fatalf("RevokeMobileAppRelease() error = %v", err)
	}
	if _, err := service.ValidateMobileAppInstallation(installation.ActivationToken, installation.DeviceID); !errors.Is(err, ErrMobileAppInstallationRevoked) {
		t.Fatalf("ValidateMobileAppInstallation() after revoke error = %v, want %v", err, ErrMobileAppInstallationRevoked)
	}
}

func TestMobileAppValidationFailsWhenCollaborationCloses(t *testing.T) {
	service, _ := newAuthTestService(t)

	creator, err := service.SignUp("mobile-admin-2", "secret1")
	if err != nil {
		t.Fatalf("SignUp() error = %v", err)
	}
	expiresAt := time.Now().UTC().Add(90 * time.Minute)
	session, err := service.CreateCollaborationSession(creator, "War room", "team-bucket", "", nil, &expiresAt)
	if err != nil {
		t.Fatalf("CreateCollaborationSession() error = %v", err)
	}
	release, err := service.CreateMobileAppRelease(creator, MobileAppReleaseInput{
		Title:              "Kipup Mobile",
		Version:            "1.0.1",
		Platform:           "android",
		Bucket:             "team-bucket",
		ObjectKey:          "mobile/kipup-101.apk",
		ExpiresAt:          time.Now().UTC().Add(time.Hour),
		CollaborationToken: session.Token,
	})
	if err != nil {
		t.Fatalf("CreateMobileAppRelease() error = %v", err)
	}
	link, _, err := service.CreateMobileAppDownloadLink(release.ID, creator, time.Now().UTC().Add(30*time.Minute))
	if err != nil {
		t.Fatalf("CreateMobileAppDownloadLink() error = %v", err)
	}
	installation, _, err := service.ActivateMobileAppInstallation(link.Token, MobileAppActivationRequest{Platform: "android", DeviceID: "device-456"})
	if err != nil {
		t.Fatalf("ActivateMobileAppInstallation() error = %v", err)
	}
	if _, err := service.CloseCollaborationSession(session.Token, creator); err != nil {
		t.Fatalf("CloseCollaborationSession() error = %v", err)
	}
	if _, err := service.ValidateMobileAppInstallation(installation.ActivationToken, installation.DeviceID); !errors.Is(err, ErrCollaborationSessionClosed) {
		t.Fatalf("ValidateMobileAppInstallation() error = %v, want %v", err, ErrCollaborationSessionClosed)
	}
}
