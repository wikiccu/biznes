package main

import (
	"fmt"
	"os"

	"github.com/wikiccu/biznes/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "biznes:", err)
		os.Exit(1)
	}

	fmt.Printf("biznes: configuration loaded (HTTP port %d)\n", cfg.HTTPPort)
}
