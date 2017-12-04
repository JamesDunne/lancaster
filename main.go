// main
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
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

				ack := []byte("ack")

				go m.DataReceiveLoop()

				// Read UDP messages from multicast:
				for {
					select {
					case msg := <-m.Data:
						if msg.Error != nil {
							return err
						}
						fmt.Printf("datarecv %s", hex.Dump(msg.Data))

						_, err := m.SendControl(ack)
						if err != nil {
							return err
						}
						fmt.Printf("ctrlsent %s", hex.Dump(ack))
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

				go m.ControlReceiveLoop()

				ticker := time.Tick(time.Second)

				msgo := []byte("hello, world!\n")
				// Send/recv loop:
				for {
					select {
					case msgi := <-m.Control:
						if msgi.Error != nil {
							return msgi.Error
						}

						fmt.Printf("ctrlrecv %s", hex.Dump(msgi.Data))
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
