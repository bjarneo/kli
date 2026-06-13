package ui

import (
	"strings"
	"testing"

	"github.com/bjarneo/kli/internal/k8s"
)

func TestRenderConfigDecodesSecretData(t *testing.T) {
	th := PickTheme("ansi")
	res := k8s.ResourceInfo{Resource: "secrets", Kind: "Secret"}
	obj := map[string]interface{}{
		"type": "Opaque",
		"data": map[string]interface{}{
			"password": "aHVudGVyMg==",
		},
	}

	out := renderConfig(th, res, obj, 80)
	if !strings.Contains(out, "Decoded Data") {
		t.Fatalf("decoded data section missing from config view:\n%s", out)
	}
	if !strings.Contains(out, "hunter2") {
		t.Fatalf("decoded secret value missing from config view:\n%s", out)
	}
	if !strings.Contains(out, "7B decoded") {
		t.Fatalf("decoded secret size metadata missing from config view:\n%s", out)
	}
	if strings.Contains(out, "aHVudGVyMg==") {
		t.Fatalf("encoded secret value leaked into config view:\n%s", out)
	}
}
