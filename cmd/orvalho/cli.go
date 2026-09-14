package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	lewcmd "github.com/lewtec/lewkit/x/cmd"
)

// orvalhoCLI is the product command tree under lewkit App
// (which already owns -v, --profile-dir, --help, version).
type orvalhoCLI struct {
	DataDir    lewcmd.StringArg `long:"data-dir" help:"host data directory (required for host commands)"`
	Config     lewcmd.StringArg `long:"config" help:"host orvalho.cue path (default: <data-dir>/orvalho.cue)"`
	Script     *scriptCmd       `cmd:"script" help:"run a Script as main (Node-compatible)"`
	Serve      *serveCmd        `cmd:"serve" help:"serve one package over local HTTP"`
	Identity   *identityCmd     `cmd:"identity" help:"manage manager identity key material"`
	ConfigCmd  *configCmd       `cmd:"config" help:"host CUE configuration"`
	Dependency *dependencyCmd   `cmd:"dependency" help:"resolve and install registry Dependencies"`
	Manager    *managerCmd      `cmd:"manager" help:"manager role (pair, sign, deploy, daemon)"`
	Worker     *workerCmd       `cmd:"worker" help:"worker role (actor host on device)"`
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Args = rewriteNodeInvocation(os.Args)
	app, err := lewcmd.Parse[lewcmd.App[orvalhoCLI]](os.Args[1:]...)
	if err != nil {
		return err
	}
	dataDir = app.Args.DataDir.Value()
	configPath = app.Args.Config.Value()
	if app.LogLevel() < slog.LevelInfo {
		verbose = true
		if err := os.Setenv("ORVALHO_VERBOSE", "1"); err != nil {
			return err
		}
	}
	err = app.Run(ctx)
	stop()
	// App.Setup's profile.Run follows ctx; wait for StopCPUProfile.
	time.Sleep(400 * time.Millisecond)
	return err
}

func (s *scriptCmd) Run(context.Context) error {
	return fmt.Errorf("script: use script run")
}

func (c *configCmd) Run(context.Context) error {
	return fmt.Errorf("config: use config validate or config show")
}

func (d *dependencyCmd) Run(context.Context) error {
	return fmt.Errorf("dependency: use install, add, or remove")
}

func (i *identityCmd) Run(context.Context) error {
	return fmt.Errorf("identity: use generate or show")
}
