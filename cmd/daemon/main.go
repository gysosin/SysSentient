package main

import (
	"fmt"
	"log"
	"time"

	"sys-sentient/internal/collector"
	"sys-sentient/internal/storage"
)

func main() {
	fmt.Println("Starting SysSentient Daemon...")

	// 1. Initialize Storage
	// Use local db for development
	store, err := storage.NewStore("./sys-sentient.db")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()
	fmt.Println("Storage initialized at ./sys-sentient.db")

	// 2. Initialize Collector
	col := collector.NewCollector()
	fmt.Println("Collector initialized. Starting polling loop (2s)...")

	// 3. Polling Loop
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
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

		// Log to console (proof of life and verification)
		fmt.Printf("[%s] CPU: %.2f%% | RAM: %d/%d MB | Procs: %s\n",
			state.Timestamp.Format(time.TimeOnly),
			state.CPUUsage,
			state.MemoryUsed/1024/1024,
			state.MemoryTotal/1024/1024,
			state.TopProcesses,
		)
	}
}
