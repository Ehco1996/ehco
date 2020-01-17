package main

import (
	"log"
	"os"

	ehco "github.com/Ehco1996/ehco"
	cli "github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()
	app.Name = "ehco"
	app.Version = "0.0.2"
	app.Usage = "A proxy used to relay tcp/udp traffic to anywhere"
	app.Action = start

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func start(ctx *cli.Context) error {
	r, err := ehco.NewRelay("127.0.0.1:1234", "127.0.0.1:9001", 60, 60)
	if err != nil {
		log.Fatal(err)
	}
	return r.ListenAndServe()
}
