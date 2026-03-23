package taskengine

import (
	"testing"

	"siptunnel/internal/repository"
)

func TestCommandStateMachineTransitions(t *testing.T) {
	sm := NewCommandStateMachine()
	if err := sm.Validate(Transition{From: repository.TaskStatusPending, To: repository.TaskStatusAccepted}); err != nil {
		t.Fatalf("expected pending->accepted to be valid: %v", err)
	}
	if err := sm.Validate(Transition{From: repository.TaskStatusAccepted, To: repository.TaskStatusVerifying}); err == nil {
		t.Fatalf("expected accepted->verifying to be invalid for command state machine")
	}
}

func TestFileStateMachineTransitions(t *testing.T) {
	sm := NewFileStateMachine()
	if err := sm.Validate(Transition{From: repository.TaskStatusTransferring, To: repository.TaskStatusVerifying}); err != nil {
		t.Fatalf("expected transferring->verifying to be valid: %v", err)
	}
	if err := sm.Validate(Transition{From: repository.TaskStatusRunning, To: repository.TaskStatusSucceeded}); err == nil {
		t.Fatalf("expected running->succeeded to be invalid for file state machine")
	}
}
