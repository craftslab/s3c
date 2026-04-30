package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials  = errors.New("invalid username or password")
	ErrUnauthorized        = errors.New("authentication required")
	ErrForbidden           = errors.New("permission denied")
	ErrUserExists          = errors.New("user already exists")
	ErrUserNotFound        = errors.New("user not found")
	ErrBuiltinAdminLocked  = errors.New("builtin admin cannot be deleted or downgraded")
	ErrLastAdminRequired   = errors.New("at least one admin must remain")
	ErrInvalidRole         = errors.New("invalid role")
	ErrInvalidUsername     = errors.New("username must be 3-64 characters")
	ErrInvalidPassword     = errors.New("password must be 4-128 characters")
	maxStoredSessions      = 500
	defaultSessionLifetime = 7 * 24 * time.Hour
	defaultUserPermissions = []Permission{PermissionUpload, PermissionDownload, PermissionSearch, PermissionPresign}
)

func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u User) HasPermission(permission Permission) bool {
	if u.IsAdmin() {
		return true
	}
	for _, current := range u.Permissions {
		if current == permission {
			return true
		}
	}
	return false
}

func (s *Service) EnsureAdmin(username, password string) error {
	normalizedUsername, err := normalizeUsername(username)
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.store.update(func(state *State) error {
		targetIndex := -1
		builtinIndex := -1
		for i, user := range state.Users {
			if sameUsername(user.Username, normalizedUsername) {
				targetIndex = i
			}
			if user.Builtin {
				builtinIndex = i
			}
		}

		switch {
		case targetIndex >= 0:
			state.Users[targetIndex].Username = normalizedUsername
			state.Users[targetIndex].Role = RoleAdmin
			state.Users[targetIndex].Permissions = clonePermissions(AllPermissions)
			state.Users[targetIndex].PasswordHash = string(hash)
			state.Users[targetIndex].Builtin = true
			state.Users[targetIndex].UpdatedAt = now
			if builtinIndex >= 0 && builtinIndex != targetIndex {
				state.Users[builtinIndex].Builtin = false
			}
		case builtinIndex >= 0:
			state.Users[builtinIndex].Username = normalizedUsername
			state.Users[builtinIndex].Role = RoleAdmin
			state.Users[builtinIndex].Permissions = clonePermissions(AllPermissions)
			state.Users[builtinIndex].PasswordHash = string(hash)
			state.Users[builtinIndex].Builtin = true
			state.Users[builtinIndex].UpdatedAt = now
		default:
			state.Users = append(state.Users, User{
				ID:           newID("user"),
				Username:     normalizedUsername,
				Role:         RoleAdmin,
				Permissions:  clonePermissions(AllPermissions),
				PasswordHash: string(hash),
				Builtin:      true,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}

		pruneExpiredSessions(state, now)
		return nil
	})
}

func (s *Service) SignUp(username, password string) (User, error) {
	normalizedUsername, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if err := validatePassword(password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	user := User{
		ID:           newID("user"),
		Username:     normalizedUsername,
		Role:         RoleUser,
		Permissions:  clonePermissions(defaultUserPermissions),
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := s.store.update(func(state *State) error {
		if findUserIndex(state.Users, normalizedUsername) >= 0 {
			return ErrUserExists
		}
		state.Users = append(state.Users, user)
		return nil
	}); err != nil {
		return User{}, err
	}
	return sanitizeUser(user), nil
}

func (s *Service) SignIn(username, password string) (string, User, error) {
	normalizedUsername, err := normalizeUsername(username)
	if err != nil {
		return "", User{}, ErrInvalidCredentials
	}

	state := s.store.snapshot()
	userIndex := findUserIndex(state.Users, normalizedUsername)
	if userIndex < 0 {
		return "", User{}, ErrInvalidCredentials
	}
	user := state.Users[userIndex]
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", User{}, ErrInvalidCredentials
	}

	token, err := newSessionToken()
	if err != nil {
		return "", User{}, err
	}
	now := time.Now().UTC()
	session := Session{
		Token:     token,
		Username:  user.Username,
		CreatedAt: now,
		ExpiresAt: now.Add(defaultSessionLifetime),
	}
	if err := s.store.update(func(state *State) error {
		pruneExpiredSessions(state, now)
		state.Sessions = append([]Session{session}, state.Sessions...)
		if len(state.Sessions) > maxStoredSessions {
			state.Sessions = state.Sessions[:maxStoredSessions]
		}
		return nil
	}); err != nil {
		return "", User{}, err
	}
	return token, sanitizeUser(user), nil
}

func (s *Service) Authenticate(token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrUnauthorized
	}

	state := s.store.snapshot()
	now := time.Now().UTC()
	for _, session := range state.Sessions {
		if session.Token != token {
			continue
		}
		if !session.ExpiresAt.After(now) {
			_ = s.store.update(func(state *State) error {
				pruneExpiredSessions(state, now)
				return nil
			})
			return User{}, ErrUnauthorized
		}
		userIndex := findUserIndex(state.Users, session.Username)
		if userIndex < 0 {
			return User{}, ErrUnauthorized
		}
		return sanitizeUser(state.Users[userIndex]), nil
	}
	return User{}, ErrUnauthorized
}

func (s *Service) SignOut(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.store.update(func(state *State) error {
		filtered := state.Sessions[:0]
		for _, session := range state.Sessions {
			if session.Token != token {
				filtered = append(filtered, session)
			}
		}
		state.Sessions = filtered
		return nil
	})
}

func (s *Service) ListUsers() []User {
	state := s.store.snapshot()
	users := make([]User, 0, len(state.Users))
	for _, user := range state.Users {
		users = append(users, sanitizeUser(user))
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].Builtin != users[j].Builtin {
			return users[i].Builtin
		}
		return users[i].Username < users[j].Username
	})
	return users
}

func (s *Service) UpdateUser(username string, role Role, permissions []Permission) (User, error) {
	normalizedUsername, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if role != RoleAdmin && role != RoleUser {
		return User{}, ErrInvalidRole
	}

	now := time.Now().UTC()
	var updated User
	if err := s.store.update(func(state *State) error {
		index := findUserIndex(state.Users, normalizedUsername)
		if index < 0 {
			return ErrUserNotFound
		}
		user := &state.Users[index]
		if user.Builtin && role != RoleAdmin {
			return ErrBuiltinAdminLocked
		}
		if user.Role == RoleAdmin && role != RoleAdmin && countAdmins(state.Users) <= 1 {
			return ErrLastAdminRequired
		}

		user.Role = role
		if role == RoleAdmin {
			user.Permissions = clonePermissions(AllPermissions)
		} else {
			user.Permissions = normalizePermissions(permissions)
		}
		user.UpdatedAt = now
		updated = sanitizeUser(*user)
		return nil
	}); err != nil {
		return User{}, err
	}
	return updated, nil
}

func (s *Service) DeleteUser(username string) error {
	normalizedUsername, err := normalizeUsername(username)
	if err != nil {
		return err
	}
	return s.store.update(func(state *State) error {
		index := findUserIndex(state.Users, normalizedUsername)
		if index < 0 {
			return ErrUserNotFound
		}
		user := state.Users[index]
		if user.Builtin {
			return ErrBuiltinAdminLocked
		}
		if user.Role == RoleAdmin && countAdmins(state.Users) <= 1 {
			return ErrLastAdminRequired
		}

		state.Users = append(state.Users[:index], state.Users[index+1:]...)
		filteredSessions := state.Sessions[:0]
		for _, session := range state.Sessions {
			if !sameUsername(session.Username, normalizedUsername) {
				filteredSessions = append(filteredSessions, session)
			}
		}
		state.Sessions = filteredSessions
		return nil
	})
}

func sanitizeUser(user User) User {
	user.PasswordHash = ""
	user.Permissions = clonePermissions(user.Permissions)
	return user
}

func clonePermissions(in []Permission) []Permission {
	if len(in) == 0 {
		return nil
	}
	out := make([]Permission, len(in))
	copy(out, in)
	return out
}

func normalizePermissions(input []Permission) []Permission {
	allowed := make(map[Permission]struct{}, len(AllPermissions))
	for _, permission := range AllPermissions {
		allowed[permission] = struct{}{}
	}
	selected := make(map[Permission]struct{}, len(input))
	for _, permission := range input {
		if _, ok := allowed[permission]; ok {
			selected[permission] = struct{}{}
		}
	}
	normalized := make([]Permission, 0, len(selected))
	for _, permission := range AllPermissions {
		if _, ok := selected[permission]; ok {
			normalized = append(normalized, permission)
		}
	}
	return normalized
}

func pruneExpiredSessions(state *State, now time.Time) {
	filtered := state.Sessions[:0]
	for _, session := range state.Sessions {
		if session.ExpiresAt.After(now) {
			filtered = append(filtered, session)
		}
	}
	state.Sessions = filtered
}

func countAdmins(users []User) int {
	total := 0
	for _, user := range users {
		if user.Role == RoleAdmin {
			total++
		}
	}
	return total
}

func findUserIndex(users []User, username string) int {
	for i, user := range users {
		if sameUsername(user.Username, username) {
			return i
		}
	}
	return -1
}

func sameUsername(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func normalizeUsername(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) < 3 || len(normalized) > 64 {
		return "", ErrInvalidUsername
	}
	return normalized, nil
}

func validatePassword(value string) error {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 4 || len(trimmed) > 128 {
		return ErrInvalidPassword
	}
	return nil
}

func newSessionToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}
