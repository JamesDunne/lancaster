// main
package main

import (
	"net"
	"os"

	"github.com/urfave/cli"
)

func main() {
	address := ""

	app := cli.NewApp()

	app.Name = "lancaster"
	app.Description = "UDP multicast file transfer client and server"
	app.Version = "1.0.0"
	app.Authors = []cli.Author{
		{Name: "James Dunne", Email: "james.jdunne@gmail.com"},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "group,g",
			Value:       "224.0.0.1:1360",
			Usage:       "UDP multicast group for transfers",
			Destination: &address,
		},
	}
	app.Commands = []cli.Command{
		cli.Command{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download files from a multicast group locally",
			Action: func(c *cli.Context) error {
				//local := c.Args().First()
				udpAddr, err := net.ResolveUDPAddr("udp", address)
				conn, err := net.ListenMulticastUDP("udp", nil, udpAddr)
				err = conn.Close()
				return err
			},
		},
	}

	app.Run(os.Args)
	return
}
