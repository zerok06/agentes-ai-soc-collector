package main

import (
	"context"
	"encoding/json"
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
	defer stateMgr.Close()

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
	runPollCycle(ctx, log, qClient, fwd, stateMgr, cfg)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down main loop")
			return
		case <-ticker.C:
			runPollCycle(ctx, log, qClient, fwd, stateMgr, cfg)
		}
	}
}

func runPollCycle(
	ctx context.Context,
	log *zap.SugaredLogger,
	qClient *qradar.Client,
	fwd *forwarder.Forwarder,
	stateMgr *state.Manager,
	cfg *config.Config,
) {
	startTime := time.Now()
	lastProcessed := stateMgr.GetLastUpdatedTime()

	// If this is the very first run (state is 0), only fetch from the last polling interval
	// to avoid downloading years of historical offenses.
	if lastProcessed == 0 {
		// QRadar times are in milliseconds
		lastProcessed = startTime.Add(-time.Duration(cfg.Collector.PollIntervalSeconds) * time.Second).UnixMilli()
		log.Infow("first run detected, skipping historical offenses", "starting_from_ms", lastProcessed)
	}

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

	for i := 0; i < cfg.Collector.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for off := range offenseCh {
				if ctx.Err() != nil {
					return
				}

				// Strict deduplication by VERSION: never process an offense version we already successfully sent
				if stateMgr.HasOffenseVersion(off.ID, off.LastUpdatedTime) {
					log.Debugw("offense version already processed, skipping", "offense_id", off.ID, "last_updated_time", off.LastUpdatedTime)

					// We still want to track highest time just to move the pointer forward
					highestTimeMu.Lock()
					if off.LastUpdatedTime > highestTime {
						highestTime = off.LastUpdatedTime
					}
					highestTimeMu.Unlock()
					continue
				}

				log.Debugw("enriching offense", "offense_id", off.ID, "worker", workerID)

				clientName, err := qClient.GetDomainName(ctx, off.DomainID)
				if err != nil {
					log.Warnw("failed to fetch domain name, using fallback", "domain_id", off.DomainID, "error", err)
					_ = stateMgr.RecordAudit(off.ID, off.LastUpdatedTime, "ERROR_DOMAIN", err.Error(), "")
					clientName = fmt.Sprintf("Domain-%d", off.DomainID)
				}

				events, err := qClient.SearchEvents(ctx, off.ID)
				if err != nil {
					log.Errorw("failed Ariel search", "offense_id", off.ID, "error", err)
					_ = stateMgr.RecordAudit(off.ID, off.LastUpdatedTime, "ERROR_ARIEL", err.Error(), "")
					// We continue processing, transform will fallback to offense data
				}

				payload := transformer.Transform(&off, events, clientName)
				payloadBytes, _ := json.Marshal(payload)

				if err := fwd.Send(ctx, payload); err != nil {
					log.Errorw("failed to forward offense", "offense_id", off.ID, "error", err)
					_ = stateMgr.RecordAudit(off.ID, off.LastUpdatedTime, "ERROR_FORWARD", err.Error(), string(payloadBytes))
					continue // DO NOT update state time for failed deliveries
				}

				// Audit log the exact payload sent alongside success status
				if err := stateMgr.RecordAudit(off.ID, off.LastUpdatedTime, "SUCCESS", "", string(payloadBytes)); err != nil {
					log.Errorw("failed to save offense to audit log", "offense_id", off.ID, "error", err)
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

	// Auto-cleanup: delete audit logs older than 7 days to prevent infinite SQLite growth
	if err := stateMgr.CleanOldLogs(7); err != nil {
		log.Errorw("failed to clean old audit logs", "error", err)
	}

	log.Infow("poll cycle completed", "duration", time.Since(startTime))
}
