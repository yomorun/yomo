package pprof

import (
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/yomorun/yomo/pkg/util"

	"github.com/yomorun/yomo/pkg/env"
)

type pprofConf struct {
	Enabled    bool
	PathPrefix string
	Endpoint   string
}

const (
	pprofEnabled = "YOMO_PPROF_ENABLED"
	pathPrefix   = "YOMO_PPROF_PATH_PREFIX"
	endpoint     = "YOMO_PPROF_ENDPOINT"
)

func newEdgeConf() pprofConf {
	conf := pprofConf{}
	conf.Enabled = env.GetBool(pprofEnabled, false)
	conf.PathPrefix = env.GetString(pathPrefix, "/debug/pprof/")
	conf.Endpoint = env.GetString(endpoint, "0.0.0.0:6060")
	return conf
}

var logger = util.GetLogger("yomo::pprof")

func Run() {
	conf := newEdgeConf()
	if conf.Enabled == false {
		return
	}

	mux := http.NewServeMux()
	pathPrefix := conf.PathPrefix
	mux.HandleFunc(pathPrefix,
		func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, pathPrefix)
			if name != "" {
				pprof.Handler(name).ServeHTTP(w, r)
				return
			}
			pprof.Index(w, r)
		})
	mux.HandleFunc(pathPrefix+"cmdline", pprof.Cmdline)
	mux.HandleFunc(pathPrefix+"profile", pprof.Profile)
	mux.HandleFunc(pathPrefix+"symbol", pprof.Symbol)
	mux.HandleFunc(pathPrefix+"trace", pprof.Trace)

	server := http.Server{
		Addr:    conf.Endpoint,
		Handler: mux,
	}

	logger.Infof("PProf server start... http://%s%s\n", conf.Endpoint, conf.PathPrefix)
	if err := server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			logger.Errorf("PProf server closed.")
		} else {
			logger.Errorf("PProf server error: %v", err)
		}
	}
}
