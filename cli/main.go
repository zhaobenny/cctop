package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kardianos/service"
	"github.com/zhaobenny/cctop/cli/internal/aggregator"
	"github.com/zhaobenny/cctop/cli/internal/config"
	"github.com/zhaobenny/cctop/cli/internal/output"
	"github.com/zhaobenny/cctop/cli/internal/sync"
	"github.com/zhaobenny/cctop/internal/model"
	"github.com/zhaobenny/cctop/cli/internal/parser"
)

const version = "0.2.0"

func main() {
	// Detect subcommand first
	command := "daily"
	args := os.Args[1:]

	// Find and extract the subcommand from args
	var filteredArgs []string
	for i, arg := range args {
		switch arg {
		case "daily", "monthly", "session", "blocks", "sync", "config":
			command = arg
			// Keep remaining args for flag parsing
			filteredArgs = append(args[:i], args[i+1:]...)
		}
		if command != "daily" || arg == "daily" {
			break
		}
	}
	if filteredArgs == nil {
		filteredArgs = args
	}

	// Handle special commands
	switch command {
	case "sync":
		runSync(filteredArgs)
		return
	case "config":
		runConfig(filteredArgs)
		return
	}

	// Create a new FlagSet for clean parsing
	fs := flag.NewFlagSet("cctop", flag.ExitOnError)

	var (
		since     string
		until     string
		timezone  string
		jsonOut   bool
		breakdown bool
		compact   bool
		offline   bool
		showHelp  bool
		showVer   bool
	)

	fs.StringVar(&since, "since", "", "Start date filter (YYYYMMDD)")
	fs.StringVar(&until, "until", "", "End date filter (YYYYMMDD)")
	fs.StringVar(&timezone, "timezone", "", "Timezone for date grouping (e.g., America/New_York)")
	fs.BoolVar(&jsonOut, "json", false, "Output as JSON")
	fs.BoolVar(&breakdown, "breakdown", false, "Show per-model breakdown")
	fs.BoolVar(&compact, "compact", false, "Force compact table output")
	fs.BoolVar(&compact, "c", false, "Force compact table output")
	fs.BoolVar(&offline, "offline", false, "Use embedded pricing data (no network)")
	fs.BoolVar(&showHelp, "help", false, "Show help")
	fs.BoolVar(&showHelp, "h", false, "Show help")
	fs.BoolVar(&showVer, "version", false, "Show version")
	fs.BoolVar(&showVer, "v", false, "Show version")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `cctop - Claude Code Token Overview Program

Usage: cctop [command] [options]

Commands:
  daily     Show daily usage report (default)
  monthly   Show monthly usage report
  session   Show usage by session
  blocks    Show usage by 5-hour billing blocks
  sync      Sync usage data to server
  config    Configure sync settings

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  cctop                      Show daily usage
  cctop daily --since 20250101
  cctop monthly --json
  cctop session --breakdown
  cctop blocks
  cctop config --server https://example.com --api-key <key>
  cctop sync
`)
	}

	fs.Parse(filteredArgs)

	if showVer {
		fmt.Printf("cctop version %s\n", version)
		return
	}

	if showHelp {
		fs.Usage()
		return
	}

	// Parse dates
	opts := aggregator.Options{
		Offline: offline,
	}

	if since != "" {
		t, err := time.Parse("20060102", since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid --since date format. Use YYYYMMDD.\n")
			os.Exit(1)
		}
		opts.Since = t
	}

	if until != "" {
		t, err := time.Parse("20060102", until)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid --until date format. Use YYYYMMDD.\n")
			os.Exit(1)
		}
		// Include the entire day
		opts.Until = t.Add(24*time.Hour - time.Second)
	}

	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid timezone: %s\n", timezone)
			os.Exit(1)
		}
		opts.Timezone = loc
	}

	// Load and parse all usage data
	records, err := parser.ParseAllFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading usage data: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("No usage data found in ~/.claude/projects/")
		return
	}

	// Filter by date range
	records = aggregator.FilterRecords(records, opts)

	if len(records) == 0 {
		fmt.Println("No usage data found for the specified date range.")
		return
	}

	// Aggregate based on command
	var results []model.AggregatedUsage
	var title string

	switch command {
	case "daily":
		results = aggregator.ByDay(records, opts)
		title = "Date"
	case "monthly":
		results = aggregator.ByMonth(records, opts)
		title = "Month"
	case "session":
		results = aggregator.BySession(records, opts)
		title = "Session"
	case "blocks":
		results = aggregator.ByBlock(records, opts)
		title = "Block"
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		fs.Usage()
		os.Exit(1)
	}

	// Output results
	opts2 := output.TableOptions{ForceCompact: compact}

	if jsonOut {
		output.PrintJSON(results)
	} else if breakdown {
		output.PrintTableWithBreakdownOpts(results, title, opts2)
	} else {
		output.PrintTableWithOptions(results, title, true, opts2)
	}
}

func runConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	var (
		server string
		apiKey string
		show   bool
	)
	fs.StringVar(&server, "server", "", "Server URL")
	fs.StringVar(&apiKey, "api-key", "", "API key for authentication")
	fs.BoolVar(&show, "show", false, "Show current configuration")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: cctop config [options]

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  cctop config --server https://example.com --api-key cctop_xxx
  cctop config --show
`)
	}

	fs.Parse(args)

	if show {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		if cfg.Server == "" {
			fmt.Println("No configuration found. Run 'cctop config --server <url> --api-key <key>' to configure.")
			return
		}
		fmt.Printf("Server: %s\n", cfg.Server)
		fmt.Printf("API Key: %s...%s\n", cfg.APIKey[:10], cfg.APIKey[len(cfg.APIKey)-4:])
		if cfg.ClientID != "" {
			fmt.Printf("Client ID: %s\n", cfg.ClientID)
		}
		return
	}

	if server == "" && apiKey == "" {
		fs.Usage()
		return
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	if server != "" {
		cfg.Server = server
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}

	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Configuration saved.")
}

// syncService implements service.Interface for background syncing
type syncService struct {
	interval time.Duration
	stop     chan struct{}
	logger   service.Logger
}

func (s *syncService) Start(svc service.Service) error {
	s.stop = make(chan struct{})
	go s.run()
	return nil
}

func (s *syncService) Stop(svc service.Service) error {
	close(s.stop)
	return nil
}

func (s *syncService) run() {
	cfg, err := config.Load()
	if err != nil || cfg.Server == "" || cfg.APIKey == "" {
		if s.logger != nil {
			s.logger.Error("Not configured. Run 'cctop config' first.")
		}
		return
	}

	client := sync.NewClient(cfg)

	// Sync immediately on start
	s.doSync(client)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.doSync(client)
		case <-s.stop:
			return
		}
	}
}

func (s *syncService) doSync(client *sync.Client) {
	lastSync, _ := client.GetSyncStatus()

	records, err := parser.ParseAllFiles()
	if err != nil {
		if s.logger != nil {
			s.logger.Errorf("Error reading usage data: %v", err)
		}
		return
	}

	var toSync []model.UsageRecord
	for _, r := range records {
		if lastSync == nil || r.Timestamp.After(*lastSync) {
			toSync = append(toSync, r)
		}
	}

	if len(toSync) == 0 {
		return
	}

	inserted, err := client.Sync(toSync)
	if err != nil {
		if s.logger != nil {
			s.logger.Errorf("Error syncing: %v", err)
		}
		return
	}

	if s.logger != nil {
		s.logger.Infof("Synced %d records", inserted)
	}
}

func runSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	var (
		dryRun   bool
		interval time.Duration
	)
	fs.BoolVar(&dryRun, "dry-run", false, "Show what would be synced without sending")
	fs.DurationVar(&interval, "interval", time.Hour, "Sync interval for service mode (e.g., 1h, 30m)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: cctop sync [command] [options]

Commands:
  (none)      Sync once
  install     Install as a background service
  start       Start the background service
  stop        Stop the background service
  uninstall   Remove the background service
  status      Show service status

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  cctop sync                       Sync once
  cctop sync install               Install service (syncs every hour)
  cctop sync install --interval 30m
  cctop sync start                 Start the service
  cctop sync stop                  Stop the service
`)
	}

	// Check for service commands before parsing flags
	var svcCommand string
	if len(args) > 0 {
		switch args[0] {
		case "install", "start", "stop", "uninstall", "status":
			svcCommand = args[0]
			args = args[1:]
		}
	}

	fs.Parse(args)

	// Create service config
	svcConfig := &service.Config{
		Name:        "cctop-sync",
		DisplayName: "cctop Sync Service",
		Description: "Automatically syncs Claude Code usage data to server",
		Arguments:   []string{"sync", "run", fmt.Sprintf("--interval=%s", interval)},
	}

	svc := &syncService{interval: interval}
	s, err := service.New(svc, svcConfig)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	// Handle service commands
	switch svcCommand {
	case "install":
		cfg, err := config.Load()
		if err != nil || cfg.Server == "" || cfg.APIKey == "" {
			fmt.Fprintf(os.Stderr, "Error: Not configured. Run 'cctop config --server <url> --api-key <key>' first.\n")
			os.Exit(1)
		}
		if err := s.Install(); err != nil {
			log.Fatalf("Failed to install service: %v", err)
		}
		if err := s.Start(); err != nil {
			log.Fatalf("Service installed but failed to start: %v", err)
		}
		fmt.Printf("Service installed and started.\n")
		fmt.Printf("Sync interval: %s\n", interval)
		return

	case "start":
		if err := s.Start(); err != nil {
			log.Fatalf("Failed to start service: %v", err)
		}
		fmt.Println("Service started.")
		return

	case "stop":
		if err := s.Stop(); err != nil {
			log.Fatalf("Failed to stop service: %v", err)
		}
		fmt.Println("Service stopped.")
		return

	case "uninstall":
		s.Stop() // ignore error
		if err := s.Uninstall(); err != nil {
			log.Fatalf("Failed to uninstall service: %v", err)
		}
		fmt.Println("Service uninstalled.")
		return

	case "status":
		status, err := s.Status()
		if err != nil {
			fmt.Printf("Service status: not installed or error (%v)\n", err)
		} else {
			switch status {
			case service.StatusRunning:
				fmt.Println("Service status: running")
			case service.StatusStopped:
				fmt.Println("Service status: stopped")
			default:
				fmt.Println("Service status: unknown")
			}
		}
		return

	case "": // No service command - do a one-time sync
		cfg, err := config.Load()
		if err != nil || cfg.Server == "" || cfg.APIKey == "" {
			fmt.Fprintf(os.Stderr, "Error: Not configured. Run 'cctop config --server <url> --api-key <key>' first.\n")
			os.Exit(1)
		}

		client := sync.NewClient(cfg)
		doSyncOnce(client, dryRun)
		return

	default:
		// Running as service (internal command)
		logger, err := s.Logger(nil)
		if err == nil {
			svc.logger = logger
		}
		if err := s.Run(); err != nil {
			logger.Error(err)
		}
	}
}

func doSyncOnce(client *sync.Client, dryRun bool) {
	lastSync, err := client.GetSyncStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not get sync status: %v\n", err)
	}

	records, err := parser.ParseAllFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading usage data: %v\n", err)
		os.Exit(1)
	}

	var toSync []model.UsageRecord
	for _, r := range records {
		if lastSync == nil || r.Timestamp.After(*lastSync) {
			toSync = append(toSync, r)
		}
	}

	if len(toSync) == 0 {
		fmt.Println("No new records to sync.")
		return
	}

	fmt.Printf("Found %d new records to sync.\n", len(toSync))

	if dryRun {
		fmt.Println("Dry run - no data sent.")
		return
	}

	inserted, err := client.Sync(toSync)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Sync complete. %d records inserted.\n", inserted)
}
