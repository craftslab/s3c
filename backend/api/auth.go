package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/craftslab/kipup/backend/app"
	"github.com/gin-gonic/gin"
)

const contextUserKey = "auth.user"

type authRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type updateUserRequest struct {
	Role        app.Role         `json:"role" binding:"required"`
	Permissions []app.Permission `json:"permissions"`
}

type createTemporaryUserRequest struct {
	ExpiresAt   string           `json:"expiresAt" binding:"required"`
	Permissions []app.Permission `json:"permissions"`
}

func (h *Handler) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := h.service.Authenticate(bearerToken(c.GetHeader("Authorization")))
		if err != nil {
			writeAuthError(c, err)
			c.Abort()
			return
		}
		c.Set(contextUserKey, user)
		c.Next()
	}
}

func (h *Handler) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := currentUserFromContext(c)
		if !ok || !user.IsAdmin() {
			c.JSON(http.StatusForbidden, gin.H{"error": app.ErrForbidden.Error()})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *Handler) requirePermission(permission app.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := currentUserFromContext(c)
		if !ok || !user.HasPermission(permission) {
			c.JSON(http.StatusForbidden, gin.H{"error": app.ErrForbidden.Error(), "permission": permission})
			c.Abort()
			return
		}
		c.Next()
	}
}

func currentUserFromContext(c *gin.Context) (app.User, bool) {
	value, ok := c.Get(contextUserKey)
	if !ok {
		return app.User{}, false
	}
	user, ok := value.(app.User)
	return user, ok
}

func (h *Handler) SignUp(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.service.SignUp(req.Username, req.Password)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, app.ErrUserExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (h *Handler) SignIn(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, user, err := h.service.SignIn(req.Username, req.Password)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

func (h *Handler) Me(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) SignOut(c *gin.Context) {
	_ = h.service.SignOut(bearerToken(c.GetHeader("Authorization")))
	c.JSON(http.StatusOK, gin.H{"message": "signed out"})
}

func (h *Handler) ListUsers(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListUsers())
}

func (h *Handler) CreateTemporaryUser(c *gin.Context) {
	var req createTemporaryUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expiresAt must be a valid RFC3339 timestamp"})
		return
	}
	user, password, err := h.service.CreateTemporaryUser(expiresAt, req.Permissions)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"user": user,
		"credentials": gin.H{
			"username": user.Username,
			"password": password,
		},
	})
}

func (h *Handler) UpdateUser(c *gin.Context) {
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.service.UpdateUser(c.Param("username"), req.Role, req.Permissions)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, app.ErrUserNotFound):
			status = http.StatusNotFound
		case errors.Is(err, app.ErrForbidden), errors.Is(err, app.ErrBuiltinAdminLocked), errors.Is(err, app.ErrLastAdminRequired), errors.Is(err, app.ErrTemporaryUserRoleLocked):
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Handler) DeleteUser(c *gin.Context) {
	if err := h.service.DeleteUser(c.Param("username")); err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, app.ErrUserNotFound):
			status = http.StatusNotFound
		case errors.Is(err, app.ErrBuiltinAdminLocked), errors.Is(err, app.ErrLastAdminRequired):
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func writeAuthError(c *gin.Context, err error) {
	status := http.StatusUnauthorized
	if errors.Is(err, app.ErrForbidden) {
		status = http.StatusForbidden
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "bearer "
	if strings.HasPrefix(strings.ToLower(header), prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return header
}
