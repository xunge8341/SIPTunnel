package taskengine

import (
	"fmt"

	"siptunnel/internal/repository"
)

type MachineType string

const (
	MachineCommand MachineType = "command"
	MachineFile    MachineType = "file"
)

type Transition struct {
	From repository.TaskStatus
	To   repository.TaskStatus
}

type StateMachine struct {
	machineType MachineType
	allowed     map[repository.TaskStatus]map[repository.TaskStatus]struct{}
}

func NewCommandStateMachine() *StateMachine {
	return &StateMachine{
		machineType: MachineCommand,
		allowed: map[repository.TaskStatus]map[repository.TaskStatus]struct{}{
			repository.TaskStatusPending: {
				repository.TaskStatusAccepted:  {},
				repository.TaskStatusCancelled: {},
			},
			repository.TaskStatusAccepted: {
				repository.TaskStatusRunning:   {},
				repository.TaskStatusFailed:    {},
				repository.TaskStatusCancelled: {},
			},
			repository.TaskStatusRunning: {
				repository.TaskStatusSucceeded: {},
				repository.TaskStatusFailed:    {},
				repository.TaskStatusCancelled: {},
			},
			repository.TaskStatusRetryWait: {
				repository.TaskStatusRunning: {},
				repository.TaskStatusFailed:  {},
			},
			repository.TaskStatusFailed: {
				repository.TaskStatusRetryWait:    {},
				repository.TaskStatusDeadLettered: {},
			},
		},
	}
}

func NewFileStateMachine() *StateMachine {
	return &StateMachine{
		machineType: MachineFile,
		allowed: map[repository.TaskStatus]map[repository.TaskStatus]struct{}{
			repository.TaskStatusPending: {
				repository.TaskStatusAccepted: {},
				repository.TaskStatusFailed:   {},
			},
			repository.TaskStatusAccepted: {
				repository.TaskStatusTransferring: {},
				repository.TaskStatusFailed:       {},
				repository.TaskStatusCancelled:    {},
			},
			repository.TaskStatusTransferring: {
				repository.TaskStatusVerifying: {},
				repository.TaskStatusFailed:    {},
				repository.TaskStatusCancelled: {},
				repository.TaskStatusRetryWait: {},
				repository.TaskStatusSucceeded: {},
			},
			repository.TaskStatusVerifying: {
				repository.TaskStatusSucceeded: {},
				repository.TaskStatusFailed:    {},
			},
			repository.TaskStatusRetryWait: {
				repository.TaskStatusTransferring: {},
				repository.TaskStatusFailed:       {},
			},
			repository.TaskStatusFailed: {
				repository.TaskStatusRetryWait:    {},
				repository.TaskStatusDeadLettered: {},
			},
		},
	}
}

func (s *StateMachine) Validate(t Transition) error {
	next, ok := s.allowed[t.From]
	if !ok {
		return fmt.Errorf("%s state machine unknown from status %q", s.machineType, t.From)
	}
	if _, ok := next[t.To]; !ok {
		return fmt.Errorf("%s state machine invalid transition %q -> %q", s.machineType, t.From, t.To)
	}
	return nil
}
