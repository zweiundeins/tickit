package routes

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/config"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/starfederation/datastar-go/datastar"
)

var setupRoutesCallCount int

func SetupRoutes(router chi.Router, deps *bootstrap.Deps) (err error) {
	setupRoutesCallCount++
	logger.Info().Int("call_count", setupRoutesCallCount).Msg("SetupRoutes called")

	if config.Global.Environment == config.Dev {
		setupReload(router)
	}

	// Register route groups from new files
	RegisterPageRoutes(router, deps)
	RegisterAPIRoutes(router, deps)
	RegisterUploadRoutes(router, deps)
	RegisterUserRoutes(router, deps)

	return nil
}

func setupReload(router chi.Router) {
	reloadChan := make(chan struct{}, 1)
	var hotReloadOnce sync.Once
	router.Get("/reload", func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r)
		reload := func() { sse.ExecuteScript("window.location.reload()") }
		hotReloadOnce.Do(reload)
		select {
		case <-reloadChan:
			reload()
		case <-r.Context().Done():
		}
	})

	router.Get("/hotreload", func(w http.ResponseWriter, r *http.Request) {
		select {
		case reloadChan <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

}
