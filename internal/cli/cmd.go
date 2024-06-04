package cli

import (
	"fmt"
	"os"
	"os/exec"

	cli "github.com/urfave/cli/v2"
)

const SystemDTMPL = `# Ehco service
[Unit]
Description=ehco
After=network.target

[Service]
LimitNOFILE=65535
ExecStart=ehco -c ""
Restart=always

[Install]
WantedBy=multi-user.target
`

var InstallCMD = &cli.Command{
	Name:  "install",
	Usage: "install ehco systemd service",
	Action: func(c *cli.Context) error {
		fmt.Printf("Install ehco systemd file to `%s`\n", SystemFilePath)
		if _, err := os.Stat(SystemFilePath); err != nil && os.IsNotExist(err) {
			f, _ := os.OpenFile(SystemFilePath, os.O_CREATE|os.O_WRONLY, 0o644)
			if _, err := f.WriteString(SystemDTMPL); err != nil {
				cliLogger.Fatal(err)
			}
			return f.Close()
		}
		command := exec.Command("vi", SystemFilePath)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		return command.Run()
	},
}
