// Package lambda adapts ding to AWS Lambda. The same binary serves the cobra
// CLI and two Lambda functions; which path runs is decided by SelectRoute based
// on the AWS_LAMBDA_RUNTIME_API environment variable and DING_LAMBDA_MODE.
package lambda

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	awslambda "github.com/aws/aws-lambda-go/lambda"
	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/discord"
	"github.com/jamesonstone/ding/internal/sendjob"
	"gorm.io/gorm"
)

// Route is the selected execution path for the binary.
type Route int

const (
	// RouteCLI runs the local cobra CLI (not in Lambda).
	RouteCLI Route = iota
	// RouteInteractions serves Discord interactions via API Gateway.
	RouteInteractions
	// RouteSend runs the monthly EventBridge send job.
	RouteSend
	// RouteUnknown is an in-Lambda invocation with an invalid DING_LAMBDA_MODE.
	RouteUnknown
)

// IsLambda reports whether the process is running inside the AWS Lambda runtime.
func IsLambda() bool { return os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" }

// SelectRoute decides which path to run. Outside Lambda it is always RouteCLI;
// inside Lambda it maps DING_LAMBDA_MODE to a handler.
func SelectRoute(isLambda bool, mode string) Route {
	if !isLambda {
		return RouteCLI
	}
	switch strings.TrimSpace(mode) {
	case "interactions":
		return RouteInteractions
	case "send":
		return RouteSend
	default:
		return RouteUnknown
	}
}

// Run wires dependencies and starts the appropriate Lambda handler. It is only
// called when IsLambda() is true.
func Run() error {
	env, err := config.LoadEnv()
	if err != nil {
		return err
	}
	gdb, err := db.Open(env.DBPath)
	if err != nil {
		return err
	}
	switch SelectRoute(true, env.LambdaMode) {
	case RouteInteractions:
		awslambda.Start(interactionHandler(gdb, env))
		return nil
	case RouteSend:
		awslambda.Start(sendHandler(gdb, env))
		return nil
	default:
		return fmt.Errorf("lambda: invalid DING_LAMBDA_MODE %q (want interactions|send)", env.LambdaMode)
	}
}

// interactionHandler verifies and routes Discord interactions from API Gateway.
func interactionHandler(gdb *gorm.DB, env config.Env) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	h := discord.Handler{Store: db.NewStore(gdb), Env: env}
	return func(_ context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		sig := headerLookup(req, "X-Signature-Ed25519")
		ts := headerLookup(req, "X-Signature-Timestamp")
		body, status := h.VerifyAndHandle(sig, ts, []byte(req.Body))
		return events.APIGatewayProxyResponse{
			StatusCode: status,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       string(body),
		}, nil
	}
}

// sendHandler runs the monthly summary for every customer. The EventBridge
// scheduled-event payload is ignored.
func sendHandler(gdb *gorm.DB, env config.Env) func(context.Context) error {
	return func(ctx context.Context) error {
		return sendjob.RunAll(ctx, sendjob.Deps{DB: gdb, Env: env})
	}
}

// headerLookup finds an HTTP header case-insensitively across the proxy
// request's single- and multi-value header maps.
func headerLookup(req events.APIGatewayProxyRequest, name string) string {
	lower := strings.ToLower(name)
	for k, v := range req.Headers {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	for k, vs := range req.MultiValueHeaders {
		if strings.ToLower(k) == lower && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}
