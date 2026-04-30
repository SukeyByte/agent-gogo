package main

import (
	"os"

	"github.com/SukeyByte/agent-gogo/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Stdout, os.Stderr))
}
