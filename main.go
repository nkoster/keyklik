package main

import (
	"fmt"
	"log"
	"os"

	"keyklik/internal/app"
	"keyklik/internal/util"
)

func main() {
	if err := app.Run(os.Args, os.Stdout, os.Stderr); err != nil {
		log.Printf("error: %v", err)
		util.Ignore(fmt.Fprintln(os.Stderr, "Run with --help for usage."))
		os.Exit(1)
	}
}
