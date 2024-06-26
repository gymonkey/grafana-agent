package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/go-kit/log/level"
	"github.com/grafana/agent/internal/boringcrypto"
	"github.com/grafana/agent/internal/build"
	"github.com/grafana/agent/internal/flowmode"
	util_log "github.com/grafana/agent/internal/util/log"
	"github.com/grafana/agent/static/config"
	"github.com/grafana/agent/static/server"

	"github.com/prometheus/client_golang/prometheus"

	// Register Prometheus SD components
	_ "github.com/grafana/loki/clients/pkg/promtail/discovery/consulagent"
	_ "github.com/prometheus/prometheus/discovery/install"

	// Register integrations
	_ "github.com/grafana/agent/static/integrations/install"

	// Embed a set of fallback X.509 trusted roots
	// Allows the app to work correctly even when the OS does not provide a verifier or systems roots pool
	_ "golang.org/x/crypto/x509roots/fallback"
	_ "net/http/pprof"
)

func init() {
	prometheus.MustRegister(build.NewCollector("agent"))
}

func main() {
	go http.ListenAndServe(":9394", nil)
	// If Windows is trying to run as a service, go through that
	// path instead.
	if IsWindowsService() {
		err := RunService()
		if err != nil {
			log.Fatalln(err)
		}
		return
	}

	runMode, err := getRunMode()
	if err != nil {
		log.Fatalln(err)
	}

	// NOTE(rfratto): Flow when run through the primary Grafana Agent binary does
	// not support being run as a Windows service. To run Flow mode as a Windows
	// service, use cmd/grafana-agent-service and cmd/grafana-agent-flow instead.
	if runMode == runModeFlow {
		flowmode.Run()
		return
	}

	// Set up logging using default values before loading the config
	defaultCfg := server.DefaultConfig()
	logger := server.NewLogger(&defaultCfg)

	reloader := func(log *server.Logger) (*config.Config, error) {
		fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		return config.Load(fs, os.Args[1:], log)
	}
	cfg, err := reloader(logger)
	if err != nil {
		log.Fatalln(err)
	}

	// After this point we can start using go-kit logging.
	logger = server.NewLogger(cfg.Server)
	util_log.Logger = logger

	level.Info(logger).Log("boringcrypto enabled", boringcrypto.Enabled)
	ep, err := NewEntrypoint(logger, cfg, reloader)
	if err != nil {
		level.Error(logger).Log("msg", "error creating the agent server entrypoint", "err", err)
		os.Exit(1)
	}

	if err = ep.Start(); err != nil {
		level.Error(logger).Log("msg", "error running agent", "err", err)
		// Don't os.Exit here; we want to do cleanup by stopping promMetrics
	}

	ep.Stop()
	level.Info(logger).Log("msg", "agent exiting")
}
