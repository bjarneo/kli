package ui

import (
	"strings"
	"testing"

	"github.com/bjarneo/kli/internal/k8s"
)

func TestKubectlTableCommandAllNamespaces(t *testing.T) {
	app := App{
		res: k8s.ResourceInfo{Resource: "pods", Namespaced: true},
	}

	got := app.kubectlGetTableCommand()
	want := "kubectl get pods --all-namespaces"
	if got != want {
		t.Fatalf("kubectlGetTableCommand() = %q; want %q", got, want)
	}
}

func TestKubectlObjectCommandUsesContextNamespaceAndGroup(t *testing.T) {
	app := App{
		client:    &k8s.Client{ContextName: "kind-kli-demo"},
		namespace: "default",
	}
	target := target{
		res:  k8s.ResourceInfo{Group: "apps", Resource: "deployments", Namespaced: true},
		name: "frontend",
	}

	got := app.kubectlGetObjectCommand(target)
	want := "kubectl --context kind-kli-demo get deployments.apps frontend -n default -o yaml"
	if got != want {
		t.Fatalf("kubectlGetObjectCommand() = %q; want %q", got, want)
	}
}

func TestKubectlLogsCommand(t *testing.T) {
	app := App{
		client: &k8s.Client{ContextName: "kind-kli-demo"},
		logs: logView{
			ns:   "kli-demo",
			pod:  "frontend-7d9",
			cont: "web",
		},
	}

	got := app.kubectlLogsCommand()
	want := "kubectl --context kind-kli-demo logs -n kli-demo frontend-7d9 -c web --tail 1000 -f"
	if got != want {
		t.Fatalf("kubectlLogsCommand() = %q; want %q", got, want)
	}
}

func TestShellJoinQuotesUnsafeArgs(t *testing.T) {
	got := shellJoin([]string{"kubectl", "--context", "team cluster", "get", "pods"})
	if !strings.Contains(got, "'team cluster'") {
		t.Fatalf("shellJoin() = %q; want quoted context", got)
	}
}
