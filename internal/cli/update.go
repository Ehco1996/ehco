package cli

import (
	"context"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/updater"
	cli "github.com/urfave/cli/v2"
)

var UpdateCMD = &cli.Command{
	Name:  "update",
	Usage: "update ehco to the latest GitHub release and restart the systemd service",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "force", Usage: "allow downgrade or channel switch"},
		&cli.BoolFlag{Name: "no-restart", Usage: "skip systemctl restart after replacing the binary"},
		&cli.StringFlag{Name: "channel", Value: updater.ChannelAuto, Usage: "auto | stable | nightly"},
	},
	Action: func(c *cli.Context) error {
		ctx, cancel := context.WithTimeout(c.Context, 5*time.Minute)
		defer cancel()
		return updater.Apply(ctx, updater.ApplyOptions{
			Channel: c.String("channel"),
			Force:   c.Bool("force"),
			Restart: !c.Bool("no-restart"),
		}, constant.Version, cliLogger, nil)
	},
}
