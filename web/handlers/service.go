package handlers

import "context"

type ChannelEventSender interface {
	HandleChannelEvent(ctx context.Context, event InboundEvent) error
	HandleUserConfirmation(ctx context.Context, confirmation InboundConfirmation) error
}

type InboundEvent struct {
	Type      string            `json:"type"`
	ChannelID string            `json:"channel_id"`
	SessionID string            `json:"session_id"`
	ProjectID string            `json:"project_id"`
	TaskID    string            `json:"task_id"`
	Text      string            `json:"text"`
	Payload   map[string]string `json:"payload"`
}

type InboundConfirmation struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id"`
	AttemptID string `json:"attempt_id"`
	ActionID  string `json:"action_id"`
	Approved  bool   `json:"approved"`
	Message   string `json:"message"`
}
