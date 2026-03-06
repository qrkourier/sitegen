package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/openziti/zrok/v2/environment"
	"github.com/openziti/zrok/v2/environment/env_core"
	restShare "github.com/openziti/zrok/v2/rest_client_zrok/share"
	"github.com/openziti/zrok/v2/sdk/golang/sdk"
)

// startZrok reads ZROK2_ENABLE_TOKEN from the environment and, if set, starts
// serving the handler on a zrok public share. If the zrok environment is not
// yet enabled, it is enabled idempotently using the token (matching the
// behavior of zrok2-enable.bash). When ZROK2_UNIQUE_NAME is set the name is
// created idempotently in the default namespace before sharing.
// Returns a cleanup function that deletes the share and closes the listener.
// Returns (nil, nil) when the env var is unset.
func startZrok(handler http.Handler) (func(), error) {
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
		if err := enableZrokEnvironment(root, token, apiEndpoint); err != nil {
			return nil, err
		}
		// Reload to pick up the persisted state.
		root, err = environment.LoadRoot()
		if err != nil {
			return nil, fmt.Errorf("reloading zrok environment: %w", err)
		}
	}

	uniqueName := os.Getenv("ZROK2_UNIQUE_NAME")
	if uniqueName != "" {
		if err := ensureZrokName(root, uniqueName); err != nil {
			return nil, err
		}
	}

	shareReq := &sdk.ShareRequest{
		BackendMode: sdk.ProxyBackendMode,
		ShareMode:   sdk.PublicShareMode,
		Target:      "sitegen",
	}
	if uniqueName != "" {
		ns, _ := root.DefaultNamespace()
		shareReq.NameSelections = []sdk.NameSelection{{NamespaceToken: ns, Name: uniqueName}}
	}

	shr, err := sdk.CreateShare(root, shareReq)
	if err != nil {
		return nil, fmt.Errorf("creating zrok share: %w", err)
	}

	listener, err := sdk.NewListener(shr.Token, root)
	if err != nil {
		_ = sdk.DeleteShare(root, shr)
		return nil, fmt.Errorf("creating zrok listener: %w", err)
	}

	// zrok terminates TLS at the frontend — never wrap the backend listener.
	fmt.Printf("Serving on zrok: %s\n", strings.Join(shr.FrontendEndpoints, ", "))
	go func() {
		if err := http.Serve(listener, handler); err != nil && !isClosedErr(err) {
			fmt.Fprintf(os.Stderr, "zrok listener error: %v\n", err)
		}
	}()

	return func() {
		_ = listener.Close()
		_ = sdk.DeleteShare(root, shr)
	}, nil
}

// enableZrokEnvironment idempotently enables a zrok environment using the
// given account token (equivalent to zrok2-enable.bash).
func enableZrokEnvironment(root env_core.Root, token, apiEndpoint string) error {
	if err := root.SetConfig(&env_core.Config{
		ApiEndpoint: apiEndpoint,
	}); err != nil {
		return fmt.Errorf("setting zrok config: %w", err)
	}

	// Set the environment with the account token so the SDK can authenticate.
	if err := root.SetEnvironment(&env_core.Environment{
		AccountToken: token,
		ApiEndpoint:  apiEndpoint,
	}); err != nil {
		return fmt.Errorf("setting zrok environment: %w", err)
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
		return fmt.Errorf("enabling zrok environment: %w", err)
	}

	// Persist the full environment with the Ziti identity returned by the API.
	if err := root.SetEnvironment(&env_core.Environment{
		AccountToken: token,
		ZitiIdentity: env.ZitiIdentity,
		ApiEndpoint:  apiEndpoint,
	}); err != nil {
		return fmt.Errorf("saving zrok environment: %w", err)
	}

	if err := root.SaveZitiIdentityNamed(root.EnvironmentIdentityName(), env.ZitiConfig); err != nil {
		return fmt.Errorf("saving zrok identity: %w", err)
	}

	return nil
}

// ensureZrokName idempotently creates a custom name in the default namespace.
// A 409 Conflict (name already exists) is treated as success.
func ensureZrokName(root env_core.Root, name string) error {
	ns, _ := root.DefaultNamespace()

	zrok, err := root.Client()
	if err != nil {
		return fmt.Errorf("getting zrok client: %w", err)
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", root.Environment().AccountToken)

	req := restShare.NewCreateShareNameParams()
	req.Body = restShare.CreateShareNameBody{
		NamespaceToken: ns,
		Name:           name,
	}

	_, err = zrok.Share.CreateShareName(req, auth)
	if err != nil {
		// 409 means the name already exists — that's fine for idempotency.
		if _, ok := err.(*restShare.CreateShareNameConflict); ok {
			fmt.Printf("zrok name %q already exists in namespace %q\n", name, ns)
			return nil
		}
		return fmt.Errorf("creating zrok name %q in namespace %q: %w", name, ns, err)
	}

	fmt.Printf("zrok name %q created in namespace %q\n", name, ns)
	return nil
}
