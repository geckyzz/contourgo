package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/geckyzz/contourgo/internal/config"
	"github.com/geckyzz/contourgo/internal/db"
	"github.com/geckyzz/contourgo/internal/discord"
	"github.com/geckyzz/contourgo/internal/monitor"
	"github.com/mattn/go-isatty"
)

func main() {
	configPath := flag.String("config", "config.toml", "Path to config TOML file")
	dbPath := flag.String("db", "bot.db", "Path to SQLite database file")
	dumpComments := flag.Bool("dump-comments", false, "Initialize database without sending Discord notifications")
	flag.Parse()

	log.Println("Starting Contour Go bot...")

	// 0. Setup logging to file and stdout
	logFile, err := os.OpenFile("bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		mw := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(mw)
	} else {
		log.Printf("Warning: could not open log file: %v", err)
	}

	// 1. Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	log.Printf("Loaded config from: %s", *configPath)
	cfg.LogConfigSummary()

	// 2. Init DB
	database, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("Error initializing DB: %v", err)
	}
	defer database.Close()
	log.Printf("Initialized SQLite database at: %s", *dbPath)

	// 3. Init Discord Bot
	forceCheckChan := make(chan bool, 1)
	bot, err := discord.NewDiscordBot(cfg, *configPath, database, forceCheckChan)
	if err != nil {
		log.Fatalf("Error creating Discord bot: %v", err)
	}

	err = bot.Start()
	if err != nil {
		log.Fatalf("Error starting Discord bot: %v", err)
	}
	defer bot.Stop()
	log.Println("Discord bot connected and running.")

	// 4. Start Monitor Loop (in goroutine)
	mon := monitor.NewMonitor(cfg, database, bot, forceCheckChan)
	mon.DumpComments = *dumpComments
	go mon.Start()

	// Wait for terminate signals or Ctrl+D on stdin (if run in a terminal)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	// We only listen to Stdin EOF if Stdin is a terminal.
	// This prevents immediate shutdown when running in non-interactive environments (e.g. Docker, systemd, or redirection).
	exitChan := make(chan struct{})
	if isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		go func() {
			buf := make([]byte, 1024)
			for {
				_, err := os.Stdin.Read(buf)
				if err != nil {
					log.Println("Ctrl+D or EOF received on stdin. Exiting...")
					close(exitChan)
					return
				}
			}
		}()
	}

	select {
	case <-sc:
		log.Println("Received termination signal.")
	case <-exitChan:
	}

	log.Println("Shutting down bot gracefully...")
}
