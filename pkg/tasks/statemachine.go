package tasks

import "fmt"

// transitionTable defines which status transitions are valid.
// Map key is the source status, value is a set of allowed destination statuses.
var transitionTable = map[Status][]Status{
	StatusTodo: {
		StatusInProgress, // agent picks it up
		StatusBlocked,    // dependency discovered
		StatusDone,       // manually closed
	},
	StatusInProgress: {
		StatusReview,    // work complete, send to review
		StatusBlocked,   // hit a dependency
		StatusWaiting,   // waiting for external event (webhook)
		StatusPaused,    // budget exceeded
		StatusFailed,    // unrecoverable error
		StatusEscalated, // needs human help
		StatusDone,      // completed directly
	},
	StatusBlocked: {
		StatusTodo,       // unblocked, return to queue
		StatusInProgress, // unblocked and immediately picked up
		StatusFailed,     // abandoned
	},
	StatusReview: {
		StatusDone,       // review passed
		StatusInProgress, // review failed, needs rework
		StatusFailed,     // review failed, giving up
		StatusEscalated,  // review needs human eyes
	},
	StatusWaiting: {
		StatusInProgress, // webhook fired, resume work
		StatusFailed,     // timed out or cancelled
		StatusEscalated,  // waited too long
	},
	StatusPaused: {
		StatusInProgress, // budget replenished or overridden
		StatusFailed,     // abandoned
		StatusEscalated,  // needs budget approval
	},
	StatusFailed: {
		StatusTodo,       // retry from scratch
		StatusInProgress, // direct retry
		StatusEscalated,  // escalate to human
	},
	StatusEscalated: {
		StatusTodo,       // human resolves, restarts
		StatusInProgress, // human resolves, resumes
		StatusDone,       // human closes it
		StatusFailed,     // human marks as unfixable
	},
	StatusDone: {
		// Terminal state. No outbound transitions normally.
		// Reopen requires explicit API call that bypasses the state machine.
	},
}

// ValidateTransition checks whether moving from one status to another is allowed
// by the state machine. Returns nil if valid, an error describing why if not.
func ValidateTransition(from, to Status) error {
	if from == to {
		return nil // no-op transition is always allowed
	}

	allowed, exists := transitionTable[from]
	if !exists {
		return fmt.Errorf("unknown source status: %s", from)
	}

	for _, s := range allowed {
		if s == to {
			return nil
		}
	}

	return fmt.Errorf("invalid transition: %s -> %s", from, to)
}

// AllStatuses returns every status the state machine knows about.
func AllStatuses() []Status {
	return []Status{
		StatusTodo,
		StatusInProgress,
		StatusBlocked,
		StatusReview,
		StatusDone,
		StatusFailed,
		StatusEscalated,
		StatusWaiting,
		StatusPaused,
	}
}
