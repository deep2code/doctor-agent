// Command kbseed seeds the MariaDB knowledge database from the compressed
// gz/ archives. It exists so local development and CI can materialise the
// knowledge store without going through the full docker build:
//
//	MARIA_DB_PASSWORD=… go run ./cmd/kbseed [-dsn DSN] [-gz internal/knowledge/gz]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/doctor-agent/internal/knowledge"
)

func main() {
	dsn := flag.String("dsn", "", "MariaDB DSN (defaults to MARIA_DB_* env composition)")
	gzDir := flag.String("gz", "internal/knowledge/gz", "directory holding *.json.zst archives")
	flag.Parse()

	if *dsn == "" {
		*dsn = os.Getenv("KNOWLEDGE_DB_DSN")
	}
	if err := knowledge.Seed(*dsn, *gzDir); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}
