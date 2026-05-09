package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jianfengxu/has-device-simulator/internal/app"
	"github.com/jianfengxu/has-device-simulator/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("device simulator exited with error", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command, flagArgs := parseCLIArgs(args)
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "usage: device-simulator [run|bind|scenario|event] --config <path> [--name <scenario-or-event>]\n")
	}
	configPath := fs.String("config", "./configs/sl100-dev.yaml", "path to config file")
	scenarioName := fs.String("name", "bind-online", "scenario or event name")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	simulator, err := app.New(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch command {
	case "run":
		return simulator.Run(ctx)
	case "bind":
		return simulator.Bind(ctx)
	case "scenario":
		return simulator.RunScenario(ctx, *scenarioName)
	case "event":
		return simulator.TriggerEvent(ctx, *scenarioName)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func parseCLIArgs(args []string) (command string, flagArgs []string) {
	if len(args) == 0 {
		return "run", nil
	}

	first := args[0]
	switch first {
	case "run", "bind", "scenario", "event":
		return first, args[1:]
	default:
		if len(first) > 0 && first[0] == '-' {
			return "run", args
		}
		return first, args[1:]
	}
}
