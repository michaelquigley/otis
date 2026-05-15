package api

import (
	"context"
	"net/http"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/state"
)

type contextKey string

const authLabelKey contextKey = "auth-label"

type Server struct {
	cfg        *config.ResolvedConfig
	store      *state.Store
	dispatcher *dispatcher.Dispatcher
	auth       *TokenStore
}

func NewServer(cfg *config.ResolvedConfig, store *state.Store, dispatch *dispatcher.Dispatcher) *Server {
	return &Server{
		cfg:        cfg,
		store:      store,
		dispatcher: dispatch,
		auth:       NewTokenStore(cfg.Global.Storage.StateDir),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects", s.authorized(s.handleProjects))
	mux.HandleFunc("GET /api/v1/projects/{project}/passes", s.authorized(s.handlePasses))
	mux.HandleFunc("GET /api/v1/projects/{project}/findings", s.authorized(s.handleFindings))
	mux.HandleFunc("GET /api/v1/projects/{project}/findings/{pass}/{seq}", s.authorized(s.handleFinding))
	mux.HandleFunc("POST /api/v1/projects/{project}/findings/{pass}/{seq}/disposition", s.authorized(s.handleDisposition))
	mux.HandleFunc("GET /api/v1/projects/{project}/runs/{pass}/{date}/{time_seq}/report", s.authorized(s.handleReport))
	mux.HandleFunc("POST /api/v1/projects/{project}/passes/{pass}/run", s.authorized(s.handleRunPass))
	return mux
}

func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		label, ok := s.auth.Authorize(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), authLabelKey, label)
		next(w, r.WithContext(ctx))
	}
}

func authLabel(ctx context.Context) string {
	label, _ := ctx.Value(authLabelKey).(string)
	return label
}
