package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/openziti/zrok/v2/environment"
	"github.com/openziti/zrok/v2/environment/env_core"
	"github.com/openziti/zrok/v2/sdk/golang/sdk"
)

// startZrok reads ZROK2_ENABLE_TOKEN from the environment and, if set, starts
// serving the handler on a zrok public share. If the zrok environment is not
// yet enabled, it is enabled idempotently using the token (matching the
// behavior of zrok2-enable.bash). Returns a cleanup function that deletes the
// share and closes the listener. Returns (nil, nil) when the env var is unset.
func startZrok(handler http.Handler, tlsConfig *tls.Config) (func(), error) {
	token := os.Getenv("ZROK2_ENABLE_TOKEN")
	if token == "" {
		return nil, nil
	}

	apiEndpoint := os.Getenv("ZROK2_API_ENDPOINT")
	if apiEndpoint == "" {
		apiEndpoint = "https://api-v2.zrok.io"
	}

	root, err := environment.LoadRoot()
	if err != nil {
		return nil, fmt.Errorf("loading zrok environment: %w", err)
	}

	if !root.IsEnabled() {
		if err := root.SetConfig(&env_core.Config{
			ApiEndpoint: apiEndpoint,
		}); err != nil {
			return nil, fmt.Errorf("setting zrok config: %w", err)
		}

		// Temporarily set the environment with just the account token so the
		// SDK can authenticate the enable request.
		if err := root.SetEnvironment(&env_core.Environment{
			AccountToken: token,
			ApiEndpoint:  apiEndpoint,
		}); err != nil {
			return nil, fmt.Errorf("setting zrok environment: %w", err)
		}

		hostname, _ := os.Hostname()
		envName := os.Getenv("ZROK2_ENVIRONMENT_NAME")
		if envName == "" {
			envName = "sitegen on " + hostname
		}

		env, err := sdk.EnableEnvironment(root, &sdk.EnableRequest{
			Host:        hostname,
			Description: envName,
		})
		if err != nil {
			return nil, fmt.Errorf("enabling zrok environment: %w", err)
		}

		// Persist the full environment with the Ziti identity returned by the API.
		if err := root.SetEnvironment(&env_core.Environment{
			AccountToken: token,
			ZitiIdentity: env.ZitiIdentity,
			ApiEndpoint:  apiEndpoint,
		}); err != nil {
			return nil, fmt.Errorf("saving zrok environment: %w", err)
		}

		if err := root.SaveZitiIdentityNamed(env.ZitiIdentity, env.ZitiConfig); err != nil {
			return nil, fmt.Errorf("saving zrok identity: %w", err)
		}

		// Reload to pick up the persisted state.
		root, err = environment.LoadRoot()
		if err != nil {
			return nil, fmt.Errorf("reloading zrok environment: %w", err)
		}
	}

	shr, err := sdk.CreateShare(root, &sdk.ShareRequest{
		BackendMode:    sdk.ProxyBackendMode,
		ShareMode:      sdk.PublicShareMode,
		NameSelections: []sdk.NameSelection{{NamespaceToken: "public"}},
		Target:         "sitegen",
	})
	if err != nil {
		return nil, fmt.Errorf("creating zrok share: %w", err)
	}

	listener, err := sdk.NewListener(shr.Token, root)
	if err != nil {
		_ = sdk.DeleteShare(root, shr)
		return nil, fmt.Errorf("creating zrok listener: %w", err)
	}

	var ln net.Listener = listener
	if tlsConfig != nil {
		ln = tls.NewListener(listener, tlsConfig)
	}

	fmt.Printf("Serving on zrok: %s\n", strings.Join(shr.FrontendEndpoints, ", "))
	go func() {
		if err := http.Serve(ln, handler); err != nil && !isClosedErr(err) {
			fmt.Fprintf(os.Stderr, "zrok listener error: %v\n", err)
		}
	}()

	return func() {
		_ = listener.Close()
		_ = sdk.DeleteShare(root, shr)
	}, nil
}
