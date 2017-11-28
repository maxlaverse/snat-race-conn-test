package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	flags "github.com/jessevdk/go-flags"
	"github.com/maxlaverse/snat-race-conn-test/lib"
)

var opts struct {
	URL           string `env:"URL" required:"true" short:"u" long:"url" description:"URL to connect to"`
	Concurrency   int    `env:"CONCURRENCY" short:"c" long:"concurrency" description:"Number of parallel requests" default:"25"`
	Interval      int    `env:"INTERVAL" short:"i" long:"interval" description:"Interval between two requests, in us" default:"100000"`
	Timeout       int    `env:"TIMEOUT" short:"t" long:"timeout" description:"Timeout for requests, in ms" default:"500"`
	PrintInterval int    `env:"PRINTINTERVAL" short:"p" long:"print-interval" description:"Interval between two stats prints, in seconds" default:"30"`
}

func main() {
	// Parse command line arguments
	_, err := flags.ParseArgs(&opts, os.Args)
	if err != nil {
		if err.(*flags.Error).Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}

	// Install signal handler
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)

	// Create a channel to get back measures
	measureCh := make(chan int64, 1024)

	log.Printf("Preparing %d requesters with a %d us interval on %s", opts.Concurrency, opts.Interval, opts.URL)
	requesters := []*lib.Requester{}
	for i := 0; i < opts.Concurrency; i++ {
		requesters = append(requesters, lib.NewRequester(opts.Interval, opts.Timeout, opts.URL, measureCh))
	}

	log.Println("Starting requesters")
	for _, requester := range requesters {
		go requester.Run()
	}

	// Measuring
	log.Println("Recording")
	measures := lib.Measure{}
	ticket := time.NewTicker(time.Second * time.Duration(opts.PrintInterval))
Loop:
	for {
		select {
		case measure := <-measureCh:
			measures = append(measures, measure)
		case <-ticket.C:
			if len(measures) == 0 {
				fmt.Printf("\nNo result:\n")
				continue
			}

			max, p99, p95, avg := measures.Stats()
			fmt.Printf("\nStatistics for the last period:\n")
			fmt.Printf("%10s: %5dms\n", "Max", max/1000000.0)
			fmt.Printf("%10s: %5dms\n", "99pctile", p99/1000000.0)
			fmt.Printf("%10s: %5dms\n", "95pctile", p95/1000000.0)
			fmt.Printf("%10s: %5dms\n", "Average", avg/1000000.0)
			fmt.Printf("%10s: %5dreq/s\n", "Rate", len(measures)/opts.PrintInterval)

			measures = lib.Measure{}
		case <-stopSignal:
			break Loop
		}
	}
	ticket.Stop()
	log.Println("Stopping requesters")
	for _, requester := range requesters {
		requester.Stop()
	}
	close(measureCh)
}
