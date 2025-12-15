package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"sys-sentient/internal/ai"
	"sys-sentient/internal/collector"
	"sys-sentient/internal/config"
	"sys-sentient/internal/pii"
	"sys-sentient/internal/server"
	"sys-sentient/internal/storage"
)

func main() {
	fmt.Println("Starting SysSentient Daemon...")

	// 1. Load Configuration
	cfg, err := config.LoadConfig("")
	if err != nil {
		log.Printf("Warning: Failed to load config, using defaults: %v", err)
		cfg, _ = config.LoadConfig("") // re-load defaults effectively
	}
	fmt.Printf("Config loaded. Poll interval: %ds\n", cfg.Collector.PollIntervalSeconds)
	fmt.Printf("Server Port: %d\n", cfg.Server.Port)

	// 2. Initialize Storage
	store, err := storage.NewStore(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()
	fmt.Printf("Storage initialized at %s\n", cfg.Database.Path)

	// 3. Initialize AI Service
	ctx := context.Background()
	aiService, err := ai.NewAIService(ctx, cfg.Gemini)
	if err != nil {
		log.Printf("AI Service disabled: %v", err)
	} else {
		fmt.Println("AI Service initialized.")
	}

	// 4. Start API Server
	srv := server.NewServer(cfg.Server, store, aiService)
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("API Server failed: %v", err)
		}
	}()

	// 5. Initialize PII Scrubber
	scrubber := pii.NewScrubber(cfg.Privacy.MaskIPs, cfg.Privacy.MaskEmails, cfg.Privacy.MaskUsernames)

	// 6. Initialize Collector
	col := collector.NewCollector()
	fmt.Println("Collector initialized. Starting polling loop...")

	// 7. Polling Loop
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
		case <-dbTicker.C:
			// Prune metrics older than 24 hours to keep DB lightweight
			if err := store.PruneOldMetrics(24); err != nil {
				log.Printf("Error pruning old metrics: %v", err)
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

			// Log to console
			fmt.Printf("[%s] CPU: %.2f%% | RAM: %d/%d MB | Procs: %s\n",
				state.Timestamp.Format(time.TimeOnly),
				state.CPUUsage,
				state.MemoryUsed/1024/1024,
				state.MemoryTotal/1024/1024,
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

					go func() {
						// Placeholder for log reading
						rawLogs := "Log reading not yet implemented. (No recent errors in dmesg)"
						scrubbedLogs := scrubber.SanitizeLog(rawLogs)

						insight, err := aiService.AnalyzeSystemState(context.Background(), *state, scrubbedLogs)
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
					}()
				}
			}
		}
	}
}
