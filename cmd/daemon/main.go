package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sys-sentient/internal/ai"
	"sys-sentient/internal/collector"
	"sys-sentient/internal/config"
	"sys-sentient/internal/logs"
	"sys-sentient/internal/pii"
	"sys-sentient/internal/server"
	"sys-sentient/internal/storage"
)

func main() {
	fmt.Println("Starting SysSentient Daemon...")

	// 1. Load Configuration
	cfg, err := config.LoadConfig("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Config loaded. Poll interval: %ds\n", cfg.Collector.PollIntervalSeconds)
	fmt.Printf("Server Port: %d\n", cfg.Server.Port)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Initialize Storage
	store, err := storage.NewStore(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()
	fmt.Printf("Storage initialized at %s\n", cfg.Database.Path)

	// 3. Initialize AI Service
	aiService, err := ai.NewAIService(ctx, cfg.Gemini)
	if err != nil {
		log.Printf("AI Service disabled: %v", err)
	} else {
		fmt.Println("AI Service initialized.")
	}

	// 4. Start API Server
	srv := server.NewServer(cfg.Server, store, aiService)
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	// 5. Initialize PII Scrubber
	scrubber := pii.NewScrubber(cfg.Privacy.MaskIPs, cfg.Privacy.MaskEmails, cfg.Privacy.MaskUsernames)

	// 6. Initialize Log Reader
	logReader := logs.NewLogReader(50) // max 50 log lines
	fmt.Println("Log reader initialized.")

	// 7. Initialize Collector
	col := collector.NewCollector()
	fmt.Println("Collector initialized. Starting polling loop...")

	// 8. Polling Loop
	interval := time.Duration(cfg.Collector.PollIntervalSeconds) * time.Second
	if interval == 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Database Maintenance Ticker (Every 1 hour)
	dbTicker := time.NewTicker(1 * time.Hour)
	defer dbTicker.Stop()

	// Analysis Cooldown
	lastAnalysisTime := time.Time{}
	analysisCooldown := 5 * time.Minute // Don't analyze more than once every 5 minutes

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutdown signal received. Stopping SysSentient daemon...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Error shutting down API server: %v", err)
			}
			return

		case err, ok := <-serverErr:
			if ok && err != nil {
				log.Printf("API Server failed: %v", err)
			}
			return

		case <-dbTicker.C:
			if err := store.PruneOldMetrics(cfg.Database.MetricsRetentionHours); err != nil {
				log.Printf("Error pruning old metrics: %v", err)
			}
			if err := store.PruneOldInsights(cfg.Database.InsightsRetentionHours); err != nil {
				log.Printf("Error pruning old insights: %v", err)
			}

		case <-ticker.C:
			// Collect
			state, err := col.Collect()
			if err != nil {
				log.Printf("Error collecting metrics: %v", err)
				continue
			}

			// Save
			if err := store.Save(state); err != nil {
				log.Printf("Error saving metrics: %v", err)
				continue
			}

			// Broadcast to WebSocket clients
			if err := srv.Hub.BroadcastMetrics(state); err != nil {
				log.Printf("Error broadcasting metrics: %v", err)
			}

			// Log to console
			fmt.Printf("[%s] CPU: %.2f%% | RAM: %d/%d MB | Load: %.2f | Procs: %s\n",
				state.Timestamp.Format(time.TimeOnly),
				state.CPUUsage,
				state.MemoryUsed/1024/1024,
				state.MemoryTotal/1024/1024,
				state.LoadAvg1,
				state.TopProcesses,
			)

			// Check Triggers for AI Analysis
			if aiService != nil {
				// Trigger conditions: High CPU (>80%) or High Memory (>90%)
				isHighCPU := state.CPUUsage > 80.0
				isHighMem := float64(state.MemoryUsed)/float64(state.MemoryTotal) > 0.9

				if (isHighCPU || isHighMem) && time.Since(lastAnalysisTime) > analysisCooldown {
					fmt.Println("⚠️  Threshold Triggered! Requesting AI Analysis...")
					lastAnalysisTime = time.Now()

					go func(ctx context.Context) {
						// Collect real system logs with timeout
						rawLogs, err := logReader.GetLogsWithTimeout(5 * time.Second)
						if err != nil {
							log.Printf("Warning: Failed to collect logs: %v", err)
							rawLogs = "Failed to collect system logs."
						}
						scrubbedLogs := scrubber.SanitizeLog(rawLogs)

						insight, err := aiService.AnalyzeSystemState(ctx, *state, scrubbedLogs)
						if err != nil {
							log.Printf("Error analyzing system state: %v", err)
							return
						}

						fmt.Printf("🤖 AI Insight: %s\n", insight)
						// Save is handled inside AnalyzeSystemState (RAG cache) or externally?
						// Wait, RAG cache saves it to cache, but we also want to save to DB for history.
						if err := store.SaveInsight(insight); err != nil {
							log.Printf("Error saving insight: %v", err)
						}
					}(ctx)
				}
			}
		}
	}
}
