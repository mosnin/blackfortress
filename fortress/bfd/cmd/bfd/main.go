// Command bfd runs the local Black Fortress runtime.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"blackfortress.dev/fortress/bfd/internal/cli"
	"blackfortress.dev/fortress/bfd/internal/paths"
	"blackfortress.dev/fortress/bfd/internal/runtime"
)

const usage = `bfd — Black Fortress runtime

Usage:
  bfd run        Start PostgreSQL, storage, mail sink, probod and the control API
  bfd status     Show runtime status
  bfd paths      Print the data directory
  bfd version
`

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		os.Exit(run())
	case "status":
		os.Exit(cli.Status(os.Stdout))
	case "paths":
		home, err := paths.Home()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		fmt.Println(home)
	case "version", "--version":
		fmt.Println(runtime.Version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func run() int {
	home, err := paths.Home()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	layout := paths.NewLayout(home)
	if err := layout.Ensure(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	logFile, err := os.OpenFile(filepath.Join(layout.Logs, "bfd.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer logFile.Close()

	logger := log.New(io.MultiWriter(os.Stderr, logFile), "bfd ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Black Fortress %s starting; data in %s", runtime.Version, home)

	// bf down stops the runtime through this file. A stale one from a crash
	// is simply overwritten.
	if err := os.WriteFile(layout.PidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		logger.Printf("cannot write %s: %v", layout.PidFile, err)
	}
	defer os.Remove(layout.PidFile)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	d := runtime.New(runtime.DefaultConfig(), layout, logger)
	if err := d.Run(ctx); err != nil {
		logger.Printf("exited with error: %v", err)
		return 1
	}

	logger.Printf("stopped")

	return 0
}
