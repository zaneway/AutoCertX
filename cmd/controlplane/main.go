package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zaneway/AutoCertX/internal/app/controlplane/bootstrap"
)

func main() {
	if err := bootstrap.Run(context.Background(), bootstrap.Options{
		ServiceName:     "controlplane",
		EnvPrefix:       "CONTROLPLANE",
		DefaultHTTPAddr: ":8080",
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
