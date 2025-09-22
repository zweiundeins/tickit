package shared

import (
	"encoding/json"
	"net/http"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/starfederation/datastar-go/datastar"
)

// SignalsAsStruct converts a generic signals object (map[string]any, etc.)
// into a typed struct. target must be a pointer to a struct.
// It first tries to read signals directly from the request using datastar.ReadSignals,
// and falls back to using signals from context if that fails.
func SignalsAsStruct[T any](r *http.Request) (T, error) {
	var target T

	// First try to read signals directly from the request
	err := datastar.ReadSignals(r, &target)
	if err == nil {
		logger.Debug().
			Type("target_type", target).
			Msg("Successfully read signals directly from request")
		return target, nil
	}

	logger.Debug().
		Err(err).
		Type("target_type", target).
		Msg("Failed to read signals directly from request, falling back to context")

	// Fallback: Get signals from context (parsed by DatastarSignalsMiddleware)
	signals := r.Context().Value("datastar_signals")
	if signals == nil {
		logger.Error().
			Type("target_type", target).
			Msg("No signals found in context either")
		return target, err // Return the original ReadSignals error
	}

	// Convert signals map to JSON and then unmarshal into target struct
	signalsBytes, err := json.Marshal(signals)
	if err != nil {
		logger.Error().
			Err(err).
			Type("target_type", target).
			Msg("Error marshaling signals from context")
		return target, err
	}

	err = json.Unmarshal(signalsBytes, &target)
	if err != nil {
		logger.Error().
			Err(err).
			Type("target_type", target).
			Msg("Error unmarshaling signals into struct")
	} else {
		logger.Debug().
			Type("target_type", target).
			Msg("Successfully read signals from context fallback")
	}
	return target, err
}
