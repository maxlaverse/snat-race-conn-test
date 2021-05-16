package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/maxlaverse/snat-race-conn-test/pkg"
	"github.com/urfave/cli/v2"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(new(logWriter))

	app := &cli.App{
		Name:  "snat-race-conn-test",
		Usage: "Test program to reproduce a race condition on SNAT'ed connections",
		Commands: []*cli.Command{
			pkg.NewServerCommand(),
			pkg.NewClientCommand(),
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type logWriter struct {
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(time.Now().Local().Format("2006-01-02T15:04:05.999 ") + string(bytes))
}
