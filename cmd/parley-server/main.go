// Command parley-server is the Server Host: one static binary serving the
// vanilla web client and the Session core over the v0.0.2 protocol.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/host"
	"github.com/wray-lee/parley/internal/protocol"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheck())
	}
	cfg := host.LoadConfig(os.Getenv)
	server, err := host.New(cfg, parley.Assets, host.Options{
		SegmentLog: func(record protocol.SegmentLog) {
			line, err := json.Marshal(record)
			if err != nil {
				return
			}
			fmt.Println(string(line))
		},
		Warn: func(line string) { fmt.Fprintln(os.Stderr, line) },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := server.Listen(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Parley listening on http://localhost:%d\n", server.Port())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	_ = server.Close()
}

// healthcheck probes this container's /healthz for the distroless image, which
// has no shell or Bun for a compose healthcheck command.
func healthcheck() int {
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", host.LoadConfig(os.Getenv).Port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthz:", response.Status)
		return 1
	}
	return 0
}
