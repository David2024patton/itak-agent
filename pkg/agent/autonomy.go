package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
)

// AutonomyLevel controls how independently an agent operates.
// Higher levels mean less human involvement. The agent earns
// higher autonomy over time based on success rate (promotion)
// and gets demoted on repeated failures.
type AutonomyLevel int

const (
	// AutonomySupervised asks before every action. Training mode.
	AutonomySupervised AutonomyLevel = 0

	// AutonomyGuided asks before destructive actions (delete, deploy, send).
	// Read-only operations are auto-approved.
	AutonomyGuided AutonomyLevel = 1

	// AutonomyCollaborative only asks when confidence < 50% or task is novel.
	// Default for new installs.
	AutonomyCollaborative AutonomyLevel = 2

	// AutonomyAutonomous only asks when confidence < 20% or facing
	// a genuinely unknown situation.
	AutonomyAutonomous AutonomyLevel = 3

	// AutonomyFullAutopilot never asks. Handles everything. Reports results only.
	// For trusted, well-tested setups.
	AutonomyFullAutopilot AutonomyLevel = 4
)

// String returns the human-readable name for an autonomy level.
func (a AutonomyLevel) String() string {
	switch a {
	case AutonomySupervised:
		return "supervised"
	case AutonomyGuided:
		return "guided"
	case AutonomyCollaborative:
		return "collaborative"
	case AutonomyAutonomous:
		return "autonomous"
	case AutonomyFullAutopilot:
		return "full_autopilot"
	default:
		return fmt.Sprintf("unknown(%d)", a)
	}
}

// ParseAutonomyLevel converts a string to an AutonomyLevel.
// Returns AutonomyCollaborative (the safe default) if unrecognized.
func ParseAutonomyLevel(s string) AutonomyLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "supervised", "0":
		return AutonomySupervised
	case "guided", "1":
		return AutonomyGuided
	case "collaborative", "2":
		return AutonomyCollaborative
	case "autonomous", "3":
		return AutonomyAutonomous
	case "full_autopilot", "autopilot", "4":
		return AutonomyFullAutopilot
	default:
		return AutonomyCollaborative
	}
}

// EscalationStep represents one step in the 7-step escalation chain.
type EscalationStep int

const (
	StepRetry           EscalationStep = iota // Same model, same approach, 2 attempts
	StepCheckErrorDB                          // Has this error been seen before? Apply known fix
	StepEscalateModel                         // Try a bigger/smarter model on the same task
	StepTryDifferent                          // Rephrase the task, use alternative tools
	StepResearchOnline                        // Search the web for the error message
	StepSelfDebug                             // Dump full state to a fresh agent for diagnosis
	StepAskUser                               // Only after ALL above steps fail
)

// String returns the human-readable name for an escalation step.
func (s EscalationStep) String() string {
	names := []string{
		"retry", "check_error_db", "escalate_model",
		"try_different", "research_online", "self_debug", "ask_user",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return fmt.Sprintf("unknown(%d)", s)
}

// EscalationChain implements the 7-step escalation pipeline.
// When an agent fails a task, the chain walks through progressively
// more expensive recovery strategies before ever asking the user.
//
// Why: Small models fail often. Without escalation, every failure
// requires human intervention. The chain makes the agent try harder
// on its own first, saving the user's time.
//
// How: Each step is attempted in order. If a step succeeds, the chain
// returns immediately. If all steps fail, the final step "ask_user"
// packages up what was tried and what was found for the human.
type EscalationChain struct {
	Agent      *FocusedAgent
	Doctor     *Doctor
	Bus        *eventbus.EventBus
	MaxRetries int // retries per step (default: 2)
}

// NewEscalationChain creates an escalation chain for a focused agent.
func NewEscalationChain(agent *FocusedAgent, doctor *Doctor, bus *eventbus.EventBus) *EscalationChain {
	return &EscalationChain{
		Agent:      agent,
		Doctor:     doctor,
		Bus:        bus,
		MaxRetries: 2,
	}
}

// EscalationResult captures the outcome of the escalation process.
type EscalationResult struct {
	Result       Result         `json:"result"`        // the final agent result
	StepsTriad   []string       `json:"steps_tried"`   // which escalation steps were attempted
	FinalStep    EscalationStep `json:"final_step"`    // which step resolved the issue (or StepAskUser)
	TotalRetries int            `json:"total_retries"` // how many total attempts across all steps
}

// RunWithEscalation wraps FocusedAgent.Run() with the 7-step escalation chain.
// It progressively tries harder recovery strategies before giving up.
func (ec *EscalationChain) RunWithEscalation(ctx context.Context, task TaskPayload) EscalationResult {
	tag := ec.Agent.Config.Name
	result := EscalationResult{}

	// Step 0: First attempt (the normal path).
	debug.Info(tag, "Escalation: initial attempt")
	agentResult := ec.Agent.Run(ctx, task)
	result.TotalRetries++
	if agentResult.Success {
		result.Result = agentResult
		return result
	}

	debug.Warn(tag, "Escalation: initial attempt failed, starting escalation chain")
	lastError := agentResult.Error

	// Step 1: Retry (same model, same approach).
	result.StepsTriad = append(result.StepsTriad, StepRetry.String())
	for i := 0; i < ec.MaxRetries; i++ {
		debug.Info(tag, "Escalation [retry]: attempt %d/%d", i+1, ec.MaxRetries)
		agentResult = ec.Agent.Run(ctx, task)
		result.TotalRetries++
		if agentResult.Success {
			result.Result = agentResult
			result.FinalStep = StepRetry
			return result
		}
		lastError = agentResult.Error
	}

	// Step 2: Check Error DB (look up known fix from Doctor).
	result.StepsTriad = append(result.StepsTriad, StepCheckErrorDB.String())
	if ec.Doctor != nil {
		fix, found := ec.Doctor.LookupFix(lastError)
		if found {
			debug.Info(tag, "Escalation [error_db]: known fix found (seen %d times): %s",
				fix.FixCount, truncate(fix.FixApplied, 100))

			// Retry the task -- the fix might have been applied by the Doctor already.
			agentResult = ec.Agent.Run(ctx, task)
			result.TotalRetries++
			if agentResult.Success {
				result.Result = agentResult
				result.FinalStep = StepCheckErrorDB
				return result
			}
			lastError = agentResult.Error
		}
	}

	// Step 3: Try Different Approach (rephrase the task with error context).
	result.StepsTriad = append(result.StepsTriad, StepTryDifferent.String())
	modifiedTask := TaskPayload{
		Agent: task.Agent,
		Task:  task.Task,
		Context: fmt.Sprintf(
			"PREVIOUS ATTEMPT FAILED with error: %s\n\nTry a DIFFERENT approach. "+
				"Use different tools or strategies than before.\n\nOriginal context: %s",
			truncate(lastError, 300), task.Context),
	}
	debug.Info(tag, "Escalation [try_different]: retrying with error context")
	agentResult = ec.Agent.Run(ctx, modifiedTask)
	result.TotalRetries++
	if agentResult.Success {
		result.Result = agentResult
		result.FinalStep = StepTryDifferent
		return result
	}
	lastError = agentResult.Error

	// Steps 4-5 (Research Online, Self-Debug) require browser/web tools
	// and additional infrastructure. Mark as tried but skip for MVP.
	result.StepsTriad = append(result.StepsTriad, StepResearchOnline.String()+" (skipped:MVP)")
	result.StepsTriad = append(result.StepsTriad, StepSelfDebug.String()+" (skipped:MVP)")

	// Step 6: Ask User (final fallback).
	result.StepsTriad = append(result.StepsTriad, StepAskUser.String())
	result.FinalStep = StepAskUser
	result.Result = Result{
		Agent:   tag,
		Success: false,
		Error: fmt.Sprintf(
			"Task failed after %d attempts across %d escalation steps.\n"+
				"Last error: %s\n"+
				"Steps tried: %s",
			result.TotalRetries,
			len(result.StepsTriad),
			lastError,
			strings.Join(result.StepsTriad, " > ")),
	}

	// Emit escalation exhausted event.
	if ec.Bus != nil {
		ec.Bus.Publish(eventbus.Event{
			Topic:   TopicEscalationExhausted,
			Agent:   tag,
			Message: fmt.Sprintf("Escalation exhausted after %d steps", len(result.StepsTriad)),
			Data: map[string]interface{}{
				"steps_tried":   result.StepsTriad,
				"total_retries": result.TotalRetries,
				"last_error":    truncate(lastError, 500),
			},
			Timestamp: time.Now(),
		})
	}

	debug.Warn(tag, "Escalation EXHAUSTED: %d steps, %d retries. Asking user.",
		len(result.StepsTriad), result.TotalRetries)

	return result
}

// ── Additional event bus topics for autonomy ──

const (
	TopicEscalationExhausted = "agent.escalation_exhausted"
	TopicDoctorActivated     = "doctor.activated"
	TopicDoctorClear         = "doctor.clear"
)
