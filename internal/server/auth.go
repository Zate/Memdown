package server

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/zate/ctx/internal/auth"
	"github.com/zate/ctx/internal/db"
)

// registerAuthRoutes adds auth-related routes to the server.
func (s *Server) registerAuthRoutes() {
	// Device flow endpoints (unauthenticated)
	s.mux.HandleFunc("POST /api/auth/device", s.handleDeviceInit)
	s.mux.HandleFunc("POST /api/auth/token", s.handleDeviceToken)
	s.mux.HandleFunc("POST /api/auth/refresh", s.handleTokenRefresh)

	// Approval web page (admin-only via password)
	s.mux.HandleFunc("GET /device/authorize", s.handleApprovalPage)
	s.mux.HandleFunc("POST /device/authorize", s.handleApprovalSubmit)

	// Device management (authenticated)
	s.mux.HandleFunc("GET /api/devices", s.requireAuth(s.handleListDevices))
	s.mux.HandleFunc("POST /api/devices/{id}/revoke", s.requireAuth(s.handleRevokeDevice))
}

// --- Device flow initiation ---

type deviceInitRequest struct {
	DeviceName string `json:"device_name"`
}

type deviceInitResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func (s *Server) handleDeviceInit(w http.ResponseWriter, r *http.Request) {
	var req deviceInitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DeviceName == "" {
		req.DeviceName = "unnamed-device"
	}

	state := s.flows.Initiate(req.DeviceName)

	scheme := "http"
	if s.config.HasTLS() {
		scheme = "https"
	}
	host := r.Host

	writeJSON(w, http.StatusOK, deviceInitResponse{
		DeviceCode:      state.DeviceCode,
		UserCode:        state.UserCode,
		VerificationURI: fmt.Sprintf("%s://%s/device/authorize", scheme, host),
		ExpiresIn:       int(auth.FlowTTL.Seconds()),
		Interval:        5,
	})
}

// --- Device token polling ---

type deviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

type deviceTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	DeviceID     string `json:"device_id"`
}

func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req deviceTokenRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	state := s.flows.GetByDeviceCode(req.DeviceCode)
	if state == nil {
		writeError(w, http.StatusBadRequest, "expired_token")
		return
	}

	if state.Denied {
		writeError(w, http.StatusForbidden, "access_denied")
		return
	}

	if !state.Approved {
		writeJSON(w, http.StatusAccepted, map[string]string{"error": "authorization_pending"})
		return
	}

	writeJSON(w, http.StatusOK, deviceTokenResponse{
		AccessToken:  state.Token,
		RefreshToken: state.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.TokenExpiry.Seconds()),
		DeviceID:     state.DeviceID,
	})
}

// --- Token refresh ---

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
}

func (s *Server) handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	refreshHash := auth.HashToken(req.RefreshToken)

	// Verify refresh token belongs to this device
	var deviceID string
	var revoked bool
	err := s.store.QueryRow(
		"SELECT id, revoked FROM devices WHERE id = $1 AND refresh_token_hash = $2",
		req.DeviceID, refreshHash,
	).Scan(&deviceID, &revoked)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token")
		return
	}
	if revoked {
		writeError(w, http.StatusForbidden, "device_revoked")
		return
	}

	// Generate new tokens
	newToken := auth.GenerateToken()
	newRefresh := auth.GenerateRefreshToken()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.store.Exec(
		"UPDATE devices SET token_hash = $1, refresh_token_hash = $2, last_seen = $3 WHERE id = $4",
		auth.HashToken(newToken), auth.HashToken(newRefresh), now, deviceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update tokens")
		return
	}

	writeJSON(w, http.StatusOK, deviceTokenResponse{
		AccessToken:  newToken,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.TokenExpiry.Seconds()),
		DeviceID:     deviceID,
	})
}

// --- Approval page ---

var approvalPageTmpl = template.Must(template.New("approve").Parse(`<!DOCTYPE html>
<html>
<head><title>ctx — Device Authorization</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 480px; margin: 80px auto; padding: 0 20px; }
.code { font-size: 2em; font-weight: bold; letter-spacing: 0.1em; text-align: center; padding: 20px; background: #f0f0f0; border-radius: 8px; margin: 20px 0; }
.actions { display: flex; gap: 12px; justify-content: center; }
button { padding: 12px 32px; font-size: 1em; border: none; border-radius: 6px; cursor: pointer; }
.approve { background: #22c55e; color: white; }
.deny { background: #ef4444; color: white; }
.msg { text-align: center; padding: 20px; font-size: 1.2em; }
input[type=password] { width: 100%; padding: 10px; margin: 10px 0; box-sizing: border-box; border: 1px solid #ccc; border-radius: 4px; }
</style>
</head>
<body>
<h1>Authorize Device</h1>
{{if .Error}}<p style="color: red;">{{.Error}}</p>{{end}}
{{if .Success}}<p class="msg">{{.Success}}</p>
{{else if .UserCode}}
<p>A device <strong>{{.DeviceName}}</strong> is requesting access. Verify this code matches what you see on the device:</p>
<div class="code">{{.UserCode}}</div>
<form method="POST" action="/device/authorize">
<input type="hidden" name="user_code" value="{{.UserCode}}">
<input type="hidden" name="admin_password" value="{{.AdminPassword}}">
<div class="actions">
<button type="submit" name="action" value="approve" class="approve">Approve</button>
<button type="submit" name="action" value="deny" class="deny">Deny</button>
</div>
</form>
{{else}}
<form method="GET" action="/device/authorize">
<p>Enter the user code displayed on your device:</p>
<input type="text" name="user_code" placeholder="ABCD-1234" style="text-transform: uppercase; font-size: 1.2em; text-align: center;">
<p>Admin password:</p>
<input type="password" name="admin_password">
<button type="submit" style="width: 100%; margin-top: 10px; background: #3b82f6; color: white;">Continue</button>
</form>
{{end}}
</body>
</html>`))

type approvalPageData struct {
	UserCode      string
	DeviceName    string
	AdminPassword string
	Error         string
	Success       string
}

func (s *Server) handleApprovalPage(w http.ResponseWriter, r *http.Request) {
	userCode := strings.ToUpper(r.URL.Query().Get("user_code"))
	adminPwd := r.URL.Query().Get("admin_password")

	data := approvalPageData{}

	if userCode != "" {
		if !s.verifyAdminPassword(adminPwd) {
			data.Error = "Invalid admin password."
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = approvalPageTmpl.Execute(w, data)
			return
		}

		state := s.flows.GetByUserCode(userCode)
		if state == nil {
			data.Error = "Invalid or expired user code."
		} else {
			data.UserCode = userCode
			data.DeviceName = state.DeviceName
			data.AdminPassword = adminPwd
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = approvalPageTmpl.Execute(w, data)
}

func (s *Server) handleApprovalSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userCode := strings.ToUpper(r.FormValue("user_code"))
	adminPwd := r.FormValue("admin_password")
	action := r.FormValue("action")

	data := approvalPageData{}

	if !s.verifyAdminPassword(adminPwd) {
		data.Error = "Invalid admin password."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = approvalPageTmpl.Execute(w, data)
		return
	}

	state := s.flows.GetByUserCode(userCode)
	if state == nil {
		data.Error = "Invalid or expired user code."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = approvalPageTmpl.Execute(w, data)
		return
	}

	if action == "deny" {
		s.flows.Deny(userCode)
		data.Success = "Device access denied."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = approvalPageTmpl.Execute(w, data)
		return
	}

	// Approve: create device record and tokens
	token := auth.GenerateToken()
	refreshToken := auth.GenerateRefreshToken()
	deviceID := db.NewID()
	now := time.Now().UTC().Format(time.RFC3339)

	// Ensure admin user exists
	userID := s.ensureAdminUser()

	_, err := s.store.Exec(
		`INSERT INTO devices (id, user_id, name, token_hash, refresh_token_hash, last_seen, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		deviceID, userID, state.DeviceName, auth.HashToken(token), auth.HashToken(refreshToken), now, now,
	)
	if err != nil {
		data.Error = fmt.Sprintf("Failed to create device: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = approvalPageTmpl.Execute(w, data)
		return
	}

	s.flows.Approve(userCode, deviceID, token, refreshToken)
	data.Success = fmt.Sprintf("Device '%s' approved! You can close this page.", state.DeviceName)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = approvalPageTmpl.Execute(w, data)
}

// --- Auth middleware ---

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.AdminPassword == "" {
			// No auth configured — allow all requests
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		tokenHash := auth.HashToken(token)

		var deviceID string
		var revoked bool
		err := s.store.QueryRow(
			"SELECT id, revoked FROM devices WHERE token_hash = $1", tokenHash,
		).Scan(&deviceID, &revoked)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if revoked {
			writeError(w, http.StatusForbidden, "device has been revoked")
			return
		}

		// Update last_seen
		now := time.Now().UTC().Format(time.RFC3339)
		ip := r.RemoteAddr
		_, _ = s.store.Exec("UPDATE devices SET last_seen = $1, last_ip = $2 WHERE id = $3", now, ip, deviceID)

		// Store device ID in context via header (simple approach without context.Context changes)
		r.Header.Set("X-Device-ID", deviceID)
		next(w, r)
	}
}

// --- Device management ---

type deviceInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LastSeen  string `json:"last_seen,omitempty"`
	LastIP    string `json:"last_ip,omitempty"`
	Revoked   bool   `json:"revoked"`
	CreatedAt string `json:"created_at"`
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.Query(
		"SELECT id, name, last_seen, last_ip, revoked, created_at FROM devices ORDER BY created_at DESC",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var devices []deviceInfo
	for rows.Next() {
		var d deviceInfo
		var lastSeen, lastIP sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &lastSeen, &lastIP, &d.Revoked, &d.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if lastSeen.Valid {
			d.LastSeen = lastSeen.String
		}
		if lastIP.Valid {
			d.LastIP = lastIP.String
		}
		devices = append(devices, d)
	}

	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := s.store.Exec("UPDATE devices SET revoked = true WHERE id = $1", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "device_id": id})
}

// --- Helpers ---

func (s *Server) verifyAdminPassword(password string) bool {
	if s.config.AdminPassword == "" {
		return true // No password configured
	}
	return password == s.config.AdminPassword
}

func (s *Server) ensureAdminUser() string {
	var id string
	err := s.store.QueryRow("SELECT id FROM users WHERE username = 'admin'").Scan(&id)
	if err == nil {
		return id
	}

	id = db.NewID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.store.Exec(
		"INSERT INTO users (id, username, password_hash, created_at) VALUES ($1, 'admin', $2, $3)",
		id, auth.HashToken(s.config.AdminPassword), now,
	)
	return id
}

// authMiddleware wraps all /api/ routes (except auth endpoints) with token validation.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health, auth endpoints, and device approval page
		path := r.URL.Path
		if path == "/health" ||
			strings.HasPrefix(path, "/api/auth/") ||
			strings.HasPrefix(path, "/device/") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth if no admin password configured (local/dev mode)
		if s.config.AdminPassword == "" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		tokenHash := auth.HashToken(token)

		var deviceID string
		var revoked bool
		err := s.store.QueryRow(
			"SELECT id, revoked FROM devices WHERE token_hash = $1", tokenHash,
		).Scan(&deviceID, &revoked)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if revoked {
			writeError(w, http.StatusForbidden, "device has been revoked")
			return
		}

		// Update last_seen
		now := time.Now().UTC().Format(time.RFC3339)
		ip := r.RemoteAddr
		_, _ = s.store.Exec("UPDATE devices SET last_seen = $1, last_ip = $2 WHERE id = $3", now, ip, deviceID)

		r.Header.Set("X-Device-ID", deviceID)
		next.ServeHTTP(w, r)
	})
}
