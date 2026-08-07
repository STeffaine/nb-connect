package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/steffaine/nb-connect/internal/app"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	return app.Run(ctx, args, output)
}
