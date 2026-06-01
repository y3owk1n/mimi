package main

import (
	"os"
	"runtime"

	"github.com/y3owk1n/mimi/cmd/mimi/cmd"
)

func main() {
	runtime.LockOSThread()

	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
