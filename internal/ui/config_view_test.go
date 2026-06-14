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

	out := renderConfig(th, res, obj, 80, nil)
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
	if strings.Contains(out, "YAML") {
		t.Fatalf("config overview should not include raw yaml:\n%s", out)
	}
}

func TestRenderConfigShowsPodUsageAndIssues(t *testing.T) {
	th := PickTheme("ansi")
	res := k8s.ResourceInfo{Resource: "pods", Kind: "Pod"}
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name": "api",
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
						"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
					},
				},
			},
		},
		"status": map[string]interface{}{
			"phase": "Running",
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "False", "reason": "ContainersNotReady"},
			},
			"containerStatuses": []interface{}{
				map[string]interface{}{
					"name":         "api",
					"restartCount": 3,
					"state": map[string]interface{}{
						"waiting": map[string]interface{}{"reason": "CrashLoopBackOff"},
					},
				},
			},
		},
	}
	usage := &k8s.PodUsage{CPUUsedMilli: 25, MemUsedBytes: 64 * 1024 * 1024}

	out := renderConfig(th, res, obj, 80, usage)
	for _, want := range []string{"Usage", "25m live", "64Mi live", "cpu 100m", "mem 128Mi", "Health", "CrashLoopBackOff", "restarts", "3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("config view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "YAML") {
		t.Fatalf("config overview should not include raw yaml:\n%s", out)
	}
}

func TestRenderConfigPutsStatusBeforeOverview(t *testing.T) {
	th := PickTheme("ansi")
	tests := []struct {
		name string
		res  k8s.ResourceInfo
		obj  map[string]interface{}
		want []string
	}{
		{
			name: "deployment",
			res:  k8s.ResourceInfo{Resource: "deployments", Kind: "Deployment"},
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Available", "status": "False", "reason": "MinimumReplicasUnavailable"},
					},
				},
			},
			want: []string{"Available=False"},
		},
		{
			name: "job",
			res:  k8s.ResourceInfo{Resource: "jobs", Kind: "Job"},
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Failed", "status": "True", "reason": "BackoffLimitExceeded"},
					},
				},
			},
			want: []string{"BackoffLimitExceeded"},
		},
		{
			name: "cronjob",
			res:  k8s.ResourceInfo{Resource: "cronjobs", Kind: "CronJob"},
			obj: map[string]interface{}{
				"status": map[string]interface{}{"lastScheduleTime": "2026-06-14T10:00:00Z"},
			},
			want: []string{"last run"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderConfig(th, tt.res, tt.obj, 80, nil)
			statusAt := strings.Index(out, "Status")
			overviewAt := strings.Index(out, "Overview")
			if statusAt < 0 || overviewAt < 0 || statusAt > overviewAt {
				t.Fatalf("status section should appear before overview:\n%s", out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("config view missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestRenderConfigSeparatesLongSecretKeys(t *testing.T) {
	th := PickTheme("ansi")
	res := k8s.ResourceInfo{Resource: "secrets", Kind: "Secret"}
	obj := map[string]interface{}{
		"type": "Opaque",
		"data": map[string]interface{}{
			"POSTGRES_REPLICATION_PASSWORD": "cmVwbGljYXRvcnBhc3M=",
		},
	}

	out := renderConfig(th, res, obj, 72, nil)
	if strings.Contains(out, "POSTGRES_REPLICATION_PASSWORDreplicatorpass") {
		t.Fatalf("long secret key was not separated from value:\n%s", out)
	}
	if !strings.Contains(out, "  replicatorpass") {
		t.Fatalf("decoded value missing expected separation:\n%s", out)
	}
}
