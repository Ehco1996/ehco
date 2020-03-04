package main

import (
	"log"
	"os"

	cli "github.com/urfave/cli/v2"

	ehco "github.com/Ehco1996/ehco"
)

var LOCAL_ADDR string
var REMOTE_ADDR string

func main() {
	app := cli.NewApp()
	app.Name = "ehco"
	app.Version = "0.0.5"
	app.Usage = "A proxy used to relay tcp/udp traffic to anywhere"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "l, local",
			Value:       "0.0.0.0:1234",
			Usage:       "监听地址",
			EnvVars:     []string{"EHCO_LOCAL_ADDR"},
			Destination: &LOCAL_ADDR,
		},
		&cli.StringFlag{
			Name:        "r,remote",
			Value:       "0.0.0.0:9001",
			Usage:       "转发地址",
			EnvVars:     []string{"EHCO_REMOTE_ADDR"},
			Destination: &REMOTE_ADDR,
		},
		&cli.BoolFlag{
			Name:        "d,debug",
			Value:       false,
			Usage:       "转发地址",
			EnvVars:     []string{"EHCO_DEBUG"},
			Destination: &ehco.DEBUG,
		},
	}

	app.Action = start
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

func start(ctx *cli.Context) error {
	r, err := ehco.NewRelay(LOCAL_ADDR, REMOTE_ADDR)
	if err != nil {
		log.Fatal(err)
	}
	return r.ListenAndServe()
}
