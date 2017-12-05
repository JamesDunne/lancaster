// main
package main

import (
	"errors"
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

				cl := NewClient(m)
				return cl.Run()
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
