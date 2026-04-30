package tools

import (
	"context"
	"errors"
	"strings"
)

func NewBuiltinRuntime(store Store, root string) *Runtime {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	runtime := NewRuntime(store)
	runtime.Register(Spec{
		Name:        "code.search",
		Description: "Search local source files for matching text.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := searchCode(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{
			Success:     true,
			Output:      output,
			EvidenceRef: "file://code.search",
		}, nil
	})
	runtime.Register(Spec{
		Name:        "code.index",
		Description: "Build a lightweight repository map and symbol index.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := runtime.indexCode(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "code://index"}, nil
	})
	runtime.Register(Spec{
		Name:        "code.symbols",
		Description: "Search the lightweight symbol index.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := runtime.codeSymbols(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "code://symbols"}, nil
	})
	runtime.Register(Spec{
		Name:        "file.read",
		Description: "Read a workspace file by relative path. Use this instead of shell.run for file contents.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := readFile(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["path"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "file.write",
		Description: "Write a workspace file.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := writeWorkspaceFile(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		runtime.invalidateCodeIndex(root)
		ref, _ := output["path"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "file.patch",
		Description: "Apply a small string replacement patch to a workspace file.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := patchWorkspaceFile(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		runtime.invalidateCodeIndex(root)
		ref, _ := output["path"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "file.diff",
		Description: "Return git diff for a workspace path.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := fileDiff(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "git://diff"}, nil
	})
	runtime.Register(Spec{
		Name:          "test.run",
		Description:   "Run the configured test command. Use this instead of shell.run for test validation.",
		RiskLevel:     "medium",
		RequiresShell: true,
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		command, _ := args["command"].(string)
		if command == "" {
			command = "go test ./..."
		}
		output, err := runCommand(ctx, root, command)
		success := err == nil
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		return Result{
			Success: success,
			Output: map[string]any{
				"command": command,
				"passed":  success,
				"summary": output,
			},
			Error:       errText,
			EvidenceRef: "exec://test.run",
		}, err
	})
	runtime.Register(Spec{
		Name:          "shell.run",
		Description:   "Run one allowlisted exec-style command in the workspace. No pipes, redirects, semicolons, glob wildcards, command substitution, environment assignments, or chained commands.",
		RiskLevel:     "medium",
		RequiresShell: true,
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		command, _ := args["command"].(string)
		output, err := runCommand(ctx, root, command)
		success := err == nil
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		return Result{
			Success: success,
			Output: map[string]any{
				"command": command,
				"passed":  success,
				"summary": output,
			},
			Error:       errText,
			EvidenceRef: "exec://shell.run",
		}, err
	})
	runtime.Register(Spec{
		Name:        "git.status",
		Description: "Return git status --short.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := gitCommand(ctx, root, "status", "--short")
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: map[string]any{"status": output}, EvidenceRef: "git://status"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.diff",
		Description: "Return git diff.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		path, _ := args["path"].(string)
		var output string
		var err error
		if strings.TrimSpace(path) == "" {
			output, err = gitCommand(ctx, root, "diff")
		} else {
			output, err = gitDiffPath(ctx, root, path)
		}
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: map[string]any{"diff": output}, EvidenceRef: "git://diff"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.branch",
		Description: "Create or checkout a git branch.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := gitBranch(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "git://branch"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.commit",
		Description: "Create a git commit from staged changes.",
		RiskLevel:   "high",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		message, _ := args["message"].(string)
		if strings.TrimSpace(message) == "" {
			return Result{Success: false, Error: "commit message is required"}, errors.New("commit message is required")
		}
		output, err := gitCommand(ctx, root, "commit", "-m", message)
		if err != nil {
			return Result{Success: false, Error: err.Error(), Output: map[string]any{"summary": output}}, err
		}
		return Result{Success: true, Output: map[string]any{"summary": output}, EvidenceRef: "git://commit"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.rollback",
		Description: "Safely restore one explicit workspace path.",
		RiskLevel:   "high",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		path, _ := args["path"].(string)
		if strings.TrimSpace(path) == "" {
			return Result{Success: false, Error: "path is required"}, errors.New("path is required")
		}
		if _, err := safeWorkspacePath(root, path); err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		output, err := gitCommand(ctx, root, "restore", "--", path)
		if err != nil {
			return Result{Success: false, Error: err.Error(), Output: map[string]any{"summary": output}}, err
		}
		return Result{Success: true, Output: map[string]any{"path": path, "summary": output}, EvidenceRef: "git://rollback"}, nil
	})
	runtime.Register(Spec{
		Name:        "artifact.write",
		Description: "Write an artifact under the configured workspace root.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := writeArtifact(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["artifact_ref"].(string)
		return Result{
			Success:     true,
			Output:      output,
			EvidenceRef: ref,
		}, nil
	})
	runtime.Register(Spec{
		Name:        "document.write",
		Description: "Write a markdown document under the configured workspace root.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := writeArtifact(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["artifact_ref"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "memory.save",
		Description: "Persist a compact project memory markdown artifact.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := saveMemory(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["memory_ref"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	return runtime
}
