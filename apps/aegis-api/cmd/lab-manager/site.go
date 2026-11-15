package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"company.local/aegis/apps/aegis-api/internal/api"
	"company.local/aegis/apps/aegis-api/internal/api/middleware"
	"company.local/aegis/apps/aegis-api/internal/api/requestctx"
	"company.local/aegis/apps/aegis-api/internal/auth"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/go-chi/chi/v5"
)

const sessionTTL = 20 * time.Minute

type labAccount struct {
	user      domain.User
	principal domain.Principal
}

type labAccounts map[string]labAccount

func (a labAccounts) GetUserByUsername(_ context.Context, name string) (domain.User, error) {
	account, ok := a[name]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return account.user, nil
}
func (a labAccounts) GetUser(ctx context.Context, id string) (domain.User, error) {
	return a.GetUserByUsername(ctx, id)
}
func (a labAccounts) GetPrincipal(_ context.Context, id string) (domain.Principal, error) {
	account, ok := a[id]
	if !ok {
		return domain.Principal{}, domain.ErrNotFound
	}
	return account.principal, nil
}
func (labAccounts) CreateUser(context.Context, domain.User) error    { return domain.ErrForbidden }
func (labAccounts) AssignRole(context.Context, string, string) error { return domain.ErrForbidden }

func labLogin(operator, viewer string) (*auth.Service, error) {
	if operator == viewer {
		return nil, errors.New("set distinct local Lab operator and viewer passwords")
	}
	accounts := make(labAccounts)
	for name, password := range map[string]string{"operator": operator, "viewer": viewer} {
		hash, err := auth.HashPassword(password)
		if err != nil {
			return nil, errors.New("both Lab passwords must contain 12–72 bytes")
		}
		permissions := []string{"devices.read", "backups.read"}
		if name == "operator" {
			permissions = append(permissions, "devices.manage", "credentials.manage", "configs.read")
		}
		accounts[name] = labAccount{
			user:      domain.User{ID: name, Username: name, Enabled: true, PasswordHash: hash},
			principal: domain.Principal{UserID: name, Username: name, Permissions: permissions},
		}
	}
	key := randomBytes(32)
	return auth.NewService(accounts, "aegis-lab", "lab-local", key, sessionTTL, sessionTTL)
}

type labSession struct {
	principal domain.Principal
	until     time.Time
}

type labSite struct {
	host, cookie string
	auth         *auth.Service
	manager      *labManager
	assets       fs.FS
	mu           sync.Mutex
	sessions     map[string]labSession
}

func newLabSite(host string, login *auth.Service, manager *labManager, assets fs.FS) *labSite {
	_, port, _ := net.SplitHostPort(host)
	return &labSite{host: host, cookie: "aegis_lab_" + port, auth: login, manager: manager, assets: assets, sessions: make(map[string]labSession)}
}

func (s *labSite) routes() http.Handler {
	router := chi.NewRouter()
	router.Use(s.localOnly, middleware.RequestID)
	router.With(middleware.NewRateLimiter(1, 5).Middleware).Post("/api/lab/session", s.signIn)
	router.Group(func(r chi.Router) {
		r.Use(s.authenticate)
		r.Get("/api/lab/session", func(w http.ResponseWriter, r *http.Request) {
			p, _ := api.Principal(r.Context())
			api.WriteData(w, r, http.StatusOK, p)
		})
		r.Delete("/api/lab/session", s.signOut)
		m := s.manager
		r.With(middleware.Require("devices.read")).Get("/api/lab/devices", m.listDevices)
		r.With(middleware.Require("devices.manage")).Post("/api/lab/devices", m.saveDevice)
		r.With(middleware.Require("devices.manage")).Put("/api/lab/devices/{deviceID}", m.saveDevice)
		r.With(middleware.Require("credentials.manage")).Get("/api/lab/credentials", m.ListCredentials)
		r.With(middleware.Require("credentials.manage")).Post("/api/lab/credentials", m.createCredential)
		devices := "/api/lab/devices/{deviceID}"
		r.With(middleware.Require("devices.manage")).Post(devices+"/test-ssh", m.deviceAction(true))
		r.With(middleware.Require("configs.read")).Post(devices+"/read-running", m.deviceAction(false))
		backups := devices + "/backups"
		r.With(middleware.Require("backups.read")).Get(backups, m.ListBackups)
		r.With(middleware.Require("backups.read")).Get(backups+"/{backupID}", m.GetBackup)
		r.With(middleware.Require("backups.read")).Get(backups+"/{leftID}/diff/{rightID}", m.DiffBackups)
		r.With(middleware.Require("devices.manage")).Post(backups, func(w http.ResponseWriter, r *http.Request) {
			// The native BackupService stores plaintext configuration and change
			// details. L1 exposes its read paths only until at-rest protection is accepted.
			labProblem(w, http.StatusForbidden, "BACKUP_STORAGE_LOCKED", "Backup creation is locked pending approved protection for configuration and change evidence at rest.")
		})
	})
	router.Get("/", s.serveUI)
	router.Get("/assets/{name}", s.serveUI)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) { api.WriteError(w, r, domain.ErrNotFound) })
	return router
}

func (s *labSite) localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || host != "127.0.0.1" || r.Host != s.host {
			labProblem(w, http.StatusForbidden, "LOOPBACK_ONLY", "Use the exact local Lab URL.")
			return
		}
		origin := r.Header.Get("Origin")
		fetchSite := r.Header.Get("Sec-Fetch-Site")
		if (origin != "" && origin != "http://"+s.host) ||
			(strings.HasPrefix(r.URL.Path, "/api/") && fetchSite != "" && fetchSite != "same-origin") ||
			(r.Method != http.MethodGet && r.Method != http.MethodHead && origin != "http://"+s.host) {
			labProblem(w, http.StatusForbidden, "ORIGIN_BLOCKED", "This Lab request must come from the same local origin.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *labSite) signIn(w http.ResponseWriter, r *http.Request) {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		labProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED", "Use the local Lab sign-in form.")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := api.DecodeJSON(w, r, &input); err != nil {
		api.WriteError(w, r, err)
		return
	}
	_, p, err := s.auth.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		labProblem(w, http.StatusUnauthorized, "LOGIN_FAILED", "The Lab role or password is incorrect.")
		return
	}
	now := time.Now()
	id := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	s.mu.Lock()
	for token, session := range s.sessions {
		if !session.until.After(now) {
			delete(s.sessions, token)
		}
	}
	if old, err := r.Cookie(s.cookie); err == nil {
		delete(s.sessions, old.Value)
	}
	if len(s.sessions) >= 64 {
		s.mu.Unlock()
		api.WriteError(w, r, domain.ErrRateLimited)
		return
	}
	s.sessions[id] = labSession{principal: p, until: now.Add(sessionTTL)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.cookie, Value: id, Path: "/api/lab", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	api.WriteData(w, r, http.StatusOK, p)
}

func (s *labSite) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cookie)
		if err != nil {
			api.WriteError(w, r, domain.ErrUnauthorized)
			return
		}
		s.mu.Lock()
		session, ok := s.sessions[cookie.Value]
		s.mu.Unlock()
		if !ok || !session.until.After(time.Now()) {
			api.WriteError(w, r, domain.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(requestctx.WithPrincipal(r.Context(), session.principal)))
	})
}

func (s *labSite) signOut(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(s.cookie)
	s.mu.Lock()
	delete(s.sessions, cookie.Value)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.cookie, Path: "/api/lab", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

var assetName = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.(js|css|svg)$`)

func (s *labSite) serveUI(w http.ResponseWriter, r *http.Request) {
	path := "index.html"
	if name := chi.URLParam(r, "name"); name != "" {
		if !assetName.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		path = "assets/" + name
	}
	if _, err := fs.Stat(s.assets, path); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, s.assets, path)
}

func labProblem(w http.ResponseWriter, status int, code, message string) {
	api.WriteJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
