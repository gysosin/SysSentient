package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sys-sentient/internal/version"
	"syscall"
	"time"

	"sys-sentient/internal/agent"
	"sys-sentient/internal/ai"
	"sys-sentient/internal/alerting"
	"sys-sentient/internal/auth"
	"sys-sentient/internal/collector"
	"sys-sentient/internal/config"
	"sys-sentient/internal/logging"
	"sys-sentient/internal/logs"
	"sys-sentient/internal/models"
	"sys-sentient/internal/pii"
	"sys-sentient/internal/server"
	"sys-sentient/internal/storage"
)

func main() {
	// Subcommands are dispatched before flag parsing, so `agent join` can own
	// its own flag set rather than sharing the daemon's.
	if len(os.Args) > 2 && os.Args[1] == "agent" && os.Args[2] == "join" {
		if err := runJoin(os.Args[3:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "enrolment failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// A monitoring agent with no --version cannot be audited across a fleet.
	showVersion := flag.Bool("version", false, "print version and exit")
	configPath := flag.String("config", "", "path to config file (default: ./config.yaml or /etc/sys-sentient)")
	backupTo := flag.String("backup", "", "write a consistent copy of the database to this path and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Get().String())
		return
	}

	// 1. Load Configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		// The logger is not configured yet, so this one stays on the stdlib.
		log.Fatalf("Failed to load config: %v", err)
	}
	logger := logging.New(os.Stdout, logging.Options{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	slog.SetDefault(logger)

	build := version.Get()
	logger.Info("starting sys-sentient", "version", build.Version, "commit", build.Commit, "go", build.GoVersion, "platform", build.Platform)
	logger.Info("configuration loaded",
		"poll_interval_seconds", cfg.Collector.PollIntervalSeconds,
		"server_port", cfg.Server.Port,
		"top_processes", cfg.Collector.TopProcesses,
		"alerting_enabled", cfg.Alerting.Enabled,
	)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Mode == config.ModeAgent {
		runAgent(ctx, cfg, logger, build)
		return
	}

	// 2. Initialize Storage
	// `--backup` is a one-shot operation, not a mode: open the database, copy
	// it, exit. Deliberately available while another daemon is running against
	// the same file — that is the case it exists for, and VACUUM INTO is safe
	// there whereas copying the file is not.
	if *backupTo != "" {
		if err := runBackup(cfg.Database.Path, *backupTo); err != nil {
			logger.Error("backup failed", "error", err)
			os.Exit(1)
		}
		return
	}

	store, err := storage.NewStore(cfg.Database.Path)
	if err != nil {
		logger.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("error closing storage", "error", err)
		}
	}()
	logger.Info("storage initialized", "path", cfg.Database.Path)

	setupToken := bootstrapSetup(store, cfg, logger)

	// 3. Initialize AI Service
	aiService, err := ai.NewAIService(ctx, cfg.Gemini)
	if err != nil {
		logger.Warn("AI service disabled", "reason", err)
	} else {
		logger.Info("AI service initialized", "model", cfg.Gemini.ModelName)
	}

	// 4. Alerting
	evaluator := alerting.NewEvaluator(alerting.DefaultRules())
	notifiers := alerting.BuildNotifiers(cfg.Alerting.WebhookURL, cfg.Alerting.SlackWebhookURL)
	dispatcher := alerting.NewDispatcher(slog.Default(), notifiers...)
	if cfg.Alerting.Enabled && !dispatcher.Enabled() {
		logger.Warn("alerting enabled but no notification channel configured; " +
			"alerts will appear in the dashboard but nobody will be notified " +
			"(set alerting.webhook_url or alerting.slack_webhook_url)")
	}

	// 5. Start API Server
	srv := server.NewServer(cfg.Server, cfg.Privacy, store, aiService, evaluator, dispatcher).
		WithAuth(cfg.Auth, setupToken)
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	if cfg.Mode == config.ModeServer {
		logger.Info("running in server mode: not collecting locally, waiting for agents to push to /api/ingest")
		runServerOnly(ctx, cfg, logger, srv, store, serverErr)
		return
	}

	// 5. Initialize PII Scrubber
	scrubber := pii.NewScrubber(cfg.Privacy.MaskIPs, cfg.Privacy.MaskEmails, cfg.Privacy.MaskUsernames)

	// 6. Initialize Log Reader
	logReader := logs.NewLogReader(50) // max 50 log lines
	logger.Debug("log reader initialized")

	// 7. Initialize Collector
	col := collector.NewCollectorWithHostID(cfg.Collector.TopProcesses, cfg.Collector.HostID)
	logger.Debug("collector initialized", "top_processes", cfg.Collector.TopProcesses)

	// 8. Polling Loop
	interval := time.Duration(cfg.Collector.PollIntervalSeconds) * time.Second
	if interval == 0 {
		interval = 2 * time.Second
	}
	logger.Info("collector started", "interval", interval)
	// Live settings. Changing the poll interval retimes this ticker in place
	// rather than requiring a restart, which is what the setting is for.
	runtime := config.NewRuntime(cfg)
	ticker := time.NewTicker(interval)
	runtime.OnPollIntervalChange(func(d time.Duration) {
		ticker.Reset(d)
		logger.Info("collector poll interval changed", "interval", d)
	})
	runtime.OnLogLevelChange(func(level string) {
		logging.SetLevel(level)
		logger.Info("log level changed", "level", level)
	})
	srv.SetRuntime(runtime)
	defer ticker.Stop()

	// Database Maintenance Ticker (Every 1 hour)
	dbTicker := time.NewTicker(1 * time.Hour)
	defer dbTicker.Stop()

	// Analysis Cooldown
	lastAnalysisTime := time.Time{}
	analysisCooldown := 5 * time.Minute // Don't analyze more than once every 5 minutes

	// last_seen only needs to be fresh enough to answer "is this host
	// reporting", not accurate to the individual sample.
	// The maintenance tick fires hourly, so 24 ticks is a daily VACUUM.
	const vacuumEveryNTicks = 24
	maintenanceTicks := 0

	const hostUpsertInterval = 30 * time.Second
	var lastHostUpsert time.Time

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received, stopping daemon")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				logger.Error("error shutting down API server", "error", err)
			}
			return

		case err, ok := <-serverErr:
			if ok && err != nil {
				logger.Error("API server failed", "error", err)
			}
			return

		case <-dbTicker.C:
			// Roll up before pruning, never after: PruneTiers deletes the raw
			// samples the rollup reads, so reversing these two loses the
			// history permanently rather than aggregating it.
			rawHours, minuteDays, fiveMinuteDays := runtime.Retention()
			policy := storage.RetentionPolicy{
				RawHours:       rawHours,
				MinuteDays:     minuteDays,
				FiveMinuteDays: fiveMinuteDays,
			}
			if err := store.Rollup(policy, time.Now()); err != nil {
				logger.Error("error rolling up metrics", "error", err)
			} else if err := store.PruneTiers(policy, time.Now()); err != nil {
				logger.Error("error pruning metric tiers", "error", err)
			}
			if err := store.PruneOldInsights(cfg.Database.InsightsRetentionHours); err != nil {
				logger.Error("error pruning old insights", "error", err)
			}
			if err := store.PruneOldAlertEvents(cfg.Database.InsightsRetentionHours); err != nil {
				logger.Error("error pruning old alert events", "error", err)
			}
			if _, err := store.PruneExpiredSessions(time.Now()); err != nil {
				logger.Error("error pruning expired sessions", "error", err)
			}

			// Reclaim what the deletes above freed. VACUUM takes an exclusive
			// lock and rewrites the file, so it runs daily rather than on
			// every maintenance tick; the checkpoint is cheap and runs each
			// time, because under continuous writes the WAL otherwise grows
			// past the database it belongs to.
			maintenanceTicks++
			if err := store.Compact(maintenanceTicks%vacuumEveryNTicks == 0); err != nil {
				logger.Warn("error compacting database", "error", err)
			}

		case <-ticker.C:
			// Collect
			state, err := col.Collect()
			if err != nil {
				logger.Error("error collecting metrics", "error", err)
				continue
			}

			// Save
			if err := store.Save(state); err != nil {
				logger.Error("error saving metrics", "error", err)
				continue
			}

			// Register this machine in the hosts table.
			//
			// Only the ingest path did this, so an all-in-one install — the
			// default, and every single-node deployment — left `hosts` empty
			// while metrics accumulated. GET /api/hosts returned [] and the
			// dashboard papered over it with `hosts.length || 1`. The fleet
			// features build on this table, so it has to be true first.
			//
			// Throttled rather than written every tick: this upserts one row
			// whose only changing field is last_seen, and doing that twice a
			// second would add a write per sample to keep a timestamp fresher
			// than anything reads it.
			if now := time.Now(); now.Sub(lastHostUpsert) >= hostUpsertInterval {
				if err := store.UpsertHost(state.HostID, state.Hostname, version.Get().Version, now); err != nil {
					logger.Warn("error registering host", "error", err)
				} else {
					lastHostUpsert = now
				}
			}

			// Broadcast to WebSocket clients
			if err := srv.Hub.BroadcastMetrics(state); err != nil {
				logger.Warn("error broadcasting metrics", "error", err)
			}

			// Evaluate alert rules. Only state transitions come back, so a rule
			// breached for an hour notifies once rather than every 2 seconds.
			//
			// Evaluate() advances the state machine, so it must be called
			// exactly once per sample: a second call would report no
			// transitions and notifications would silently never fire.
			if cfg.Alerting.Enabled {
				now := time.Now()
				transitions := evaluator.Evaluate(*state, now)

				for _, transition := range transitions {
					logger.Warn("alert transition",
						"state", string(transition.State),
						"rule", transition.RuleID,
						"severity", string(transition.Severity),
						"metric", string(transition.Metric),
						"value", transition.Value,
						"threshold", transition.Threshold,
						"host", transition.Hostname,
					)

					if err := store.SaveAlertEvent(storage.AlertEvent{
						OccurredAt: now,
						RuleID:     transition.RuleID,
						RuleName:   transition.RuleName,
						Metric:     string(transition.Metric),
						State:      string(transition.State),
						Severity:   string(transition.Severity),
						Value:      transition.Value,
						Threshold:  transition.Threshold,
						Hostname:   transition.Hostname,
					}); err != nil {
						logger.Error("error saving alert event", "error", err)
					}
				}

				// Notify off the collector's critical path: a wedged Slack
				// endpoint must not stall metrics collection.
				if len(transitions) > 0 {
					go dispatcher.Dispatch(ctx, transitions)
				}
			}

			// Log to console
			logger.Debug("metrics collected",
				"cpu", state.CPUUsage,
				"memory_used_mb", state.MemoryUsed/1024/1024,
				"memory_total_mb", state.MemoryTotal/1024/1024,
				"load1", state.LoadAvg1,
				"processes", len(state.Processes),
			)

			// Check Triggers for AI Analysis
			if aiService != nil {
				if shouldTriggerAutomaticAnalysis(*state) && time.Since(lastAnalysisTime) > analysisCooldown {
					logger.Info("threshold crossed, requesting AI analysis")
					lastAnalysisTime = time.Now()

					go func(ctx context.Context) {
						// Collect real system logs with timeout
						rawLogs, err := logReader.GetLogsWithTimeout(5 * time.Second)
						if err != nil {
							logger.Warn("failed to collect logs for analysis", "error", err)
							rawLogs = "Failed to collect system logs."
						}
						scrubbedLogs := scrubber.SanitizeLog(rawLogs)

						insight, err := aiService.AnalyzeSystemState(ctx, scrubber.SanitizeState(*state), scrubbedLogs)
						if err != nil {
							logger.Error("error analyzing system state", "error", err)
							return
						}

						logger.Info("AI insight generated", "summary", formatInsightLogSummary(insight))
						// Persist insight history for dashboard retrieval.
						if err := store.SaveInsight(insight); err != nil {
							logger.Error("error saving insight", "error", err)
						}
					}(ctx)
				}
			}
		}
	}
}

func shouldTriggerAutomaticAnalysis(state models.SystemState) bool {
	if state.CPUUsage > 80.0 {
		return true
	}
	return state.MemoryTotal > 0 && float64(state.MemoryUsed)/float64(state.MemoryTotal) > 0.9
}

func formatInsightLogSummary(raw string) string {
	var analysis models.AIAnalysis
	if err := json.Unmarshal([]byte(raw), &analysis); err != nil {
		return "AI insight generated"
	}

	status := strings.TrimSpace(analysis.Status)
	if status == "" {
		status = "Unknown"
	}
	summary := compactLogText(analysis.Summary, 120)
	if summary == "" {
		return fmt.Sprintf("AI insight generated: status=%s actions=%d", status, len(analysis.RecommendedActions))
	}
	return fmt.Sprintf("AI insight generated: status=%s actions=%d summary=%q", status, len(analysis.RecommendedActions), summary)
}

func compactLogText(value string, maxLength int) string {
	compact := strings.Join(strings.Fields(value), " ")
	if len(compact) <= maxLength {
		return compact
	}
	return compact[:maxLength] + "..."
}

// runServerOnly serves the API and dashboard without collecting from the local
// machine. Retention still runs, because the server owns the fleet's data.
func runServerOnly(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	srv *server.Server,
	store *storage.Store,
	serverErr <-chan error,
) {
	dbTicker := time.NewTicker(1 * time.Hour)
	defer dbTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received, stopping server")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				logger.Error("error shutting down API server", "error", err)
			}
			return

		case err, ok := <-serverErr:
			if ok && err != nil {
				logger.Error("API server failed", "error", err)
			}
			return

		case <-dbTicker.C:
			if err := store.PruneOldMetrics(cfg.Database.MetricsRetentionHours); err != nil {
				logger.Error("error pruning old metrics", "error", err)
			}
			if err := store.PruneOldInsights(cfg.Database.InsightsRetentionHours); err != nil {
				logger.Error("error pruning old insights", "error", err)
			}
			if err := store.PruneOldAlertEvents(cfg.Database.InsightsRetentionHours); err != nil {
				logger.Error("error pruning old alert events", "error", err)
			}
			if _, err := store.PruneExpiredSessions(time.Now()); err != nil {
				logger.Error("error pruning expired sessions", "error", err)
			}
		}
	}
}

// runAgent collects locally and pushes to a server.
//
// No storage, no AI, no dashboard: the agent's only job is to sample and
// forward, buffering to disk so a network partition loses nothing.
func runAgent(ctx context.Context, cfg *config.Config, logger *slog.Logger, build version.Info) {
	client, err := agent.New(agent.Options{
		ServerURL:          cfg.Agent.ServerURL,
		Key:                cfg.Agent.Key,
		AgentVersion:       build.Version,
		SpoolPath:          cfg.Agent.SpoolPath,
		BatchSize:          cfg.Agent.BatchSize,
		CACertPath:         cfg.Agent.CACertPath,
		InsecureSkipVerify: cfg.Agent.InsecureSkipVerify,
		Logger:             logger,
	})
	if err != nil {
		logger.Error("failed to start agent", "error", err)
		os.Exit(1)
	}

	col := collector.NewCollectorWithHostID(cfg.Collector.TopProcesses, cfg.Collector.HostID)

	interval := time.Duration(cfg.Collector.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Pushing on a slower cadence than collection batches samples, which cuts
	// request overhead and lets a brief outage be absorbed by the spool.
	flushInterval := interval * 5
	if flushInterval < 5*time.Second {
		flushInterval = 5 * time.Second
	}
	flushTicker := time.NewTicker(flushInterval)
	defer flushTicker.Stop()

	logger.Info("agent started",
		"server", cfg.Agent.ServerURL,
		"interval", interval,
		"flush_interval", flushInterval,
		"spool", cfg.Agent.SpoolPath,
	)

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received, flushing spool before exit")
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := client.Flush(flushCtx); err != nil {
				logger.Warn("final flush failed; samples remain spooled for the next start", "error", err)
			}
			cancel()
			return

		case <-ticker.C:
			state, err := col.Collect()
			if err != nil {
				logger.Error("error collecting metrics", "error", err)
				continue
			}
			if err := client.Enqueue(*state); err != nil {
				logger.Error("error buffering sample", "error", err)
			}

		case <-flushTicker.C:
			if err := client.Flush(ctx); err != nil {
				// Expected during an outage: the spool retains the samples and
				// the next tick retries.
				switch {
				case errors.Is(err, agent.ErrCredentialRevoked):
					logger.Error("this agent has been revoked; it will keep collecting but the server will not accept its data. Re-enrol with `sys-sentient agent join` using a new token.",
						"server", cfg.Agent.ServerURL, "pending", client.Pending())
				case errors.Is(err, agent.ErrCredentialRejected):
					logger.Error("the server does not recognise this agent's credential. Check agent.key, or re-enrol with `sys-sentient agent join`.",
						"server", cfg.Agent.ServerURL, "pending", client.Pending())
				default:
					logger.Warn("push failed, samples remain spooled", "pending", client.Pending(), "error", err)
				}
			}
		}
	}
}

// bootstrapSetup mints the one-time first-run token when no account exists.
//
// Printing a secret to the log is deliberate and documented: it is the only
// way to create the first admin without shipping a default password, it is
// single-use, and it dies with the process.
func bootstrapSetup(store *storage.Store, cfg *config.Config, logger *slog.Logger) *auth.SetupToken {
	if cfg.Server.Insecure {
		return nil
	}
	n, err := store.CountUsers()
	if err != nil {
		logger.Error("failed to count users", "error", err)
		os.Exit(1)
	}
	if n > 0 {
		return nil
	}
	token, err := auth.NewSetupToken()
	if err != nil {
		logger.Error("failed to generate setup token", "error", err)
		os.Exit(1)
	}
	logger.Warn("FIRST-RUN SETUP REQUIRED: no users exist yet",
		"url", fmt.Sprintf("http://localhost:%d/setup", cfg.Server.Port),
		"token", token.String(),
	)
	return token
}

// runBackup writes a consistent copy of the database and verifies it.
//
// Verification is not optional here: a corrupt SQLite file opens and answers
// simple queries perfectly well, so a backup that is never checked is a backup
// nobody knows is broken until they need it.
func runBackup(dbPath, destPath string) error {
	store, err := storage.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Backup(destPath); err != nil {
		return err
	}

	verify, err := storage.NewStore(destPath)
	if err != nil {
		return fmt.Errorf("reopen backup: %w", err)
	}
	defer func() { _ = verify.Close() }()

	result, err := verify.IntegrityCheck()
	if err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("backup failed its integrity check: %s", result)
	}

	fmt.Printf("backup written to %s (integrity check: %s)\n", destPath, result)
	return nil
}
