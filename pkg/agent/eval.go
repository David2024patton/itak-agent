package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// EvalResult holds the outcome of a single evaluation run.
type EvalResult struct {
	RunNumber  int           `json:"run_number"`
	Success    bool          `json:"success"`
	Output     string        `json:"output"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration_ms"`
}

// EvalReport summarizes an evaluation across multiple runs.
type EvalReport struct {
	TaskName     string        `json:"task_name"`
	Agent        string        `json:"agent"`
	Runs         []EvalResult  `json:"runs"`
	TotalRuns    int           `json:"total_runs"`
	Successes    int           `json:"successes"`
	Failures     int           `json:"failures"`
	SuccessRate  float64       `json:"success_rate"`
	AvgDuration  time.Duration `json:"avg_duration_ms"`
	TotalDuration time.Duration `json:"total_duration_ms"`
}

// Evaluator runs an agent multiple times on the same task to measure reliability.
type Evaluator struct {
	NumRuns int
}

// NewEvaluator creates an evaluator with a specified number of runs.
func NewEvaluator(numRuns int) *Evaluator {
	if numRuns < 1 {
		numRuns = 3
	}
	return &Evaluator{NumRuns: numRuns}
}

// Evaluate runs the agent N times on the same task and reports results.
func (e *Evaluator) Evaluate(ctx context.Context, taskName string, agent *FocusedAgent, task TaskPayload) *EvalReport {
	debug.Info("eval", "Starting evaluation %q: %d runs on agent %q", taskName, e.NumRuns, agent.Config.Name)

	report := &EvalReport{
		TaskName:  taskName,
		Agent:     agent.Config.Name,
		TotalRuns: e.NumRuns,
	}

	for i := 0; i < e.NumRuns; i++ {
		start := time.Now()

		result := agent.Run(ctx, task)
		duration := time.Since(start)

		evalResult := EvalResult{
			RunNumber: i + 1,
			Duration:  duration,
		}

		if !result.Success {
			evalResult.Success = false
			evalResult.Error = result.Error
			evalResult.Output = result.Output
			report.Failures++
		} else {
			evalResult.Success = true
			evalResult.Output = result.Output
			report.Successes++
		}

		report.Runs = append(report.Runs, evalResult)
		report.TotalDuration += duration

		status := "✓"
		if !evalResult.Success {
			status = "✗"
		}
		debug.Info("eval", "  Run %d/%d: %s (%s)", i+1, e.NumRuns, status, duration.Round(time.Millisecond))
	}

	report.SuccessRate = float64(report.Successes) / float64(report.TotalRuns) * 100
	if report.TotalRuns > 0 {
		report.AvgDuration = report.TotalDuration / time.Duration(report.TotalRuns)
	}

	debug.Info("eval", "Evaluation %q complete: %.0f%% success (%d/%d), avg %s",
		taskName, report.SuccessRate, report.Successes, report.TotalRuns, report.AvgDuration.Round(time.Millisecond))

	return report
}

// EvaluateParallel runs evaluations concurrently across multiple agents.
func (e *Evaluator) EvaluateParallel(ctx context.Context, taskName string, agents []*FocusedAgent, task TaskPayload) []*EvalReport {
	var wg sync.WaitGroup
	reports := make([]*EvalReport, len(agents))

	for i, ag := range agents {
		wg.Add(1)
		go func(idx int, agent *FocusedAgent) {
			defer wg.Done()
			reports[idx] = e.Evaluate(ctx, fmt.Sprintf("%s/%s", taskName, agent.Config.Name), agent, task)
		}(i, ag)
	}

	wg.Wait()
	return reports
}

// Summary returns a human-readable summary of an eval report.
func (r *EvalReport) Summary() string {
	return fmt.Sprintf("[%s] Agent: %s | Runs: %d | Success: %d (%.0f%%) | Avg: %s",
		r.TaskName, r.Agent, r.TotalRuns, r.Successes, r.SuccessRate, r.AvgDuration.Round(time.Millisecond))
}
