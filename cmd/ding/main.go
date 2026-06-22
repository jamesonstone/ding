// Command ding is the single binary for the ding invoice service. When running
// inside AWS Lambda it serves the configured handler; otherwise it runs the
// local CLI. See internal/lambda.SelectRoute for the routing rule.
package main

import (
	"log"

	"github.com/jamesonstone/ding/internal/cli"
	"github.com/jamesonstone/ding/internal/lambda"
)

func main() {
	if lambda.IsLambda() {
		if err := lambda.Run(); err != nil {
			log.Fatalf("ding: %v", err)
		}
		return
	}
	cli.Execute()
}
