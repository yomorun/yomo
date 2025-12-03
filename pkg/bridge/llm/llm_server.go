package llm

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/yomorun/yomo"
	pkgai "github.com/yomorun/yomo/pkg/bridge/ai"
	"github.com/yomorun/yomo/pkg/bridge/ai/provider"
	_ "github.com/yomorun/yomo/pkg/bridge/ai/register"
	"github.com/yomorun/yomo/pkg/id"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// BasicAPIServer provides restful service for end user
type BasicAPIServer struct {
	httpHandler http.Handler
}

// Serve starts the Basic API Server
func Serve(config *pkgai.Config, logger *slog.Logger, source yomo.Source, reducer yomo.StreamFunction) error {
	provider, err := provider.GetProvider(config.Server.Provider)
	if err != nil {
		return err
	}
	srv, err := NewBasicAPIServer(config, provider, source, reducer, logger)
	if err != nil {
		return err
	}

	return http.ListenAndServe(config.Server.Addr, srv.httpHandler)
}

// NewBasicAPIServer creates a new restful service
func NewBasicAPIServer(config *pkgai.Config, provider provider.LLMProvider, source yomo.Source, reducer yomo.StreamFunction, logger *slog.Logger) (*BasicAPIServer, error) {
	logger = logger.With("service", "llm-bridge")

	opts := &pkgai.ServiceOptions{
		Logger:         logger,
		SourceBuilder:  func(_ string) yomo.Source { return source },
		ReducerBuilder: func(_ string) yomo.StreamFunction { return reducer },
	}
	service := pkgai.NewService(provider, opts)

	mux := pkgai.NewServeMux(pkgai.NewHandler(service))

	server := &BasicAPIServer{
		httpHandler: pkgai.DecorateHandler(mux, DecorateReqContext(service, logger)),
	}

	logger.Info("[llm] start llm bridge service", "addr", config.Server.Addr, "provider", provider.Name())
	return server, nil
}

// DecorateReqContext decorates the context of the request, it injects a transID into the request's context,
// log the request information and start tracing the request.
func DecorateReqContext(service *pkgai.Service, logger *slog.Logger) func(handler http.Handler) http.Handler {
	hostname, _ := os.Hostname()
	tracer := otel.Tracer("yomo-llm-bridge")

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := pkgai.NewResponseWriter(w, logger)

			ctx := r.Context()
			ctx = pkgai.WithTracerContext(ctx, tracer)

			start := time.Now()

			caller, err := service.LoadOrCreateCaller(r)
			if err != nil {
				logger.Error("failed to load or create caller", "error", err)
				pkgai.RespondWithError(ww, http.StatusBadRequest, err)
				return
			}
			ctx = pkgai.WithCallerContext(ctx, caller)

			// trace every request
			ctx, span := tracer.Start(
				ctx,
				r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("host", hostname)),
			)
			defer span.End()

			transID := id.New(32)
			ctx = pkgai.WithTransIDContext(ctx, transID)

			handler.ServeHTTP(ww, r.WithContext(ctx))

			duration := time.Since(start)

			logContent := []any{
				"namespace", fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				"stream", ww.IsStream(),
				"host", hostname,
				"requestId", transID,
				"duration", duration,
			}
			if traceID := span.SpanContext().TraceID(); traceID.IsValid() {
				logContent = append(logContent, "traceId", traceID.String())
			}
			if err := ww.GetError(); err != nil {
				logger.Error("llm birdge request", append(logContent, "err", err)...)
			} else {
				logger.Info("llm birdge request", logContent...)
			}
		})
	}
}
