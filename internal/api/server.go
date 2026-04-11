package api

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/report"
	"github.com/hjma/probex/internal/store"
)

type Server struct {
	router    *chi.Mux
	store     store.Store
	runner    *probe.Runner
	registry  *probe.Registry
	generator *report.Generator
	alertEval AlertEvaluator
}

func NewServer(s store.Store, runner *probe.Runner, registry *probe.Registry, gen *report.Generator, alertEval AlertEvaluator) *Server {
	srv := &Server{
		store:     s,
		runner:    runner,
		registry:  registry,
		generator: gen,
		alertEval: alertEval,
	}
	srv.setupRoutes()
	return srv
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	taskH := NewTaskHandler(s.store, s.runner)
	resultH := NewResultHandler(s.store)
	agentH := NewAgentHandler(s.store, s.alertEval)
	pluginH := NewPluginHandler(s.registry)
	reportH := NewReportHandler(s.store, s.generator)
	alertH := NewAlertHandler(s.store)
	probeH := NewProbeHandler(s.registry, s.store, s.alertEval)

	r.Route("/api/v1", func(r chi.Router) {
		// Tasks
		r.Post("/tasks", taskH.Create)
		r.Get("/tasks", taskH.List)
		r.Get("/tasks/{id}", taskH.Get)
		r.Put("/tasks/{id}", taskH.Update)
		r.Delete("/tasks/{id}", taskH.Delete)
		r.Post("/tasks/{id}/pause", taskH.Pause)
		r.Post("/tasks/{id}/resume", taskH.Resume)
		r.Post("/tasks/{id}/run", taskH.RunOnce)

		// Results
		r.Get("/results", resultH.Query)
		r.Get("/results/summary", resultH.Summary)
		r.Get("/results/latest", resultH.Latest)
		r.Delete("/results", resultH.Clear)

		// Export
		r.Get("/export/csv", resultH.ExportCSV)
		r.Get("/export/json", resultH.ExportJSON)

		// Agents
		r.Post("/agents/register", agentH.Register)
		r.Get("/agents", agentH.List)
		r.Get("/agents/{id}", agentH.Get)
		r.Delete("/agents/{id}", agentH.Delete)
		r.Post("/agents/{id}/heartbeat", agentH.Heartbeat)
		r.Get("/agents/{id}/tasks", agentH.GetTasks)
		r.Post("/agents/{id}/results", agentH.PushResults)

		// Plugins (legacy)
		r.Get("/plugins", pluginH.List)

		// Probes (new unified registry)
		r.Get("/probes", probeH.List)
		r.Get("/probes/{name}", probeH.Get)
		r.Post("/probes/register", probeH.Register)
		r.Delete("/probes/{name}", probeH.Deregister)
		r.Post("/probes/{name}/push", probeH.PushResults)

		// Reports
		r.Post("/reports", reportH.Create)
		r.Get("/reports", reportH.List)
		r.Get("/reports/{id}", reportH.Get)
		r.Delete("/reports/{id}", reportH.Delete)
		r.Get("/reports/{id}/download", reportH.Download)

		// Alerts
		r.Post("/alerts/rules", alertH.CreateRule)
		r.Get("/alerts/rules", alertH.ListRules)
		r.Get("/alerts/rules/{id}", alertH.GetRule)
		r.Put("/alerts/rules/{id}", alertH.UpdateRule)
		r.Delete("/alerts/rules/{id}", alertH.DeleteRule)
		r.Get("/alerts/events", alertH.ListEvents)
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]string{"status": "ok"})
	})

	s.router = r
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
