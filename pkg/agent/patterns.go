package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/David2024patton/GOAgent/pkg/debug"
)

// ChainRunner executes agents sequentially  -  each agent's output becomes the next agent's input.
type ChainRunner struct {
	Name   string
	Agents []*FocusedAgent
}

// NewChainRunner creates a sequential pipeline of agents.
func NewChainRunner(name string, agents ...*FocusedAgent) *ChainRunner {
	return &ChainRunner{
		Name:   name,
		Agents: agents,
	}
}

// Run executes the chain: input → Agent1 → output1 → Agent2 → output2 → ...
func (cr *ChainRunner) Run(ctx context.Context, input string) (string, error) {
	debug.Info("chain", "Starting chain %q with %d agents", cr.Name, len(cr.Agents))

	current := input
	for i, ag := range cr.Agents {
		debug.Info("chain", "  Step %d/%d: %s", i+1, len(cr.Agents), ag.Config.Name)

		result := ag.Run(ctx, TaskPayload{
			Agent:   ag.Config.Name,
			Task:    current,
			Context: fmt.Sprintf("You are step %d of %d in a pipeline. Process the input and pass your output forward.", i+1, len(cr.Agents)),
		})

		if !result.Success {
			debug.Warn("chain", "Step %d (%s) returned failure: %s", i+1, ag.Config.Name, result.Error)
			return "", fmt.Errorf("chain step %d (%s) failed: %s", i+1, ag.Config.Name, result.Error)
		}

		current = result.Output
		debug.Debug("chain", "Step %d output (%d chars)", i+1, len(current))
	}

	debug.Info("chain", "Chain %q completed successfully", cr.Name)
	return current, nil
}

// ParallelRunner executes multiple agents concurrently and collects their results.
type ParallelRunner struct {
	Name   string
	Agents []*FocusedAgent
}

// NewParallelRunner creates a concurrent agent executor.
func NewParallelRunner(name string, agents ...*FocusedAgent) *ParallelRunner {
	return &ParallelRunner{
		Name:   name,
		Agents: agents,
	}
}

// ParallelResult holds the output of a single parallel agent.
type ParallelResult struct {
	Agent   string
	Output  string
	Success bool
	Error   string
}

// Run executes all agents concurrently and waits for all to complete.
func (pr *ParallelRunner) Run(ctx context.Context, input string) ([]ParallelResult, error) {
	debug.Info("parallel", "Starting parallel %q with %d agents", pr.Name, len(pr.Agents))

	var wg sync.WaitGroup
	results := make([]ParallelResult, len(pr.Agents))

	for i, ag := range pr.Agents {
		wg.Add(1)
		go func(idx int, agent *FocusedAgent) {
			defer wg.Done()

			debug.Debug("parallel", "  Agent %s starting", agent.Config.Name)

			result := agent.Run(ctx, TaskPayload{
				Agent:   agent.Config.Name,
				Task:    input,
				Context: fmt.Sprintf("You are running in parallel with %d other agents. Work independently.", len(pr.Agents)-1),
			})

			results[idx] = ParallelResult{
				Agent:   agent.Config.Name,
				Output:  result.Output,
				Success: result.Success,
				Error:   result.Error,
			}

			debug.Debug("parallel", "  Agent %s completed (success: %v)", agent.Config.Name, result.Success)
		}(i, ag)
	}

	wg.Wait()

	// Count successes.
	successes := 0
	for _, r := range results {
		if r.Success {
			successes++
		}
	}

	debug.Info("parallel", "Parallel %q completed: %d/%d succeeded", pr.Name, successes, len(pr.Agents))
	return results, nil
}
