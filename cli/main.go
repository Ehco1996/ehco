package main

import (
	"log"
	"os"

	cli "github.com/urfave/cli/v2"

	ehco "github.com/Ehco1996/ehco"
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
	r, err := ehco.NewRelay("127.0.0.1:1234", "127.0.0.1:9001")
	if err != nil {
		log.Fatal(err)
	}
	return r.ListenAndServe()
}
