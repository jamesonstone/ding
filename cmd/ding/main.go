// Command ding is the single binary for the ding invoice service. With
// DING_HTTP_LISTEN set it runs as a persistent HTTP server (EC2 / container);
// when running inside AWS Lambda it serves the configured handler; otherwise it
// runs the local CLI. See internal/lambda.SelectRoute and internal/httpserver
// for the routing rules.
package main

import (
	"log"
	"os"

	"github.com/jamesonstone/ding/internal/cli"
	"github.com/jamesonstone/ding/internal/httpserver"
	"github.com/jamesonstone/ding/internal/lambda"
)

func main() {
	// prioritize http server mode (ec2/container)
	if httpAddr := os.Getenv("DING_HTTP_LISTEN"); httpAddr != "" {
		if err := httpserver.Start(httpAddr); err != nil {
			log.Fatalf("ding: %v", err)
		}
		return
	}
	// then check for lambda mode
	if lambda.IsLambda() {
		if err := lambda.Run(); err != nil {
			log.Fatalf("ding: %v", err)
		}
		return
	}
	// fall back to local cli
	cli.Execute()
}
