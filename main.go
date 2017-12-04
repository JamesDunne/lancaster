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
	netInterfaceName := ""
	netInterface := (*net.Interface)(nil)
	address := ""
	datagramSize := 1500
	ttl := 8
	loopbackEnable := false

	app := cli.NewApp()

	app.Name = "lancaster"
	app.Description = "UDP multicast file transfer client and server"
	app.Version = "1.0.0"
	app.Authors = []cli.Author{
		{Name: "James Dunne", Email: "james.jdunne@gmail.com"},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "interface,i",
			Value:       "",
			Usage:       "Interface name to bind to",
			Destination: &netInterfaceName,
		},
		cli.StringFlag{
			Name:        "group,g",
			Value:       "236.0.0.100:1360",
			Usage:       "UDP multicast group for transfers",
			Destination: &address,
		},
		cli.IntFlag{
			Name:        "datagram size,s",
			Value:       1500,
			Destination: &datagramSize,
		},
		cli.IntFlag{
			Name:        "ttl,t",
			Value:       8,
			Destination: &ttl,
		},
		cli.BoolFlag{
			Name:        "loopback enable,l",
			Destination: &loopbackEnable,
		},
	}
	app.Before = func(c *cli.Context) error {
		// Find network interface by name:
		if netInterfaceName != "" {
			var err error
			netInterface, err = net.InterfaceByName(netInterfaceName)
			if err != nil {
				return err
			}
		}
		return nil
	}
	app.Commands = []cli.Command{
		cli.Command{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download files from a multicast group locally",
			Action: func(c *cli.Context) error {
				m, err := NewMulticast(address, netInterface)
				if err != nil {
					return err
				}

				m.SetDatagramSize(datagramSize)
				if err != nil {
					return err
				}
				m.SetTTL(ttl)
				if err != nil {
					return err
				}
				m.SetLoopback(loopbackEnable)
				if err != nil {
					return err
				}
				//local := c.Args().First()

				buf := make([]byte, datagramSize)
				ack := []byte("ack")

				// Read UDP messages from multicast:
				for {
					// TODO: use second parameter *net.UDPAddr to authenticate source?
					n, err := m.RecvData(buf)
					if err != nil {
						return err
					}
					msg := buf[:n]
					fmt.Printf("datarecv %s", hex.Dump(msg))

					n, err = m.SendControl(ack)
					if err != nil {
						return err
					}
					fmt.Printf("ctrlsent %s", hex.Dump(ack))
				}

				err = m.controlConn.Close()
				return err
			},
		},
		cli.Command{
			Name:    "serve",
			Aliases: []string{"s"},
			Usage:   "server files to a multicast group",
			Action: func(c *cli.Context) error {
				local := c.Args().First()
				fmt.Printf("%s\n", local)

				m, err := NewMulticast(address, netInterface)
				if err != nil {
					return err
				}

				m.SetDatagramSize(datagramSize)
				if err != nil {
					return err
				}
				m.SetTTL(ttl)
				if err != nil {
					return err
				}
				m.SetLoopback(loopbackEnable)
				if err != nil {
					return err
				}

				ctrl := make(chan []byte)
				ctrlErr := make(chan error)

				// Start a message receive loop:
				go func() {
					for {
						buf := make([]byte, datagramSize)
						n, err := m.RecvControl(buf)
						if err != nil {
							ctrlErr <- err
							return
						}
						ctrl <- buf[0:n]
					}
				}()

				ticker := time.Tick(time.Second)

				msgo := []byte("hello, world!\n")
				// Send/recv loop:
				for {
					select {
					case msgi := <-ctrl:
						fmt.Printf("ctrlrecv %s", hex.Dump(msgi))
					case err = <-ctrlErr:
						break
					case <-ticker:
						_, err := m.SendData(msgo)
						if err != nil {
							return err
						}
						fmt.Printf("datasent %s", hex.Dump(msgo))
					}
				}

				err = m.controlConn.Close()
				return err
			},
		},
	}

	app.RunAndExitOnError()
	return
}
