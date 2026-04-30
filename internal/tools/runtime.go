package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/capability"
	"github.com/SukeyByte/agent-gogo/internal/domain"
)

func (r *Runtime) Call(ctx context.Context, req CallRequest) (CallResponse, error) {
	if err := ctx.Err(); err != nil {
		return CallResponse{}, err
	}
	req = normalizeCallRequest(req)
	inputJSON, err := stableJSON(req.Args)
	if err != nil {
		return CallResponse{}, err
	}
	r.log(ctx, "tool.call.request", map[string]any{
		"attempt_id": req.AttemptID,
		"name":       req.Name,
		"args":       copyArgs(req.Args),
	})

	handler, ok := r.handlers[req.Name]
	if !ok {
		result := Result{Success: false, Error: ErrToolNotFound.Error()}
		call, auditErr := r.audit(ctx, req, inputJSON, result)
		if auditErr != nil {
			return CallResponse{}, auditErr
		}
		return CallResponse{Result: result, ToolCall: call}, ErrToolNotFound
	}
	spec := r.specs[req.Name]
	if err := r.resolveCapability(ctx, spec, req); err != nil {
		result := Result{Success: false, Error: err.Error()}
		call, auditErr := r.audit(ctx, req, inputJSON, result)
		if auditErr != nil {
			return CallResponse{}, auditErr
		}
		return CallResponse{Result: result, ToolCall: call}, err
	}

	result, handlerErr := handler(ctx, copyArgs(req.Args))
	if handlerErr != nil {
		result.Success = false
		result.Error = handlerErr.Error()
	}
	call, err := r.audit(ctx, req, inputJSON, result)
	if err != nil {
		return CallResponse{}, err
	}
	if handlerErr != nil {
		return CallResponse{Result: result, ToolCall: call}, handlerErr
	}
	if !result.Success && result.Error != "" {
		return CallResponse{Result: result, ToolCall: call}, fmt.Errorf("tool %s failed: %s", req.Name, result.Error)
	}
	return CallResponse{Result: result, ToolCall: call}, nil
}

func normalizeCallRequest(req CallRequest) CallRequest {
	req.Args = copyArgs(req.Args)
	if req.Name != "test.run" {
		return req
	}
	command, _ := req.Args["command"].(string)
	command = strings.TrimSpace(command)
	switch {
	case command == "":
		req.Args["command"] = "go test ./..."
	case command == "./..." || command == "..." || strings.HasPrefix(command, "./"):
		req.Args["command"] = "go test " + command
	}
	return req
}

func (r *Runtime) resolveCapability(ctx context.Context, spec Spec, req CallRequest) error {
	if r.capabilities == nil {
		r.capabilities = capability.NewRegistry()
	}
	r.capabilities.RegisterTool(capability.ToolSpec{
		Name:          spec.Name,
		Description:   spec.Description,
		RiskLevel:     spec.RiskLevel,
		RequiresShell: spec.RequiresShell,
	})
	resolution, err := r.capabilities.ResolveTool(ctx, capability.ToolRequest{
		ToolName: req.Name,
		Args:     copyArgs(req.Args),
		Policy: capability.Policy{
			AllowedTools:              r.security.AllowedTools,
			AllowShell:                r.security.AllowShell,
			ShellAllowlist:            append([]string(nil), r.security.ShellAllowlist...),
			RequireConfirmationAtRisk: r.security.RequireConfirmationAtRisk,
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCapabilityBlocked, err)
	}
	if resolution.RequiresConfirmation {
		if r.confirmationGate == nil {
			return fmt.Errorf("%w: %s", ErrConfirmationRequired, req.Name)
		}
		approved, err := r.confirmationGate.Confirm(ctx, ConfirmationRequest{
			ToolName:  req.Name,
			RiskLevel: spec.RiskLevel,
			Args:      copyArgs(req.Args),
		})
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("%w: %s rejected", ErrConfirmationRequired, req.Name)
		}
	}
	return nil
}

func (r *Runtime) audit(ctx context.Context, req CallRequest, inputJSON string, result Result) (domain.ToolCall, error) {
	outputJSON, err := stableJSON(result.Output)
	if err != nil {
		return domain.ToolCall{}, err
	}
	status := domain.ToolCallStatusSucceeded
	if !result.Success {
		status = domain.ToolCallStatusFailed
	}
	call := domain.ToolCall{
		AttemptID:   req.AttemptID,
		Name:        req.Name,
		InputJSON:   inputJSON,
		OutputJSON:  outputJSON,
		Status:      status,
		Error:       result.Error,
		EvidenceRef: result.EvidenceRef,
	}
	if r.store == nil {
		r.log(ctx, "tool.call.response", map[string]any{
			"attempt_id":   call.AttemptID,
			"name":         call.Name,
			"status":       call.Status,
			"error":        call.Error,
			"evidence_ref": call.EvidenceRef,
			"output":       result.Output,
		})
		return call, nil
	}
	created, err := r.store.CreateToolCall(ctx, call)
	if err != nil {
		return domain.ToolCall{}, err
	}
	r.log(ctx, "tool.call.response", map[string]any{
		"id":           created.ID,
		"attempt_id":   created.AttemptID,
		"name":         created.Name,
		"status":       created.Status,
		"error":        created.Error,
		"evidence_ref": created.EvidenceRef,
		"output":       result.Output,
	})
	return created, nil
}

func (r *Runtime) log(ctx context.Context, stage string, payload any) {
	if r.logger == nil {
		return
	}
	_ = r.logger.Log(ctx, stage, payload)
}

func stableJSON(value any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func copyArgs(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(args))
	for key, value := range args {
		result[key] = value
	}
	return result
}
