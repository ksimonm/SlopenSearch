// Command slopensearch serves the OpenSearch REST API (see internal/httpapi).
// Configuration comes from the environment: HOST, PORT, SLOP_DATA_DIR,
// SLOP_IN_MEMORY.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"slopensearch/internal/httpapi"
)

func main() {
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "9200"
	}
	addr := net.JoinHostPort(host, port)
	server := httpapi.NewServer()
	if err := server.OpenPersistence(persistenceDir()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	fmt.Println("slopensearch (go) listening on " + addr + server.PersistenceDescription())
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := srv.Serve(httpapi.NewListener(ln)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// persistenceDir returns the data directory from the environment:
// SLOP_DATA_DIR (default "data", relative to the working directory), or ""
// when SLOP_IN_MEMORY is true.
func persistenceDir() string {
	if v := strings.TrimSpace(os.Getenv("SLOP_IN_MEMORY")); v != "" {
		if on, err := strconv.ParseBool(v); err == nil && on {
			return ""
		}
	}
	if d := os.Getenv("SLOP_DATA_DIR"); d != "" {
		return d
	}
	return "data"
}
