package pkg

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/urfave/cli/v2"
)

// ClientOpts holds the options the "client" command supports
type ClientOpts struct {
	RemoteAddr         string
	LocalIP            string
	GoRoutines         int
	DialIntervalUs     int
	TimeoutMs          int
	SummaryIntervalSec int
}

func NewClientCommand() *cli.Command {
	var opts ClientOpts

	return &cli.Command{
		Name:        "client",
		Aliases:     []string{"c"},
		Description: "Starts multiple Go routines that continuously connect to an endpoints and periodically print statistics",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "remote-addr",
				Usage:       "Remote address (<host>:<port>) to connect to",
				Aliases:     []string{"r"},
				EnvVars:     []string{"REMOTE_ADDR"},
				Destination: &opts.RemoteAddr,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "local-ip",
				Usage:       "Local IP  to connect from",
				Aliases:     []string{"l"},
				EnvVars:     []string{"LOCAL_IP"},
				Destination: &opts.LocalIP,
			},
			&cli.IntFlag{
				Name:        "go-routines",
				Usage:       "Number of Go routines to starts, which by default is equal to the number of CPU cores",
				Aliases:     []string{"c"},
				EnvVars:     []string{"GO_ROUTINES"},
				Value:       runtime.NumCPU(),
				Destination: &opts.GoRoutines,
			},
			&cli.IntFlag{
				Name:        "timeout-ms",
				Usage:       "Connection timeout in millisecond. Should be less than a second.",
				Aliases:     []string{"t"},
				Value:       500,
				EnvVars:     []string{"TIMEOUT_MS"},
				Destination: &opts.TimeoutMs,
			},
			&cli.IntFlag{
				Name:        "dial-interval-us",
				Aliases:     []string{"d"},
				EnvVars:     []string{"DIAL_INTERVAL_US"},
				Value:       100000,
				Destination: &opts.DialIntervalUs,
			},
			&cli.IntFlag{
				Name:        "summary-interval-sec",
				Aliases:     []string{"s"},
				EnvVars:     []string{"SUMMARY_INTERVAL_S"},
				Value:       5,
				Destination: &opts.SummaryIntervalSec,
			},
		},
		Action: func(c *cli.Context) error {
			return runClient(opts)
		},
	}
}

func runClient(opts ClientOpts) error {
	localAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:", opts.LocalIP))
	if err != nil {
		return fmt.Errorf("unable to parse local IP: %w", err)
	}

	dialer := &net.Dialer{
		LocalAddr: localAddr,
		Timeout:   time.Duration(opts.TimeoutMs) * time.Millisecond,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Concurrent dialer
	log.Println("Starting Go routines!")
	requestDurationCh := make(chan float64, 1024)
	var reqTotal int64
	var errTotal int64
	var wg sync.WaitGroup
	for i := 0; i < opts.GoRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(time.Duration(opts.DialIntervalUs) * time.Microsecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					start := time.Now()
					conn, err := dialer.DialContext(ctx, "tcp", opts.RemoteAddr)

					elapsed := time.Since(start)
					requestDurationCh <- elapsed.Seconds()

					atomic.AddInt64(&reqTotal, 1)
					if err != nil {
						log.Printf("error after %dms, err: \"%s\"", elapsed.Nanoseconds()/1000000, err)
						atomic.AddInt64(&errTotal, 1)
						continue
					}
					conn.Close()
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Statistics computation and display
	requestDurations := []float64{}
	ticker := time.NewTicker(time.Second * time.Duration(opts.SummaryIntervalSec))
	defer func() {
		ticker.Stop()
		log.Println("Stopping...")
		wg.Wait()
	}()

	for {
		select {
		case requestDuration := <-requestDurationCh:
			requestDurations = append(requestDurations, requestDuration)
		case <-ticker.C:
			printPeriodStatistics(requestDurations, errTotal, reqTotal, opts.SummaryIntervalSec)
			requestDurations = requestDurations[:0]
		case <-ctx.Done():
			return nil
		}
	}
}

func printPeriodStatistics(reqDurations []float64, errTotal int64, reqTotal int64, periodDurationSec int) {
	if len(reqDurations) == 0 {
		log.Printf("No request completed in the current time frame")
		return
	}
	sort.Float64s(reqDurations)

	var b bytes.Buffer
	fmt.Fprintf(&b, "Summary of the last %d seconds:\n", periodDurationSec)
	fmt.Fprintf(&b, "             Max response time: %5.1fms\n", reqDurations[len(reqDurations)-1]*1000)
	fmt.Fprintf(&b, "                 99 percentile: %5.1fms\n", reqDurations[(len(reqDurations)-1)*99/100]*1000)
	fmt.Fprintf(&b, "                 95 percentile: %5.1fms\n", reqDurations[(len(reqDurations)-1)*95/100]*1000)
	fmt.Fprintf(&b, "                        median: %5.1fms\n", reqDurations[(len(reqDurations)-1)*50/100]*1000)
	fmt.Fprintf(&b, "                  Request Rate: %5dreq/s\n\n", len(reqDurations)/periodDurationSec)
	fmt.Fprintf(&b, "Requests since start (error/total): %5d/%d\n", errTotal, reqTotal)
	log.Println(b.String())
}
