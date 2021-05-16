package pkg

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/urfave/cli/v2"
)

// ServerOpts holds the options the "server" command supports
type ServerOpts struct {
	LocalAddr  string
	GoRoutines int
}

func NewServerCommand() *cli.Command {
	var opts ServerOpts

	return &cli.Command{
		Name:        "server",
		Aliases:     []string{"s"},
		Description: "Simple TCP server to connect to",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "local-addr",
				Usage:       "Local address (<host>:<port>) to listen at",
				Aliases:     []string{"r"},
				EnvVars:     []string{"LOCAL_ADDR"},
				Destination: &opts.LocalAddr,
				Value:       "0.0.0.0:8080",
			},
			&cli.IntFlag{
				Name:        "go-routines",
				Usage:       "Number of Go routines to starts, which by default is equal to the number of CPU cores",
				Aliases:     []string{"c"},
				EnvVars:     []string{"GO_ROUTINES"},
				Value:       runtime.NumCPU(),
				Destination: &opts.GoRoutines,
			},
		},
		Action: func(c *cli.Context) error {
			return runServer(opts)
		},
	}
}

func runServer(opts ServerOpts) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	l, err := net.Listen("tcp4", opts.LocalAddr)
	if err != nil {
		return fmt.Errorf("error while listening on '%s': %w", opts.LocalAddr, err)
	}
	defer l.Close()

	// Can't remember why this was needed
	rand.Seed(time.Now().Unix())

	log.Println("Ready to accept connections!")
	var wg sync.WaitGroup
	for i := 0; i < opts.GoRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				c, err := l.Accept()
				if err != nil {
					log.Printf("Error while accepting new connection: %v", err)
					return
				}
				c.Close()
			}
		}()
	}

	<-ctx.Done()
	log.Println("Stopping...")
	l.Close()
	wg.Wait()

	return nil
}
