package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/agentes-ai/qradar-collector/internal/config"
	"github.com/agentes-ai/qradar-collector/internal/forwarder"
	"github.com/agentes-ai/qradar-collector/internal/logger"
	"github.com/agentes-ai/qradar-collector/internal/qradar"
	"github.com/agentes-ai/qradar-collector/internal/state"
	"github.com/agentes-ai/qradar-collector/internal/transformer"
)

func main() {
	configPath := "config.yaml"
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		configPath = envPath
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: loading config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(cfg.Logging.Level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: creating logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Infow("starting QRadar collector",
		"poll_interval", cfg.Collector.PollIntervalSeconds,
		"state_file", cfg.Collector.StateFile,
		"worker_count", cfg.Collector.WorkerCount,
	)

	stateMgr, err := state.NewManager(cfg.Collector.StateFile)
	if err != nil {
		log.Fatalw("failed to initialize state manager", "error", err)
	}

	qClient := qradar.NewClient(
		cfg.QRadar.BaseURL,
		cfg.QRadar.APIToken,
		cfg.QRadar.Version,
		cfg.Collector.HTTPTimeoutSeconds,
		cfg.QRadar.TLSInsecure,
		log,
	)

	fwd := forwarder.NewForwarder(
		cfg.Destination.URL,
		cfg.Destination.APIKey,
		cfg.Collector.HTTPTimeoutSeconds,
		log,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Infow("received shutdown signal", "signal", sig)
		cancel()
	}()

	ticker := time.NewTicker(time.Duration(cfg.Collector.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Initial poll
	runPollCycle(ctx, log, qClient, fwd, stateMgr, cfg.Collector.WorkerCount)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down main loop")
			return
		case <-ticker.C:
			runPollCycle(ctx, log, qClient, fwd, stateMgr, cfg.Collector.WorkerCount)
		}
	}
}

func runPollCycle(
	ctx context.Context,
	log *zap.SugaredLogger,
	qClient *qradar.Client,
	fwd *forwarder.Forwarder,
	stateMgr *state.Manager,
	workerCount int,
) {
	startTime := time.Now()
	lastProcessed := stateMgr.GetLastUpdatedTime()
	
	log.Debugw("starting poll cycle", "since", lastProcessed)

	offenses, err := qClient.GetOffenses(ctx, lastProcessed)
	if err != nil {
		log.Errorw("failed to fetch offenses", "error", err)
		return
	}

	if len(offenses) == 0 {
		log.Debug("no new offenses found")
		return
	}

	log.Infow("processing new offenses", "count", len(offenses))

	// Worker pool pattern for concurrent processing
	offenseCh := make(chan qradar.Offense, len(offenses))
	var highestTime int64
	var highestTimeMu sync.Mutex

	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for off := range offenseCh {
				if ctx.Err() != nil {
					return
				}

				log.Debugw("enriching offense", "offense_id", off.ID, "worker", workerID)
				events, err := qClient.SearchEvents(ctx, off.ID)
				if err != nil {
					log.Errorw("failed Ariel search", "offense_id", off.ID, "error", err)
					// We continue processing, transform will fallback to offense data
				}

				payload := transformer.Transform(&off, events)

				if err := fwd.Send(ctx, payload); err != nil {
					log.Errorw("failed to forward offense", "offense_id", off.ID, "error", err)
					continue // DO NOT update state time for failed deliveries
				}

				// Safely track highest successful time
				highestTimeMu.Lock()
				if off.LastUpdatedTime > highestTime {
					highestTime = off.LastUpdatedTime
				}
				highestTimeMu.Unlock()
			}
		}(i)
	}

	for _, off := range offenses {
		offenseCh <- off
	}
	close(offenseCh)

	// Wait for all workers to finish this batch
	wg.Wait()

	// Only update state if we successfully forwarded at least one offense
	// with a newer timestamp.
	if highestTime > lastProcessed {
		if err := stateMgr.SetLastUpdatedTime(highestTime); err != nil {
			log.Errorw("failed to save state", "error", err)
		} else {
			log.Infow("state updated", "new_last_updated_time", highestTime)
		}
	}

	log.Infow("poll cycle completed", "duration", time.Since(startTime))
}
