// main
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
)

func main() {
	netInterfaceName := ""
	netInterface := (*net.Interface)(nil)
	address := ""
	ttl := 0
	loopbackEnable := false

	createMulticast := func() (*Multicast, error) {
		m, err := NewMulticast(address, netInterface)
		if err != nil {
			return nil, err
		}

		m.SetTTL(ttl)
		m.SetLoopback(loopbackEnable)
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

				hashId := []byte(nil)
				if c.Args().Present() {
					hashId, err = hex.DecodeString(c.Args().First())
					if err != nil {
						return err
					}
					if len(hashId) != hashSize {
						return errors.New(fmt.Sprintf("id must be %d characters", hashSize*2))
					}
				}

				cl := NewClient(m, hashId)
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
					return errors.New("Require arguments to specify which files to serve")
				}

				// directory name ending with ":::subdir" means to add recursively into subdir (or root).
				// directory name ending with "::subdir" means to add non-recursively into subdir (or root).
				// file name ending with "::alias" means to rename file.
				//
				// for directories:
				// "../asdf" -> "/*"
				// "../asdf::asdf" -> "/asdf/*"
				// "../asdf:::asdf" -> "/asdf/**" (recursively)
				// "/abs/path" => "/*"
				// "/abs/path::" => "/*"
				// "/abs/path:::" => "/**" (recursively)
				//
				// for files:
				// "hjkl" -> "/hjkl"
				// "hjkl::" -> "/hjkl"
				// "hjkl::asdf" -> "/asdf"

				files := make([]TarballFile, 0, len(args))
				for _, a := range args {
					localPath := a
					subdir := ""
					isRecursive := false

					// let "a::b" specify path 'a' with subdir 'b':
					// e.g. "../hello::hello"
					sep := strings.LastIndex(a, ":::")
					if sep > 0 {
						isRecursive = true
						localPath = a[:sep]
						subdir = a[sep+3:]
					} else {
						sep = strings.LastIndex(a, "::")
						if sep > 0 {
							localPath = a[:sep]
							subdir = a[sep+2:]
						}
					}

					localPath = filepath.Clean(localPath)

					stat, err := os.Stat(localPath)
					if err != nil {
						fmt.Printf("%s\n", err)
						// Skip file due to error:
						continue
					}

					if stat.IsDir() {
						// Walk directory tree:
						filepath.Walk(localPath, func(fullPath string, info os.FileInfo, err error) error {
							// Skip starting directory entry:
							if fullPath == localPath {
								return nil
							}

							// Allow/prevent recursion accordingly:
							if info.IsDir() {
								if !isRecursive {
									return filepath.SkipDir
								}
								return nil
							}

							// Translate to relative path with '/'s:
							relPath := filepath.ToSlash(fullPath[len(localPath)+1:])

							// Prepend subdir:
							tarPath := relPath
							if subdir != "" {
								tarPath = subdir + "/" + tarPath
							}

							// Add file to virtual tarball list:
							files = append(files, TarballFile{
								Path:      tarPath,
								LocalPath: fullPath,
								Size:      info.Size(),
								Mode:      info.Mode(),
							})
							return nil
						})
					} else {
						tarPath := localPath
						if subdir != "" {
							// Rename file:
							tarPath = subdir
						}

						// Add file to virtual tarball list:
						files = append(files, TarballFile{
							Path:      tarPath,
							LocalPath: localPath,
							Size:      stat.Size(),
							Mode:      stat.Mode(),
						})
					}
				}
				if len(files) == 0 {
					return errors.New("no files to serve")
				}

				// Treat collection of files as virtual tarball for reading:
				tb, err := NewVirtualTarballReader(files)
				if err != nil {
					return err
				}
				defer tb.Close()

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
