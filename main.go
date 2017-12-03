// main
package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/urfave/cli"
)

func main() {
	netInterfaceName := "LAN"
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
			Value:       "LAN",
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
			Value:       true,
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
				//local := c.Args().First()
				udpAddr, err := net.ResolveUDPAddr("udp", address)
				if err != nil {
					return err
				}

				conn, err := net.ListenMulticastUDP("udp", netInterface, udpAddr)
				if err != nil {
					return err
				}

				// Set advanced options for TTL and loopback:
				sysconn, err := conn.SyscallConn()
				if err != nil {
					return err
				}
				sysconn.Control(func(fd uintptr) {
					lp := 0
					if loopbackEnable {
						lp = -1
					}
					syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
					syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, lp)
				})
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

				sysconn, err := conn.SyscallConn()
				if err != nil {
					return err
				}
				sysconn.Control(func(fd uintptr) {
					syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 5)
					syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, -1)
				})
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
