package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spadeide/spade-rendezvous/internal/config"
	"github.com/spadeide/spade-rendezvous/internal/server"
)

// Version is set via -ldflags at package time.
var Version = "0.1.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("spade-rendezvous: ")

	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "serve":
		if err := runServe(args); err != nil {
			log.Fatal(err)
		}
	case "version", "-v", "--version":
		fmt.Println(Version)
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stderr, `spade-rendezvous — SpadeIDE internet introducer + blind relay

  spade-rendezvous serve     run (reads SPADE_RV_* env)
  spade-rendezvous version

See docs/plans/remote-access-implementation.md §6.
`)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	_ = fs.Parse(args)

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	cfg.Version = Version

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return server.ListenAndServe(ctx, cfg)
}
