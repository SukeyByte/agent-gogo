package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const Version = "0.1.0-dev"

type Options struct {
	ConfigPath string
}

func Main(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	ctx := context.Background()
	if isWebCommand(args[0]) {
		opts, addr, err := parseWebInput(args[1:])
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			printUsage(stderr)
			return 2
		}
		if err := RunWebConsole(ctx, opts, addr, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	goal, opts, err := parseAgentInput(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return 2
	}
	if err := RunAgent(ctx, goal, opts, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func parseAgentInput(args []string) (string, Options, error) {
	flags := flag.NewFlagSet("agent-gogo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "config path")
	if err := flags.Parse(args); err != nil {
		return "", Options{}, err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return "", Options{}, errors.New("missing agent goal")
	}
	return strings.TrimSpace(strings.Join(rest, " ")), Options{ConfigPath: *configPath}, nil
}

func parseWebInput(args []string) (Options, string, error) {
	flags := flag.NewFlagSet("agent-gogo web", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "config path")
	addr := flags.String("addr", "", "listen address")
	if err := flags.Parse(args); err != nil {
		return Options{}, "", err
	}
	return Options{ConfigPath: *configPath}, strings.TrimSpace(*addr), nil
}

func isWebCommand(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "web", "console", "serve":
		return true
	default:
		return false
	}
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintf(writer, `agent-gogo %s

Usage:
  agent-gogo web --addr 127.0.0.1:8080
  agent-gogo "实现一个可测试的 Web Console Dashboard"
  agent-gogo "打开 https://example.com，提取页面信息并总结"
  agent-gogo "为当前仓库规划并执行下一步 runtime 改造"

Config:
  DEEPSEEK_API_KEY is required for the default DeepSeek provider.
  AGENT_GOGO_BROWSER_MCP_URL overrides the Chrome MCP HTTP bridge URL.
  AGENT_GOGO_ALLOW_SHELL=true enables allowlisted engineering commands.
`, Version)
}
