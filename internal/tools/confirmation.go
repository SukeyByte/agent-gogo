package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

type CLIConfirmationGate struct {
	reader io.Reader
	writer io.Writer
}

func NewCLIConfirmationGate(reader io.Reader, writer io.Writer) *CLIConfirmationGate {
	return &CLIConfirmationGate{reader: reader, writer: writer}
}

func (g *CLIConfirmationGate) Confirm(ctx context.Context, req ConfirmationRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if g.reader == nil {
		return false, nil
	}
	if g.writer != nil {
		_, _ = fmt.Fprintf(g.writer, "Confirm %s risk tool %q? Type yes to approve: ", req.RiskLevel, req.ToolName)
	}
	scanner := bufio.NewScanner(g.reader)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "yes" || answer == "y", nil
}
