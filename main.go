// Command kli is a terminal UI for Kubernetes: browse any resource, view and
// edit objects, follow logs, and switch namespaces and contexts, all from the
// keyboard. It uses your default kubeconfig.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/bjarneo/kli/internal/k8s"
	"github.com/bjarneo/kli/internal/ui"
	"github.com/bjarneo/kli/internal/upgrade"
)

// version is set at build time with -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "upgrade" {
		if err := upgrade.Run(version); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "config" {
		if err := runConfig(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	var (
		ctxFlag, nsFlag, resFlag, themeFlag, kubeconfigFlag string
		checkFlag, versionFlag                              bool
	)
	flag.StringVar(&ctxFlag, "context", "", "kubeconfig context to use (default: current-context)")
	flag.StringVar(&nsFlag, "namespace", "", "initial namespace (empty = all namespaces)")
	flag.StringVar(&nsFlag, "n", "", "initial namespace (shorthand)")
	flag.StringVar(&resFlag, "resource", "", "initial resource, e.g. pods, deploy, svc")
	flag.StringVar(&themeFlag, "theme", "", "color theme: ansi (default) or tokyonight")
	flag.StringVar(&kubeconfigFlag, "kubeconfig", "", "path to the kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	flag.BoolVar(&checkFlag, "check", false, "run a read-only connectivity check and exit")
	flag.BoolVar(&versionFlag, "version", false, "print version and exit")
	flag.Parse()

	switch {
	case versionFlag:
		fmt.Println("kli", version)
		return
	case checkFlag:
		if err := check(ctxFlag, kubeconfigFlag, nsFlag, resFlag); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if err := ui.Run(ui.Options{
		Context:    ctxFlag,
		Namespace:  nsFlag,
		Resource:   resFlag,
		Theme:      themeFlag,
		Kubeconfig: kubeconfigFlag,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// check performs a read-only listing to validate connectivity and discovery
// without starting the UI. Useful in non-interactive environments.
func check(ctxName, kubeconfig, ns, resQuery string) error {
	if resQuery == "" {
		resQuery = "pods"
	}
	cl, err := k8s.NewClient(ctxName, kubeconfig)
	if err != nil {
		return err
	}
	fmt.Printf("context:   %s\nhost:      %s\nnamespace: %q\nresources: %d discovered\n",
		cl.ContextName, cl.Host, cl.Namespace, len(cl.Registry().All()))

	ri, ok := cl.Registry().Resolve(resQuery)
	if !ok {
		return fmt.Errorf("unknown resource %q", resQuery)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tbl, err := cl.ListTable(ctx, ri, ns)
	if err != nil {
		return err
	}
	fmt.Printf("listed %s: %d columns, %d rows\n", ri.Key(), len(tbl.Columns), len(tbl.Rows))
	return nil
}

// runConfig handles the `kli config <subcommand>` family.
func runConfig(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "init":
		force := false
		for _, a := range args[1:] {
			if a == "--force" || a == "-f" {
				force = true
			}
		}
		path, err := ui.WriteDefaultConfig(force)
		if err != nil {
			return err
		}
		fmt.Println("wrote", path)
		return nil
	case "path":
		path, err := ui.ConfigPath()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	default:
		return fmt.Errorf("usage: kli config <init [--force] | path>")
	}
}
