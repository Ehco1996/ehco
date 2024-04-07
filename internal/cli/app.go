package cli

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/pkg/log"
	cli "github.com/urfave/cli/v2"
)

var cliLogger = log.MustNewLogger("info").Sugar().Named("cli")

func startAction(ctx *cli.Context) error {
	cfg, err := InitConfigAndComponents()
	if err != nil {
		cliLogger.Fatalf("InitConfigAndComponents meet err=%s", err.Error())
	}

	mainCtx, stop := signal.NotifyContext(ctx.Context, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	MustStartComponents(mainCtx, cfg)

	<-mainCtx.Done()

	cliLogger.Info("ehco exit now...")
	return nil
}

func CreateCliAPP() *cli.App {
	cli.VersionPrinter = func(c *cli.Context) {
		println("Welcome to ehco (ehco is a network relay tool and a typo)")
		println(fmt.Sprintf("Version=%s", constant.Version))
		println(fmt.Sprintf("GitBranch=%s", constant.GitBranch))
		println(fmt.Sprintf("GitRevision=%s", constant.GitRevision))
		println(fmt.Sprintf("BuildTime=%s", constant.BuildTime))
	}
	app := cli.NewApp()
	app.Name = "ehco"
	app.Flags = RootFlags
	app.Version = constant.Version
	app.Commands = []*cli.Command{InstallCMD}
	app.Usage = "ehco is a network relay tool and a typo :)"
	app.Action = startAction
	return app
}
