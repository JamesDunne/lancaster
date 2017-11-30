// main
package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/urfave/cli"
)

func main() {
	address := ""
	datagramSize := 1500

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
		cli.IntFlag{
			Name:        "datagram size,s",
			Value:       1500,
			Destination: &datagramSize,
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
				if err != nil {
					return err
				}

				conn, err := net.ListenMulticastUDP("udp", nil, udpAddr)
				if err != nil {
					return err
				}

				conn.SetReadBuffer(datagramSize)
				buf := make([]byte, datagramSize)

				// Read UDP messages from multicast:
				for {
					// TODO: use second parameter *net.UDPAddr to authenticate source?
					n, _, err := conn.ReadFromUDP(buf)
					if err != nil {
						return err
					}
					msg := buf[:n]
					fmt.Printf("%s", hex.Dump(msg))
				}

				err = conn.Close()
				return err
			},
		},
		cli.Command{
			Name:    "serve",
			Aliases: []string{"s"},
			Usage:   "server files to a multicast group",
			Action: func(c *cli.Context) error {
				//local := c.Args().First()
				udpAddr, err := net.ResolveUDPAddr("udp", address)
				if err != nil {
					return err
				}

				conn, err := net.DialUDP("udp", nil, udpAddr)
				if err != nil {
					return err
				}

				conn.SetWriteBuffer(datagramSize)
				msg := []byte("hello, world!\n")

				// Write UDP messages to multicast:
				for {
					_, err := conn.Write(msg)
					if err != nil {
						return err
					}
					fmt.Printf("%s", hex.Dump(msg))
					time.Sleep(1 * time.Second)
				}

				err = conn.Close()
				return err
			},
		},
	}

	app.RunAndExitOnError()
	return
}
