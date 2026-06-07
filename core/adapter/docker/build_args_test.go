// core/adapter/docker/build_args_test.go
package docker

import (
	"strings"
	"testing"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/usecase/ports"
)

func indexOfArg(args []string, val string) int {
	for i, a := range args {
		if a == val {
			return i
		}
	}
	return -1
}

func hasFlagValue(args []string, flag, val string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}

func testSpec() ports.RunnerStartSpec {
	return ports.RunnerStartSpec{
		JobID:      domain.JobID("j1"),
		Repo:       "owner/repo",
		BaseBranch: "main",
		WorkBranch: "agentic/x",
		Spec:       domain.SpecSource{Type: domain.SourceTypeInline, Value: "noop"},
		GitCreds:   domain.GitCreds{Token: "fake-token"},
		Anthropic:  domain.AnthropicCreds{APIKey: "sk-fake"},
	}
}

func TestBuildRunArgs_noNetwork_omitsNetworkAndDNS(t *testing.T) {
	cfg := Config{Image: "runner:dev", DNS: []string{"1.1.1.1", "1.0.0.1"}} // Network == ""
	args := buildRunArgs(cfg, testSpec(), "/work/j1", "/work/j1.secrets")

	if indexOfArg(args, "--network") != -1 {
		t.Fatalf("--network must be absent when Network==\"\": %v", args)
	}
	if indexOfArg(args, "--dns") != -1 {
		t.Fatalf("--dns must be absent when Network==\"\": %v", args)
	}
	if args[len(args)-1] != "runner:dev" {
		t.Fatalf("Image must be the last arg (no EntryOverride): %v", args)
	}
}

func TestBuildRunArgs_withNetwork_addsFlagsBeforeImage(t *testing.T) {
	cfg := Config{Image: "runner:dev", Network: "runner-net", DNS: []string{"1.1.1.1", "1.0.0.1"}}
	args := buildRunArgs(cfg, testSpec(), "/work/j1", "/work/j1.secrets")

	if !hasFlagValue(args, "--network", "runner-net") {
		t.Fatalf("missing --network runner-net: %v", args)
	}
	if !hasFlagValue(args, "--dns", "1.1.1.1") || !hasFlagValue(args, "--dns", "1.0.0.1") {
		t.Fatalf("missing --dns entries: %v", args)
	}
	img := indexOfArg(args, "runner:dev")
	if net := indexOfArg(args, "--network"); net == -1 || net >= img {
		t.Fatalf("--network must precede Image: net=%d img=%d (%v)", net, img, args)
	}
	if dns := indexOfArg(args, "--dns"); dns == -1 || dns >= img {
		t.Fatalf("--dns must precede Image: dns=%d img=%d (%v)", dns, img, args)
	}
	if args[len(args)-1] != "runner:dev" {
		t.Fatalf("Image must be the last arg (no EntryOverride): %v", args)
	}
}

func TestBuildRunArgs_imageImmediatelyBeforeEntryOverride(t *testing.T) {
	cfg := Config{Image: "runner:dev", Network: "runner-net", DNS: []string{"1.1.1.1"}, EntryOverride: []string{"sh", "-c", "echo hi"}}
	args := buildRunArgs(cfg, testSpec(), "/work/j1", "/work/j1.secrets")

	img := indexOfArg(args, "runner:dev")
	if img == -1 || img != len(args)-1-len(cfg.EntryOverride) {
		t.Fatalf("Image must be immediately before EntryOverride: img=%d len=%d (%v)", img, len(args), args)
	}
	if args[img+1] != "sh" || args[len(args)-1] != "echo hi" {
		t.Fatalf("EntryOverride must follow Image in order: %v", args)
	}
}

func TestBuildRunArgs_secretsMountAndNoSecretEnv(t *testing.T) {
	cfg := Config{Image: "runner:dev"}
	args := buildRunArgs(cfg, testSpec(), "/work/j1", "/work/j1.secrets")

	if !hasFlagValue(args, "-v", "/work/j1.secrets:/run/delegator-secrets:ro") {
		t.Fatalf("read-only secrets mount missing: %v", args)
	}
	for _, a := range args {
		if strings.HasPrefix(a, "GH_TOKEN=") || strings.HasPrefix(a, "ANTHROPIC_API_KEY=") {
			t.Fatalf("secrets must NOT be passed as -e env (visible to docker inspect): found %q", a)
		}
	}
	// Non-secret env must remain.
	if !hasFlagValue(args, "-e", "JOB_ID=j1") {
		t.Fatalf("JOB_ID env should remain: %v", args)
	}
	if !hasFlagValue(args, "-e", "REPO=owner/repo") {
		t.Fatalf("REPO env should remain: %v", args)
	}
}

func TestBuildRunArgs_alwaysHasHardeningAndWorkspaceMount(t *testing.T) {
	cfg := Config{Image: "runner:dev"}
	args := buildRunArgs(cfg, testSpec(), "/work/j1", "/work/j1.secrets")

	if indexOfArg(args, "--cap-drop=ALL") == -1 {
		t.Fatalf("--cap-drop=ALL must always be present: %v", args)
	}
	if !hasFlagValue(args, "-v", "/work/j1:/workspace") {
		t.Fatalf("workspace bind mount missing: %v", args)
	}
}
