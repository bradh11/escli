// Command escli is a read-only Elasticsearch client for people and AI agents.
package main

import (
	"os"

	"github.com/bradh11/escli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
