package app

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	ErrMobileAppReleaseNotFound      = errors.New("mobile app release not found")
	ErrMobileAppReleaseRevoked       = errors.New("mobile app release has been revoked")
	ErrMobileAppReleaseExpired       = errors.New("mobile app release has expired")
	ErrMobileDownloadLinkNotFound    = errors.New("mobile app download link not found")
	ErrMobileDownloadLinkExpired     = errors.New("mobile app download link has expired")
	ErrMobileAppInstallationNotFound = errors.New("mobile app installation not found")
	ErrMobileAppInstallationRevoked  = errors.New("mobile app installation has been revoked")
	ErrInvalidMobileAppTitle         = errors.New("mobile app title is required")
	ErrInvalidMobileAppVersion       = errors.New("mobile app version is required")
	ErrInvalidMobileAppBucket        = errors.New("mobile app bucket is required")
	ErrInvalidMobileAppObjectKey     = errors.New("mobile app object key is required")
	ErrInvalidMobileAppPlatform      = errors.New("mobile app platform must be android or ios")
	ErrInvalidMobileAppExpiry        = errors.New("mobile app expiry must be in the future")
	ErrInvalidMobileAppDeviceID      = errors.New("mobile app device id is required")
	ErrMobileAppActivationDenied     = errors.New("mobile app activation request is invalid")
)

const defaultMobileOfflineGraceSeconds = 24 * 60 * 60

type MobileAppPlatform string

type MobileAppReleaseStatus string

type MobileAppInstallationStatus string

const (
	MobileAppPlatformAndroid MobileAppPlatform = "android"
	MobileAppPlatformIOS     MobileAppPlatform = "ios"
)

const (
	MobileAppReleaseActive  MobileAppReleaseStatus = "active"
	MobileAppReleaseRevoked MobileAppReleaseStatus = "revoked"
)

const (
	MobileAppInstallationActive  MobileAppInstallationStatus = "active"
	MobileAppInstallationExpired MobileAppInstallationStatus = "expired"
	MobileAppInstallationRevoked MobileAppInstallationStatus = "revoked"
)

type MobileAppRelease struct {
	ID                  string                 `json:"id"`
	Title               string                 `json:"title"`
	Version             string                 `json:"version"`
	Platform            MobileAppPlatform      `json:"platform"`
	Bucket              string                 `json:"bucket"`
	ObjectKey           string                 `json:"objectKey"`
	FileName            string                 `json:"fileName"`
	ContentType         string                 `json:"contentType,omitempty"`
	Size                int64                  `json:"size"`
	Status              MobileAppReleaseStatus `json:"status"`
	CreatedBy           string                 `json:"createdBy"`
	CollaborationToken  string                 `json:"collaborationToken,omitempty"`
	CollaborationTitle  string                 `json:"collaborationTitle,omitempty"`
	OfflineGraceSeconds int                    `json:"offlineGraceSeconds"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
	ExpiresAt           time.Time              `json:"expiresAt"`
	RevokedAt           *time.Time             `json:"revokedAt,omitempty"`
}

type MobileAppDownloadLink struct {
	ID         string            `json:"id"`
	ReleaseID  string            `json:"releaseId"`
	Token      string            `json:"token"`
	CreatedBy  string            `json:"createdBy"`
	CreatedAt  time.Time         `json:"createdAt"`
	ExpiresAt  time.Time         `json:"expiresAt"`
	Platform   MobileAppPlatform `json:"platform"`
	Downloaded int               `json:"downloaded"`
}

type MobileAppInstallation struct {
	ID                  string                      `json:"id"`
	ReleaseID           string                      `json:"releaseId"`
	DownloadLinkID      string                      `json:"downloadLinkId"`
	ActivationToken     string                      `json:"activationToken"`
	Platform            MobileAppPlatform           `json:"platform"`
	DeviceID            string                      `json:"deviceId"`
	DeviceName          string                      `json:"deviceName,omitempty"`
	AppVersion          string                      `json:"appVersion,omitempty"`
	Status              MobileAppInstallationStatus `json:"status"`
	CollaborationToken  string                      `json:"collaborationToken,omitempty"`
	CollaborationTitle  string                      `json:"collaborationTitle,omitempty"`
	OfflineGraceSeconds int                         `json:"offlineGraceSeconds"`
	CreatedAt           time.Time                   `json:"createdAt"`
	ActivatedAt         time.Time                   `json:"activatedAt"`
	LastValidatedAt     *time.Time                  `json:"lastValidatedAt,omitempty"`
	LastSeenAt          *time.Time                  `json:"lastSeenAt,omitempty"`
	ExpiresAt           time.Time                   `json:"expiresAt"`
	RevokedAt           *time.Time                  `json:"revokedAt,omitempty"`
}

type MobileAppReleaseInput struct {
	Title               string
	Version             string
	Platform            string
	Bucket              string
	ObjectKey           string
	ContentType         string
	Size                int64
	CollaborationToken  string
	ExpiresAt           time.Time
	OfflineGraceSeconds int
}

type MobileAppActivationRequest struct {
	Platform   string
	DeviceID   string
	DeviceName string
	AppVersion string
}

type MobileAppValidationResult struct {
	Valid               bool
	Reason              string
	Release             MobileAppRelease
	Installation        MobileAppInstallation
	CollaborationToken  string
	CollaborationTitle  string
	ExpiresAt           time.Time
	OfflineGraceSeconds int
	ServerTime          time.Time
}

func (s *Service) ListMobileAppReleases() []MobileAppRelease {
	state := s.store.snapshot()
	now := time.Now().UTC()
	items := append([]MobileAppRelease(nil), state.MobileAppReleases...)
	sort.Slice(items, func(i, j int) bool {
		leftExpired := isMobileAppReleaseExpired(items[i], now)
		rightExpired := isMobileAppReleaseExpired(items[j], now)
		if leftExpired != rightExpired {
			return !leftExpired
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func (s *Service) CreateMobileAppRelease(user User, input MobileAppReleaseInput) (MobileAppRelease, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return MobileAppRelease{}, ErrInvalidMobileAppTitle
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		return MobileAppRelease{}, ErrInvalidMobileAppVersion
	}
	platform, err := normalizeMobileAppPlatform(input.Platform)
	if err != nil {
		return MobileAppRelease{}, err
	}
	bucket := strings.TrimSpace(input.Bucket)
	if bucket == "" {
		return MobileAppRelease{}, ErrInvalidMobileAppBucket
	}
	objectKey := normalizeMobileObjectKey(input.ObjectKey)
	if objectKey == "" {
		return MobileAppRelease{}, ErrInvalidMobileAppObjectKey
	}
	expiresAt := input.ExpiresAt.UTC()
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		return MobileAppRelease{}, ErrInvalidMobileAppExpiry
	}
	offlineGraceSeconds := input.OfflineGraceSeconds
	if offlineGraceSeconds <= 0 {
		offlineGraceSeconds = defaultMobileOfflineGraceSeconds
	}
	release := MobileAppRelease{
		ID:                  newID("mobile-release"),
		Title:               title,
		Version:             version,
		Platform:            platform,
		Bucket:              bucket,
		ObjectKey:           objectKey,
		FileName:            path.Base(objectKey),
		ContentType:         strings.TrimSpace(input.ContentType),
		Size:                input.Size,
		Status:              MobileAppReleaseActive,
		CreatedBy:           user.Username,
		CollaborationToken:  strings.TrimSpace(input.CollaborationToken),
		OfflineGraceSeconds: offlineGraceSeconds,
		CreatedAt:           now,
		UpdatedAt:           now,
		ExpiresAt:           expiresAt,
	}
	if release.CollaborationToken != "" {
		state := s.store.snapshot()
		session, findErr := findCollaborationSession(state.CollaborationSessions, release.CollaborationToken)
		if findErr != nil {
			return MobileAppRelease{}, findErr
		}
		release.CollaborationTitle = session.Title
	}
	if err := s.store.update(func(state *State) error {
		state.MobileAppReleases = append([]MobileAppRelease{release}, state.MobileAppReleases...)
		return nil
	}); err != nil {
		return MobileAppRelease{}, err
	}
	return release, nil
}

func (s *Service) RevokeMobileAppRelease(id string) (MobileAppRelease, error) {
	now := time.Now().UTC()
	var release MobileAppRelease
	if err := s.store.update(func(state *State) error {
		index := findMobileAppReleaseIndex(state.MobileAppReleases, id)
		if index < 0 {
			return ErrMobileAppReleaseNotFound
		}
		state.MobileAppReleases[index].Status = MobileAppReleaseRevoked
		state.MobileAppReleases[index].UpdatedAt = now
		state.MobileAppReleases[index].RevokedAt = &now
		release = state.MobileAppReleases[index]
		for i := range state.MobileAppInstallations {
			if state.MobileAppInstallations[i].ReleaseID != release.ID || state.MobileAppInstallations[i].Status == MobileAppInstallationRevoked {
				continue
			}
			state.MobileAppInstallations[i].Status = MobileAppInstallationRevoked
			state.MobileAppInstallations[i].RevokedAt = &now
		}
		return nil
	}); err != nil {
		return MobileAppRelease{}, err
	}
	return release, nil
}

func (s *Service) CreateMobileAppDownloadLink(releaseID string, user User, expiresAt time.Time) (MobileAppDownloadLink, MobileAppRelease, error) {
	now := time.Now().UTC()
	release, err := s.findMobileAppRelease(releaseID)
	if err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	if err := ensureMobileAppReleaseAvailable(release, now); err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	expiresAt = expiresAt.UTC()
	if !expiresAt.After(now) {
		return MobileAppDownloadLink{}, MobileAppRelease{}, ErrInvalidMobileAppExpiry
	}
	if expiresAt.After(release.ExpiresAt) {
		expiresAt = release.ExpiresAt
	}
	token, err := randomString(40, "abcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	link := MobileAppDownloadLink{
		ID:        newID("mobile-link"),
		ReleaseID: release.ID,
		Token:     token,
		CreatedBy: user.Username,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Platform:  release.Platform,
	}
	if err := s.store.update(func(state *State) error {
		state.MobileAppDownloadLinks = append([]MobileAppDownloadLink{link}, state.MobileAppDownloadLinks...)
		return nil
	}); err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	return link, release, nil
}

func (s *Service) GetMobileAppDownloadLink(token string) (MobileAppDownloadLink, MobileAppRelease, error) {
	state := s.store.snapshot()
	now := time.Now().UTC()
	link, err := findMobileAppDownloadLink(state.MobileAppDownloadLinks, token)
	if err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	if !link.ExpiresAt.After(now) {
		return MobileAppDownloadLink{}, MobileAppRelease{}, ErrMobileDownloadLinkExpired
	}
	release, err := findMobileAppRelease(state.MobileAppReleases, link.ReleaseID)
	if err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	if err := ensureMobileAppReleaseAvailable(release, now); err != nil {
		return MobileAppDownloadLink{}, MobileAppRelease{}, err
	}
	if release.CollaborationToken != "" {
		if err := ensureMobileCollaborationActive(state.CollaborationSessions, release.CollaborationToken, now); err != nil {
			return MobileAppDownloadLink{}, MobileAppRelease{}, err
		}
	}
	return link, release, nil
}

func (s *Service) RecordMobileAppDownload(linkID string) {
	_ = s.store.update(func(state *State) error {
		for i := range state.MobileAppDownloadLinks {
			if state.MobileAppDownloadLinks[i].ID == linkID {
				state.MobileAppDownloadLinks[i].Downloaded++
				return nil
			}
		}
		return nil
	})
}

func (s *Service) ActivateMobileAppInstallation(downloadToken string, req MobileAppActivationRequest) (MobileAppInstallation, MobileAppRelease, error) {
	link, release, err := s.GetMobileAppDownloadLink(downloadToken)
	if err != nil {
		return MobileAppInstallation{}, MobileAppRelease{}, err
	}
	platform, err := normalizeMobileAppPlatform(req.Platform)
	if err != nil {
		return MobileAppInstallation{}, MobileAppRelease{}, err
	}
	if platform != release.Platform {
		return MobileAppInstallation{}, MobileAppRelease{}, ErrMobileAppActivationDenied
	}
	deviceID := normalizeMobileDeviceID(req.DeviceID)
	if deviceID == "" {
		return MobileAppInstallation{}, MobileAppRelease{}, ErrInvalidMobileAppDeviceID
	}
	activationToken, err := randomString(64, "abcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		return MobileAppInstallation{}, MobileAppRelease{}, err
	}
	now := time.Now().UTC()
	installation := MobileAppInstallation{
		ID:                  newID("mobile-install"),
		ReleaseID:           release.ID,
		DownloadLinkID:      link.ID,
		ActivationToken:     activationToken,
		Platform:            platform,
		DeviceID:            deviceID,
		DeviceName:          strings.TrimSpace(req.DeviceName),
		AppVersion:          strings.TrimSpace(req.AppVersion),
		Status:              MobileAppInstallationActive,
		CollaborationToken:  release.CollaborationToken,
		CollaborationTitle:  release.CollaborationTitle,
		OfflineGraceSeconds: release.OfflineGraceSeconds,
		CreatedAt:           now,
		ActivatedAt:         now,
		ExpiresAt:           release.ExpiresAt,
	}
	if err := s.store.update(func(state *State) error {
		for i := range state.MobileAppInstallations {
			current := state.MobileAppInstallations[i]
			if current.ReleaseID != release.ID || current.DeviceID != deviceID {
				continue
			}
			state.MobileAppInstallations[i] = installation
			return nil
		}
		state.MobileAppInstallations = append([]MobileAppInstallation{installation}, state.MobileAppInstallations...)
		return nil
	}); err != nil {
		return MobileAppInstallation{}, MobileAppRelease{}, err
	}
	return installation, release, nil
}

func (s *Service) ValidateMobileAppInstallation(activationToken, deviceID string) (MobileAppValidationResult, error) {
	deviceID = normalizeMobileDeviceID(deviceID)
	if deviceID == "" {
		return MobileAppValidationResult{}, ErrInvalidMobileAppDeviceID
	}
	state := s.store.snapshot()
	now := time.Now().UTC()
	installation, err := findMobileAppInstallationByToken(state.MobileAppInstallations, activationToken)
	if err != nil {
		return MobileAppValidationResult{}, err
	}
	if installation.DeviceID != deviceID {
		return MobileAppValidationResult{}, ErrMobileAppActivationDenied
	}
	if installation.Status == MobileAppInstallationRevoked {
		return MobileAppValidationResult{Valid: false, Reason: "revoked", Installation: installation, ExpiresAt: installation.ExpiresAt, ServerTime: now}, ErrMobileAppInstallationRevoked
	}
	if !installation.ExpiresAt.After(now) {
		return MobileAppValidationResult{Valid: false, Reason: "expired", Installation: installation, ExpiresAt: installation.ExpiresAt, ServerTime: now}, ErrMobileAppReleaseExpired
	}
	release, err := findMobileAppRelease(state.MobileAppReleases, installation.ReleaseID)
	if err != nil {
		return MobileAppValidationResult{}, err
	}
	if err := ensureMobileAppReleaseAvailable(release, now); err != nil {
		return MobileAppValidationResult{Valid: false, Reason: mobileValidationReason(err), Installation: installation, Release: release, ExpiresAt: installation.ExpiresAt, ServerTime: now}, err
	}
	if release.CollaborationToken != "" {
		if err := ensureMobileCollaborationActive(state.CollaborationSessions, release.CollaborationToken, now); err != nil {
			return MobileAppValidationResult{Valid: false, Reason: mobileValidationReason(err), Installation: installation, Release: release, CollaborationToken: release.CollaborationToken, CollaborationTitle: release.CollaborationTitle, ExpiresAt: installation.ExpiresAt, ServerTime: now}, err
		}
	}
	_ = s.store.update(func(state *State) error {
		for i := range state.MobileAppInstallations {
			if state.MobileAppInstallations[i].ID != installation.ID {
				continue
			}
			state.MobileAppInstallations[i].LastValidatedAt = &now
			state.MobileAppInstallations[i].LastSeenAt = &now
			installation = state.MobileAppInstallations[i]
			return nil
		}
		return nil
	})
	return MobileAppValidationResult{
		Valid:               true,
		Reason:              "active",
		Release:             release,
		Installation:        installation,
		CollaborationToken:  release.CollaborationToken,
		CollaborationTitle:  release.CollaborationTitle,
		ExpiresAt:           installation.ExpiresAt,
		OfflineGraceSeconds: installation.OfflineGraceSeconds,
		ServerTime:          now,
	}, nil
}

func (s *Service) ListMobileAppInstallations(releaseID string) ([]MobileAppInstallation, error) {
	state := s.store.snapshot()
	if _, err := findMobileAppRelease(state.MobileAppReleases, releaseID); err != nil {
		return nil, err
	}
	items := make([]MobileAppInstallation, 0)
	for _, item := range state.MobileAppInstallations {
		if item.ReleaseID == releaseID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (s *Service) RevokeMobileAppInstallation(id string) (MobileAppInstallation, error) {
	now := time.Now().UTC()
	var installation MobileAppInstallation
	if err := s.store.update(func(state *State) error {
		index := findMobileAppInstallationIndex(state.MobileAppInstallations, id)
		if index < 0 {
			return ErrMobileAppInstallationNotFound
		}
		state.MobileAppInstallations[index].Status = MobileAppInstallationRevoked
		state.MobileAppInstallations[index].RevokedAt = &now
		installation = state.MobileAppInstallations[index]
		return nil
	}); err != nil {
		return MobileAppInstallation{}, err
	}
	return installation, nil
}

func (s *Service) findMobileAppRelease(id string) (MobileAppRelease, error) {
	state := s.store.snapshot()
	return findMobileAppRelease(state.MobileAppReleases, id)
}

func findMobileAppRelease(items []MobileAppRelease, id string) (MobileAppRelease, error) {
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return MobileAppRelease{}, ErrMobileAppReleaseNotFound
}

func findMobileAppReleaseIndex(items []MobileAppRelease, id string) int {
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func findMobileAppDownloadLink(items []MobileAppDownloadLink, token string) (MobileAppDownloadLink, error) {
	token = strings.TrimSpace(token)
	for _, item := range items {
		if item.Token == token {
			return item, nil
		}
	}
	return MobileAppDownloadLink{}, ErrMobileDownloadLinkNotFound
}

func findMobileAppInstallationByToken(items []MobileAppInstallation, activationToken string) (MobileAppInstallation, error) {
	activationToken = strings.TrimSpace(activationToken)
	for _, item := range items {
		if item.ActivationToken == activationToken {
			return item, nil
		}
	}
	return MobileAppInstallation{}, ErrMobileAppInstallationNotFound
}

func findMobileAppInstallationIndex(items []MobileAppInstallation, id string) int {
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func ensureMobileAppReleaseAvailable(release MobileAppRelease, now time.Time) error {
	if release.Status == MobileAppReleaseRevoked {
		return ErrMobileAppReleaseRevoked
	}
	if isMobileAppReleaseExpired(release, now) {
		return ErrMobileAppReleaseExpired
	}
	return nil
}

func isMobileAppReleaseExpired(release MobileAppRelease, now time.Time) bool {
	return !release.ExpiresAt.After(now)
}

func ensureMobileCollaborationActive(items []CollaborationSession, token string, now time.Time) error {
	session, err := findCollaborationSession(items, token)
	if err != nil {
		return err
	}
	if session.Status == CollaborationSessionClosed {
		return ErrCollaborationSessionClosed
	}
	if session.ExpiresAt != nil && !session.ExpiresAt.After(now) {
		return ErrCollaborationSessionExpired
	}
	return nil
}

func normalizeMobileAppPlatform(value string) (MobileAppPlatform, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(MobileAppPlatformAndroid):
		return MobileAppPlatformAndroid, nil
	case string(MobileAppPlatformIOS):
		return MobileAppPlatformIOS, nil
	default:
		return "", ErrInvalidMobileAppPlatform
	}
}

func normalizeMobileObjectKey(value string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "/"))
}

func normalizeMobileDeviceID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		value = value[:128]
	}
	return value
}

func mobileValidationReason(err error) string {
	switch {
	case errors.Is(err, ErrMobileAppReleaseExpired), errors.Is(err, ErrMobileDownloadLinkExpired), errors.Is(err, ErrCollaborationSessionExpired):
		return "expired"
	case errors.Is(err, ErrMobileAppReleaseRevoked), errors.Is(err, ErrMobileAppInstallationRevoked), errors.Is(err, ErrCollaborationSessionClosed):
		return "revoked"
	default:
		return fmt.Sprintf("invalid:%s", err.Error())
	}
}
