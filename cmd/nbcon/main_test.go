package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunWritesHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Launch services discovered from NetBox", "sync", "list"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help output %q does not contain %q", output.String(), want)
		}
	}
}
