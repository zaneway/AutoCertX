package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zaneway/AutoCertX/internal/platform/runtime"
)

func main() {
	if err := runtime.Run(context.Background(), runtime.Options{
		ServiceName:     "agent",
		EnvPrefix:       "AGENT",
		DefaultHTTPAddr: ":8081",
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
