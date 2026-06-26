package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cost-board/internal/auth"
	"cost-board/internal/store"
)

type Server struct {
	store    *store.Store
	auth     *auth.Auth
	rl       *rateLimiter
	embedFS  http.FileSystem
}

func NewServer(s *store.Store, a *auth.Auth, embedFS http.FileSystem) *Server {
	return &Server{
		store:   s,
		auth:    a,
		rl:      newRateLimiter(),
		embedFS: embedFS,
	}
}

func (srv *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/login", handleLogin(srv.auth, srv.rl))
	mux.HandleFunc("/api/logout", authMiddleware(srv.auth, handleLogout(srv.auth)))
	mux.HandleFunc("/api/items", srv.handleItems)

	if srv.embedFS != nil {
		fileServer := http.FileServer(srv.embedFS)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			cleanPath := strings.TrimPrefix(r.URL.Path, "/")
			if cleanPath == "" {
				cleanPath = "index.html"
			}
			if f, err := srv.embedFS.Open("/" + cleanPath); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			r.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r)
		})
	}

	return srv.withMiddleware(mux)
}

func (srv *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		handleGetItems(srv.store)(w, r)
	case http.MethodPut:
		authMiddleware(srv.auth, handlePutItems(srv.store))(w, r)
	default:
		w.Header().Set("Allow", "GET, HEAD, PUT")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (srv *Server) withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.ServeHTTP(w, r)
	})
}

func (srv *Server) Start(host string, port int) error {
	go srv.cleanupLoop()

	addr := host + ":" + strconv.Itoa(port)
	log.Printf("cost-board listening on http://%s", addr)
	return http.ListenAndServe(addr, srv.Handler())
}

func (srv *Server) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		srv.auth.CleanupExpiredSessions()
	}
}
