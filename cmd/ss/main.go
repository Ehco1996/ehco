package main

import (
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/Ehco1996/ehco/internal/ss"

	"github.com/shadowsocks/go-shadowsocks2/core"
	cli "github.com/urfave/cli/v2"
)

var SsUri string

func main() {
	app := cli.NewApp()
	app.Name = "ss"
	app.Version = "0.1.6"
	app.Usage = "ss :)"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "uri",
			Usage:       "ss",
			Value:       "ss://AEAD_CHACHA20_POLY1305:zxczxc123@0.0.0.0:5555",
			EnvVars:     []string{"EHCO_SS_URI"},
			Destination: &SsUri,
		},
	}

	app.Action = start
	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}

}

func start(ctx *cli.Context) error {
	addr, cipher, password, err := parseURL(SsUri)
	ciph, err := core.PickCipher(cipher, nil, password)
	if err != nil {
		return err
	}
	go ss.TcpRemote(addr, ciph.StreamConn)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	return nil
}

func parseURL(s string) (addr, cipher, password string, err error) {
	u, err := url.Parse(s)
	if err != nil {
		return
	}

	addr = u.Host
	if u.User != nil {
		cipher = u.User.Username()
		password, _ = u.User.Password()
	}
	return
}
