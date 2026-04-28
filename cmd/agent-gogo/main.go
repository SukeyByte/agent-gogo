package main

import (
	"fmt"

	"github.com/sukeke/agent-gogo/internal/app"
)

func main() {
	fmt.Printf("agent-gogo %s\n", app.Version)
	fmt.Println("runtime scaffold is ready")
}
