package main

import (
	"os"

	"github.com/y3owk1n/mimi/cmd/mimi/cmd"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
