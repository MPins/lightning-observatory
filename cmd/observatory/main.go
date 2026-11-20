// Command observatory reads the public Lightning graph and exports metrics
// about it for Prometheus.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MPins/lightning-observatory/internal/graph"
	"github.com/MPins/lightning-observatory/internal/source/lnd"
	"github.com/MPins/lightning-observatory/internal/version"
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "print the version and exit")
		cfg         lnd.Config
		logEvery    = flag.Duration("log-interval", time.Minute, "how often to log the graph size")
	)
	flag.StringVar(&cfg.Host, "lnd.host", "localhost:10009", "lnd gRPC address")
	flag.StringVar(&cfg.TLSCertPath, "lnd.tlscert", "", "path to lnd's tls.cert")
	flag.StringVar(&cfg.MacaroonPath, "lnd.macaroon", "", "path to a macaroon; readonly.macaroon is enough")
	flag.Parse()

	if *showVersion {
		fmt.Println("observatory", version.Get())
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, *logEvery); err != nil {
		slog.Error("observatory stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg lnd.Config, logEvery time.Duration) error {
	slog.Info("starting", "version", version.Get().String(), "lnd", cfg.Host)

	src, err := lnd.Dial(cfg)
	if err != nil {
		return err
	}
	defer src.Close()

	g, updates, err := graph.Load(ctx, src)
	if err != nil {
		return err
	}
	logSize(g, "graph loaded")

	tick := time.NewTicker(logEvery)
	defer tick.Stop()
	for {
		select {
		case u, ok := <-updates:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("lnd graph stream closed")
			}
			g.Apply(u)
		case <-tick.C:
			logSize(g, "graph")
		case <-ctx.Done():
			return nil
		}
	}
}

func logSize(g *graph.Graph, msg string) {
	slog.Info(msg, "nodes", g.NodeCount(), "channels", g.ChannelCount())
}
