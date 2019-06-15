package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/txthinking/brook"
	"github.com/urfave/cli"

	"github.com/Ehco1996/ehco"
)

func main() {
	app := cli.NewApp()
	app.Name = "Ehco"
	app.Version = "0.0.1"
	app.Usage = "A proxy used to relay tcp/udp traffic to anywhere"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Load configuration from json such as 'config.json'",
		},
	}

	app.Action = func(c *cli.Context) error {
		filePath := c.String("config")
		fmt.Println("read config file from:", filePath)

		errch := make(chan error)
		go func() {
			config, err := ehco.ReadConfigFromPath(filePath)
			if err != nil {
				errch <- err
				return
			}
			for i := 0; i < len(config.LocalPorts); i++ {
				ls := config.LocalHost + ":" + strconv.Itoa(config.LocalPorts[i])
				rs := config.RemoteHost + ":" + strconv.Itoa(config.RemotePorts[i])
				fmt.Println(ls, rs)
				go func() {
					fmt.Println(fmt.Sprintf("run relay server at %s to %s", ls, rs))
					// hard code tcptimeout to 60 tcp/udp deadline to 0
					errch <- brook.RunRelay(ls, rs, 60, 0, 0)
				}()
			}
		}()

		return <-errch

	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
