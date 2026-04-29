package app

import "testing"

func TestParseAgentInputTreatsAllWordsAsGoal(t *testing.T) {
	goal, opts, err := parseAgentInput([]string{"--config", "configs/test.yaml", "为苏柯宇写一个个人网页并完成部署"})
	if err != nil {
		t.Fatalf("parse input: %v", err)
	}
	if goal != "为苏柯宇写一个个人网页并完成部署" {
		t.Fatalf("unexpected goal %q", goal)
	}
	if opts.ConfigPath != "configs/test.yaml" {
		t.Fatalf("unexpected config path %q", opts.ConfigPath)
	}
}

func TestParseWebInput(t *testing.T) {
	opts, addr, err := parseWebInput([]string{"--config", "configs/test.yaml", "--addr", "127.0.0.1:9090"})
	if err != nil {
		t.Fatalf("parse web input: %v", err)
	}
	if opts.ConfigPath != "configs/test.yaml" {
		t.Fatalf("unexpected config path %q", opts.ConfigPath)
	}
	if addr != "127.0.0.1:9090" {
		t.Fatalf("unexpected addr %q", addr)
	}
	if !isWebCommand("console") {
		t.Fatal("expected console to be a web command")
	}
}

func TestExtractPersonalSiteName(t *testing.T) {
	if got := extractPersonalSiteName("为张三写一个个人网页并完成部署"); got != "张三" {
		t.Fatalf("expected 张三, got %q", got)
	}
}
