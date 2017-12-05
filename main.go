// main
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/urfave/cli"
)

func main() {
	netInterfaceName := ""
	netInterface := (*net.Interface)(nil)
	address := ""
	datagramSize := 1500
	ttl := 8
	loopbackEnable := false

	createMulticast := func() (*Multicast, error) {
		m, err := NewMulticast(address, netInterface)
		if err != nil {
			return nil, err
		}

		m.SetDatagramSize(datagramSize)
		if err != nil {
			return nil, err
		}
		m.SetTTL(ttl)
		if err != nil {
			return nil, err
		}
		m.SetLoopback(loopbackEnable)
		if err != nil {
			return nil, err
		}
		return m, nil
	}

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
				m, err := createMulticast()
				if err != nil {
					return err
				}

				//local := c.Args().First()

				ack := []byte("ack")

				go m.DataReceiveLoop()
				go m.ControlReceiveLoop()

				// Read UDP messages from multicast:
				for {
					select {
					case ctrl := <-m.Control:
						if ctrl.Error != nil {
							return ctrl.Error
						}
						fmt.Printf("ctrlrecv\n%s", hex.Dump(ctrl.Data))
						if len(ctrl.Data) >= 1 {
							switch ctrl.Data[0] {
							case 0x01:
								hashId := ctrl.Data[1:]
								fmt.Printf("announcement: %v\n", hashId)
							}
						}
					case msg := <-m.Data:
						if msg.Error != nil {
							return msg.Error
						}
						fmt.Printf("datarecv\n%s", hex.Dump(msg.Data))

						_, err := m.SendControl(ack)
						if err != nil {
							return err
						}
						fmt.Printf("ctrlsent\n%s", hex.Dump(ack))
					}
				}

				err = m.controlConn.Close()
				return err
			},
		},
		cli.Command{
			Name:    "serve",
			Aliases: []string{"s"},
			Usage:   "serve files to a multicast group",
			Action: func(c *cli.Context) error {
				args := c.Args()
				if !args.Present() {
					return errors.New("Required arguments for files to serve")
				}

				files := make([]TarballFile, 0, len(args))
				for _, a := range args {
					stat, err := os.Stat(a)
					if os.IsNotExist(err) {
						continue
					}
					if err != nil {
						return err
					}

					// Add file to virtual tarball list:
					files = append(files, TarballFile{
						Path: a,
						Size: stat.Size(),
						Mode: stat.Mode(),
					})
				}
				if len(files) == 0 {
					return errors.New("no files to serve")
				}

				// Treat collection of files as virtual tarball for reading:
				tb, err := NewTarball(files)
				defer tb.Close()

				err = tb.HashFiles()
				if err != nil {
					return err
				}

				m, err := createMulticast()
				if err != nil {
					return err
				}

				// Create server and run loop:
				s := NewServer(m, tb)
				return s.Run()
			},
		},
	}

	app.RunAndExitOnError()
	return
}
