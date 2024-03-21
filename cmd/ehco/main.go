package main

import (
	"os"
	"time"

	"github.com/Ehco1996/ehco/internal/cli"
	sentry "github.com/getsentry/sentry-go"
)

func main() {
	defer func() {
		err := recover()
		if err != nil {
			sentry.CurrentHub().Recover(err)
			sentry.Flush(time.Second * 5)
		}
	}()
	app := cli.CreateCliAPP()
	if err := app.Run(os.Args); err != nil {
		println("Run app meet err=", err.Error())
	}
}
