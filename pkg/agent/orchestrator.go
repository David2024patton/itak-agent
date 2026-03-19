package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
	"github.com/David2024patton/iTaKAgent/pkg/llm"
	"github.com/David2024patton/iTaKAgent/pkg/memory"
	"github.com/David2024patton/iTaKAgent/pkg/task"
	"github.com/David2024patton/iTaKAgent/pkg/tasks"
)

// delegationSystemPrompt is the baked-in system prompt for the orchestrator.
// It tells the LLM to ONLY reason and delegate  -  never use tools.
// %d = agent count, %s = data directory, %s = agent descriptions
const delegationSystemPrompt = `You are iTaKAgent v0.2.0, an AI assistant created by David Patton.
You have %d agent(s) you can delegate tasks to. They are listed below.
For a full list of your capabilities, refer to the file: capabilities.md in your data directory.

WHEN TO DELEGATE:
- Files/folders/data: delegate to "scout"
- Create files or run commands: delegate to "operator"
- Visit websites: delegate to "browser"
- Research/URLs: delegate to "researcher"
- Write code: delegate to "coder"
- Build new projects: delegate to "architect"

WHEN TO ANSWER DIRECTLY (no delegation):
- Greetings, identity questions, capability questions
- Simple questions you already know the answer to

Data directory: %s

AVAILABLE AGENTS:
%s

RESPOND IN JSON:
For delegation:
{"reasoning": "why", "delegations": [{"agent": "name", "task": "what to do", "context": "details"}]}

For direct answers:
{"reasoning": "why", "delegations": [], "direct_response": "your answer"}

IMPORTANT: Always include "direct_response" when answering directly. Never leave both delegations and direct_response empty.`


// synthesisSystemPrompt is used when the orchestrator combines agent results.
const synthesisSystemPrompt = `You are the iTaKAgent Orchestrator synthesizing results.
Given the original user request and the results from your focused agents, create a clear, helpful final response.
Be concise. Present the information naturally. Do not mention agents or delegation mechanics to the user.

STYLE RULES (MANDATORY):
- NEVER use em dashes or en dashes. Use commas, periods, colons, or parentheses instead.
- Avoid AI slop: do not use phrases like "in today's fast-paced world", "it's worth noting", "dive deep", "leverage", "harness the power", "game-changer".
- Write concise, direct prose. Do not repeat the same idea in different words.
- Use hyphens (-) only for compound adjectives (e.g., "pest-control site"). Never as a stand-in for em dashes.`

// NewOrchestrator creates an orchestrator with its LLM client and registered agents.
func NewOrchestrator(cfg OrchestratorConfig, agents map[string]*FocusedAgent, mem *memory.Manager, trace *debug.StepLogger, tokens *llm.TokenTracker, bus *eventbus.EventBus) *Orchestrator {
	client := llm.NewOpenAIClient(cfg.LLM)

	// Build the agent registry from the provided agents map.
	registry := NewRegistry()
	for name, fa := range agents {
		registry.Register(AgentEntry{
			Name:        name,
			Role:        fa.Config.Role,
			Personality: fa.Config.Personality,
			Tools:       fa.Config.ToolNames,
		})
	}

	return &Orchestrator{
		Config:    cfg,
		LLMClient: client,
		Agents:    agents,
		Registry:  registry,
		Memory:    mem,
		Trace:     trace,
		Tokens:    tokens,
		Bus:       bus,
		Tasks:     task.NewTracker(100),
	}
}

// emit publishes an event to the bus if one is attached.
func (o *Orchestrator) emit(e eventbus.Event) {
	if o.Bus != nil {
		o.Bus.Publish(e)
	}
}

// emitError publishes a categorized agent.error event for the Doctor.
func (o *Orchestrator) emitError(category, message string) {
	o.emit(eventbus.Event{
		Topic:   eventbus.TopicAgentError,
		Agent:   "orchestrator",
		Message: message,
		Data: map[string]interface{}{
			"category": category,
		},
	})
}

// Run processes a user message through the orchestrator pipeline:
// 1. Reason about the request
// 2. Delegate to focused agents
// 3. Collect results
// 4. Synthesize final response
func (o *Orchestrator) Run(ctx context.Context, userMessage string) (string, error) {
	debug.Info("orchestrator", "Processing: %s", truncate(userMessage, 80))
	debug.Separator("orchestrator")

	// ── Doctor Halt/Resume: check if the Doctor is actively healing ──
	// If the Doctor is fixing something, wait for it to finish before delegating.
	if o.Bus != nil && o.isDoctorActive() {
		debug.Warn("orchestrator", "Doctor is active -- waiting for clear signal before delegating")
		if !o.waitForDoctorClear(ctx, 30*time.Second) {
			debug.Warn("orchestrator", "Doctor did not clear within timeout -- proceeding anyway")
		}
	}

	// ── Prompt Injection Defense ──
	// Scan user input BEFORE any processing.
	if o.Guard != nil {
		scan := o.Guard.ScanInput(userMessage, "user")
		if scan.Blocked {
			debug.Warn("orchestrator", "INPUT BLOCKED by guard (severity=%s reasons=%v)",
				scan.Severity, scan.Reasons)
			o.emitError("input_blocked", fmt.Sprintf("Guard blocked input: %v", scan.Reasons))
			return "I can't process that request. It was flagged by the security system.", nil
		}
	}

	// Trace + Bus: user message received.
	if o.Trace != nil {
		o.Trace.Record(debug.StepUserMessage, "orchestrator", "", userMessage, "", nil)
	}
	o.emit(eventbus.NewEvent(eventbus.TopicUserInput, userMessage))

	// Log user message to session memory and archive.
	if o.Memory != nil {
		userMsg := llm.Message{Role: llm.RoleUser, Content: userMessage}
		o.Memory.Session.Add(userMsg)
		o.Memory.Archive.LogMessage(memory.LogMessage{
			Role:      string(llm.RoleUser),
			Content:   userMessage,
			Timestamp: time.Now(),
		})

		// Auto-track entities in user messages.
		if o.Config.Memory.AutoEntities {
			o.Memory.Entities.Track(userMessage, o.Memory.Archive.NextID())
		}

		// Chat compression: when messages exceed the window, compress old ones
		// into a brief recap. This keeps context tight for tiny models (2B-4B).
		// Uses lightweight text compression (no extra LLM call) to save tokens.
		if o.Memory.Session.NeedsSummarization() {
			oldMsgs := o.Memory.Session.GetOldMessages()
			if len(oldMsgs) > 0 {
				recap := compressMessages(oldMsgs)
				o.Memory.Session.SetSummary(recap)
				debug.Info("orchestrator", "Compressed %d old messages into %d-char recap", len(oldMsgs), len(recap))
			}
		}
	}

	// ── Workflow Router: check predefined templates BEFORE calling the LLM ──
	// For tiny models (2B-4B), structured JSON output is unreliable.
	// Workflow templates use keyword matching to route common requests
	// through the correct multi-agent pipeline without any LLM call.
	if tpl, matched := matchWorkflow(userMessage); matched {
		debug.Info("orchestrator", "Workflow matched: %q, skipping LLM routing", tpl.Name)
		o.status(fmt.Sprintf("iTaKAgent Workflow: %s", tpl.Name))

		delegation := buildWorkflowDelegation(tpl, userMessage)

		// Trace + Bus: workflow match event.
		if o.Trace != nil {
			o.Trace.Record(debug.StepDelegation, "orchestrator", "", delegation.Reasoning, "", map[string]interface{}{
				"workflow":    tpl.Name,
				"agent_count": len(delegation.Delegations),
			})
		}

		// Jump directly to delegation execution (skip LLM call).
		return o.executeDelegations(ctx, userMessage, delegation)
	}

	// Build the agent descriptions for the system prompt.
	agentDescs := o.buildAgentDescriptions()
	dataDir := ""
	if o.Memory != nil {
		dataDir = o.Memory.DataDir
	}
	sysPrompt := fmt.Sprintf(delegationSystemPrompt, len(o.Agents), dataDir, agentDescs)

	if o.Config.SystemPrompt != "" {
		sysPrompt = o.Config.SystemPrompt + "\n\n" + sysPrompt
	}

	debug.Debug("orchestrator", "System prompt length: %d chars, %d agents available", len(sysPrompt), len(o.Agents))

	// Build messages WITH conversation history from session memory.
	messages := []llm.Message{{Role: llm.RoleSystem, Content: sysPrompt}}
	if o.Memory != nil {
		history := o.Memory.Session.GetContext()
		messages = append(messages, history...)
		debug.Debug("orchestrator", "Including %d messages of conversation history", len(history))
	} else {
		// Fallback if no memory: just use the current message.
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMessage})
	}

	// Status: Thinking
	o.status("iTaKAgent Thinking...")

	debug.Info("orchestrator", "Calling LLM for delegation decision...")

	// Wrap with a 60s timeout so slow/down LLM backends don't hang forever.
	llmCtx, llmCancel := context.WithTimeout(ctx, 60*time.Second)
	defer llmCancel()

	resp, err := o.LLMClient.Chat(llmCtx, messages, nil) // no tools for orchestrator
	if err != nil {
		debug.Error("orchestrator", "LLM call failed: %v", err)
		o.emitError("orchestrator_llm_failure", fmt.Sprintf("Orchestrator LLM call failed: %v", err))
		return "", fmt.Errorf("orchestrator LLM call: %w", err)
	}

	debug.JSON("orchestrator", "Raw LLM response", resp.Content)
	if resp.Usage != nil {
		debug.Debug("orchestrator", "Tokens  -  prompt: %d, completion: %d, total: %d",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}

	// Step 2: Parse the delegation decision.
	delegation, directResponse, err := parseDelegation(resp.Content)
	if err != nil {
		debug.Error("orchestrator", "Failed to parse delegation: %v", err)
		o.emitError("delegation_parse_failure", fmt.Sprintf("Failed to parse delegation: %v", err))
		return "", fmt.Errorf("parse delegation: %w", err)
	}

	// If the orchestrator answered directly, log and return it.
	if directResponse != "" {
		debug.Info("orchestrator", "Direct response (no delegation): %s", truncate(directResponse, 100))
		o.logAssistantResponse(directResponse)
		return directResponse, nil
	}

	if len(delegation.Delegations) == 0 {
		debug.Warn("orchestrator", "No delegations and no direct response  -  unclear request")
		fallback := "I wasn't sure how to help with that. Could you rephrase?"
		o.logAssistantResponse(fallback)
		return fallback, nil
	}

	debug.Info("orchestrator", "Reasoning: %s", truncate(delegation.Reasoning, 150))
	debug.Info("orchestrator", "Delegating to %d agent(s)", len(delegation.Delegations))

	// Trace + Bus: delegation decision.
	if o.Trace != nil {
		o.Trace.Record(debug.StepDelegation, "orchestrator", "", delegation.Reasoning, "", map[string]interface{}{
			"agent_count": len(delegation.Delegations),
		})
	}

	return o.executeDelegations(ctx, userMessage, delegation)
}

// executeDelegations runs the delegation pipeline: creates tasks, executes agents
// in sequence with pipeline chaining, and synthesizes the final response.
// This is shared between the workflow router path and the LLM-based routing path.
func (o *Orchestrator) executeDelegations(ctx context.Context, userMessage string, delegation *Delegation) (string, error) {
	o.emit(eventbus.Event{
		Topic:   eventbus.TopicOrchestratorDelegation,
		Message: delegation.Reasoning,
		Data:    map[string]interface{}{"agent_count": len(delegation.Delegations)},
	})

	// Step 3: Create mandatory task list + dashboard tasks.
	taskList := o.Tasks.Create(userMessage)
	dashboardTaskIDs := make(map[string]string) // step-N -> dashboard task ID
	for i, t := range delegation.Delegations {
		stepID := fmt.Sprintf("step-%d", i+1)
		taskList.AddItem(stepID, t.Task, t.Agent)

		// Bridge: create a real dashboard task so the user sees it on the board.
		if o.DashboardTasks != nil {
			priority := tasks.PriorityMedium
			dt, createErr := o.DashboardTasks.CreateTask(
				t.Task,             // title
				t.Context,          // description
				priority,           // priority
				nil,                // labels
				nil,                // due date
				o.ActiveProjectID,  // project_id
			)
			if createErr == nil {
				dashboardTaskIDs[stepID] = dt.ID
				// Set assigned agent immediately.
				o.DashboardTasks.UpdateTask(
					dt.ID, t.Task, t.Context,
					tasks.StatusInProgress, t.Agent,
					priority, nil, nil, nil,
					o.ActiveProjectID, nil, nil, "",
				)
				debug.Info("orchestrator", "Dashboard task %s created for %s -> %s", dt.ID, stepID, t.Agent)
			}
		}
	}

	o.status(fmt.Sprintf("iTaKAgent Task: %s", taskList.Summary()))

	// Step 4: Execute delegations with task tracking, pipeline chaining, and swarm parallelism.
	// Sequential tasks get the previous agent's output in context (pipeline chaining).
	// Consecutive swarm tasks are batched and executed in parallel via goroutines.
	results := make([]Result, 0, len(delegation.Delegations))
	var pipelineOutput string // output from previous delegation for chaining
	for i := 0; i < len(delegation.Delegations); i++ {
		dtask := delegation.Delegations[i]
		taskID := fmt.Sprintf("step-%d", i+1)

		// ── Swarm Batch Detection ──
		// If this task is marked as Swarm, collect all consecutive swarm tasks
		// and execute them in parallel.
		if dtask.Swarm {
			swarmBatch := []TaskPayload{dtask}
			swarmIDs := []string{taskID}
			// Collect consecutive swarm tasks.
			for j := i + 1; j < len(delegation.Delegations) && delegation.Delegations[j].Swarm; j++ {
				swarmBatch = append(swarmBatch, delegation.Delegations[j])
				swarmIDs = append(swarmIDs, fmt.Sprintf("step-%d", j+1))
			}

			debug.Separator("swarm")
			debug.Info("orchestrator", "SWARM: launching %d parallel tasks", len(swarmBatch))
			o.status(fmt.Sprintf("iTaKAgent Swarm: %d pages in parallel...", len(swarmBatch)))

			// Inject pipeline output from previous step into ALL swarm tasks.
			if pipelineOutput != "" {
				for k := range swarmBatch {
					pipelinePrefix := fmt.Sprintf("[PIPELINE INPUT - shared CSS/layout from previous step]:\n%s\n\n[YOUR TASK]:\n",
						truncate(pipelineOutput, 6000))
					if swarmBatch[k].Context != "" {
						swarmBatch[k].Context = pipelinePrefix + swarmBatch[k].Context
					} else {
						swarmBatch[k].Context = pipelinePrefix + swarmBatch[k].Task
					}
				}
			}

			// Execute swarm tasks in parallel.
			swarmResults := make([]Result, len(swarmBatch))
			var wg sync.WaitGroup
			for k, st := range swarmBatch {
				agent, ok := o.Agents[st.Agent]
				if !ok {
					swarmResults[k] = Result{Agent: st.Agent, Success: false, Error: fmt.Sprintf("unknown agent: %s", st.Agent)}
					continue
				}
				wg.Add(1)
				go func(idx int, task TaskPayload, ag *FocusedAgent) {
					defer wg.Done()
					debug.Info("swarm", "Worker %d/%d starting: %s", idx+1, len(swarmBatch), truncate(task.Task, 80))
					swarmResults[idx] = ag.Run(ctx, task)
					if swarmResults[idx].Success {
						debug.Info("swarm", "Worker %d/%d done (%d chars)", idx+1, len(swarmBatch), len(swarmResults[idx].Output))
					} else {
						debug.Warn("swarm", "Worker %d/%d failed: %s", idx+1, len(swarmBatch), swarmResults[idx].Error)
					}
				}(k, st, agent)
			}
			wg.Wait()

			debug.Info("orchestrator", "SWARM: all %d workers completed", len(swarmBatch))

			// Merge swarm results: combine all outputs, extract+write files from each.
			var mergedOutput strings.Builder
			projectDir := "/app/data/projects/latest"
			if o.Memory != nil && o.Memory.DataDir != "" {
				projectDir = o.Memory.DataDir + "/projects/latest"
			}
			allFiles := make(map[string]string)

			for k, sr := range swarmResults {
				results = append(results, sr)
				sid := swarmIDs[k]
				if sr.Success {
					taskList.Complete(sid, truncate(sr.Output, 200))
					mergedOutput.WriteString(sr.Output)
					mergedOutput.WriteString("\n\n")

					// Extract code files from each swarm worker's output.
					codeFiles := extractCodeFiles(sr.Output)
					for name, content := range codeFiles {
						allFiles[name] = content
					}

					if dtID, ok := dashboardTaskIDs[sid]; ok && o.DashboardTasks != nil {
						o.DashboardTasks.UpdateTaskStatus(dtID, tasks.StatusDone)
					}
				} else {
					taskList.Fail(sid, sr.Error)
					if dtID, ok := dashboardTaskIDs[sid]; ok && o.DashboardTasks != nil {
						o.DashboardTasks.UpdateTaskStatus(dtID, tasks.StatusTodo)
					}
				}
			}

			// Write all extracted files at once.
			if len(allFiles) > 0 {
				if err := writeCodeFiles(projectDir, allFiles); err != nil {
					debug.Warn("orchestrator", "Swarm: failed to write files: %v", err)
				} else {
					debug.Info("orchestrator", "Swarm: wrote %d files to %s", len(allFiles), projectDir)
					var fileList strings.Builder
					fileList.WriteString("\n\nFiles created (swarm):\n")
					for name := range allFiles {
						fileList.WriteString("  - " + name + "\n")
					}
					mergedOutput.WriteString(fileList.String())
				}
			}

			pipelineOutput = mergedOutput.String()

			// Skip past the swarm batch in the outer loop.
			i += len(swarmBatch) - 1
			continue
		}

		// ── Sequential Task Execution ──
		agent, ok := o.Agents[dtask.Agent]

		// Pipeline chaining: inject previous agent's output into this agent's context.
		if pipelineOutput != "" && i > 0 {
			pipelinePrefix := fmt.Sprintf("[PIPELINE INPUT from previous agent]:\n%s\n\n[YOUR TASK]:\n",
				truncate(pipelineOutput, 4000))
			if dtask.Context != "" {
				dtask.Context = pipelinePrefix + dtask.Context
			} else {
				dtask.Context = pipelinePrefix + dtask.Task
			}
			debug.Debug("orchestrator", "Pipeline: injected %d chars from previous agent into %q context",
				len(pipelineOutput), dtask.Agent)
		}

		if !ok {
			debug.Error("orchestrator", "Unknown agent %q in delegation %d", dtask.Agent, i+1)
			o.emitError("unknown_agent", fmt.Sprintf("unknown agent %q in delegation", dtask.Agent))
			taskList.Fail(taskID, fmt.Sprintf("unknown agent: %s", dtask.Agent))
			results = append(results, Result{
				Agent:   dtask.Agent,
				Success: false,
				Error:   fmt.Sprintf("unknown agent: %s", dtask.Agent),
			})
			continue
		}

		// Mark task as running.
		taskList.Start(taskID)
		o.status(fmt.Sprintf("iTaKAgent Delegating to %s...", dtask.Agent))

		debug.Separator(dtask.Agent)
		debug.Info("orchestrator", "-> Delegating [%d/%d] to %q: %s",
			i+1, len(delegation.Delegations), dtask.Agent, truncate(dtask.Task, 100))

		// Trace + Bus: agent start.
		if o.Trace != nil {
			o.Trace.Record(debug.StepAgentStart, dtask.Agent, "", dtask.Task, "", nil)
		}
		o.emit(eventbus.AgentEvent(eventbus.TopicAgentStart, dtask.Agent, dtask.Task))

		startTime := time.Now()

		// ── Escalation: wrap agent.Run() with escalation chain ──
		var result Result
		agentAutonomy := agent.Config.Autonomy
		if agentAutonomy == 0 && o.Config.Autonomy > 0 {
			agentAutonomy = o.Config.Autonomy
		}

		if agentAutonomy > AutonomySupervised {
			chain := NewEscalationChain(agent, o.Doctor, o.Bus)
			escResult := chain.RunWithEscalation(ctx, dtask)
			result = escResult.Result

			if escResult.FinalStep > StepRetry {
				debug.Info("orchestrator", "Escalation resolved at step %q after %d retries",
					escResult.FinalStep, escResult.TotalRetries)
			}
		} else {
			result = agent.Run(ctx, dtask)
		}

		if result.Success {
			debug.Info("orchestrator", "<- %q succeeded: %s", dtask.Agent, truncate(result.Output, 100))
			taskList.Complete(taskID, truncate(result.Output, 200))

			// Auto-extract code files from coder output.
			// Tiny models can't call file_write reliably, so we parse their
			// raw output for code blocks and write the files ourselves.
			if dtask.Agent == "coder" && len(result.Output) > 100 {
				codeFiles := extractCodeFiles(result.Output)
				if len(codeFiles) > 0 {
					projectDir := "/app/data/projects/latest"
					if o.Memory != nil && o.Memory.DataDir != "" {
						projectDir = o.Memory.DataDir + "/projects/latest"
					}
					if err := writeCodeFiles(projectDir, codeFiles); err != nil {
						debug.Warn("orchestrator", "Auto-extract: failed to write files: %v", err)
					} else {
						debug.Info("orchestrator", "Auto-extract: wrote %d files to %s", len(codeFiles), projectDir)
						// Append file list to the output so the user sees what was created.
						var fileList strings.Builder
						fileList.WriteString("\n\nFiles created:\n")
						for name := range codeFiles {
							fileList.WriteString("  - " + name + "\n")
						}
						result.Output += fileList.String()
					}
				}
			}

			if dtID, ok := dashboardTaskIDs[taskID]; ok && o.DashboardTasks != nil {
				o.DashboardTasks.UpdateTaskStatus(dtID, tasks.StatusDone)
				o.DashboardTasks.AddComment(dtID, dtask.Agent, truncate(result.Output, 500))
			}
		} else {
			debug.Error("orchestrator", "<- %q failed: %s", dtask.Agent, result.Error)
			o.emitError("agent_task_failed", fmt.Sprintf("agent %q failed task: %s", dtask.Agent, result.Error))
			taskList.Fail(taskID, result.Error)

			if dtID, ok := dashboardTaskIDs[taskID]; ok && o.DashboardTasks != nil {
				o.DashboardTasks.UpdateTaskStatus(dtID, tasks.StatusTodo)
				o.DashboardTasks.AddComment(dtID, dtask.Agent, "FAILED: "+result.Error)
			}
		}
		results = append(results, result)

		// Update pipeline output for the next delegation in the chain.
		if result.Success {
			pipelineOutput = result.Output
		} else {
			pipelineOutput = ""
		}

		// Trace + Bus: agent complete.
		if o.Trace != nil {
			output := result.Output
			if !result.Success {
				output = result.Error
			}
			o.Trace.RecordTimed(debug.StepAgentComplete, dtask.Agent, "", dtask.Task, startTime, truncate(output, 500))
		}
		o.emit(eventbus.Event{
			Topic: eventbus.TopicAgentComplete,
			Agent: dtask.Agent,
			Data: map[string]interface{}{
				"success":     result.Success,
				"duration_ms": time.Since(startTime).Milliseconds(),
			},
			Message: truncate(result.Output, 200),
		})

		// Auto-reflect: record what the agent learned from this task.
		if o.Config.Memory.AutoReflect && o.Memory != nil {
			outcome := "success"
			lessons := truncate(result.Output, 200)
			if !result.Success {
				outcome = "failure"
				lessons = result.Error
			}
			if err := o.Memory.Reflections.Add(dtask.Agent, dtask.Task, outcome, lessons); err != nil {
				debug.Warn("orchestrator", "Auto-reflect save failed: %v", err)
			} else {
				debug.Debug("orchestrator", "Auto-reflected for %q: %s -> %s", dtask.Agent, outcome, truncate(lessons, 80))
			}
		}
	}

	// Archive the completed task list.
	o.Tasks.Archive(taskList.ID)

	// Step 5: Synthesize results.
	debug.Separator("orchestrator")
	var finalResponse string
	successResults := make([]Result, 0)
	for _, r := range results {
		if r.Success {
			successResults = append(successResults, r)
		}
	}

	if len(successResults) == 1 {
		finalResponse = successResults[0].Output
		debug.Info("orchestrator", "Single-agent result, skipping synthesis (saving tokens)")
	} else if len(successResults) == 0 {
		var errSummary strings.Builder
		for _, r := range results {
			errSummary.WriteString(fmt.Sprintf("[%s] %s\n", r.Agent, r.Error))
		}
		finalResponse = "I wasn't able to complete that task. Here's what happened:\n" + errSummary.String()
		debug.Warn("orchestrator", "All agents failed, returning error summary")
	} else if pipelineOutput != "" {
		// Pipeline results: use the accumulated output from chaining.
		// This avoids an extra LLM synthesis call (which conflicts with ForceJSON)
		// and is faster since the pipeline output already contains everything.
		finalResponse = pipelineOutput
		debug.Info("orchestrator", "Pipeline result, using accumulated output (%d chars)", len(pipelineOutput))
	} else {
		o.status("iTaKAgent Synthesizing...")
		debug.Info("orchestrator", "Synthesizing %d result(s)...", len(results))
		var err error
		finalResponse, err = o.synthesize(ctx, userMessage, results)
		if err != nil {
			return "", err
		}
	}

	// Trace: final response.
	if o.Trace != nil {
		o.Trace.Record(debug.StepFinalResponse, "orchestrator", "", userMessage, truncate(finalResponse, 500), nil)
	}

	// Log the synthesized response.
	o.logAssistantResponse(finalResponse)
	return finalResponse, nil
}

// logAssistantResponse saves the assistant's response to session + archive.
func (o *Orchestrator) logAssistantResponse(response string) {
	if o.Memory == nil {
		return
	}
	o.Memory.Session.Add(llm.Message{Role: llm.RoleAssistant, Content: response})
	o.Memory.Archive.LogMessage(memory.LogMessage{
		Role:      string(llm.RoleAssistant),
		Content:   response,
		Timestamp: time.Now(),
	})

	// Auto-track entities in responses.
	if o.Config.Memory.AutoEntities {
		o.Memory.Entities.Track(response, o.Memory.Archive.NextID())
	}
}

// status fires the status callback if set.
func (o *Orchestrator) status(msg string) {
	if o.StatusFunc != nil {
		o.StatusFunc(msg)
	}
}

// buildAgentDescriptions generates the agent list for the system prompt.
func (o *Orchestrator) buildAgentDescriptions() string {
	// Prefer registry-based descriptions (includes capabilities).
	if o.Registry != nil && o.Registry.Count() > 0 {
		return o.Registry.Describe()
	}

	// Fallback to direct agent map (for backward compatibility).
	var sb strings.Builder
	for name, agent := range o.Agents {
		sb.WriteString(fmt.Sprintf("- **%s** (role: %s): %s\n", name, agent.Config.Role, agent.Config.Personality))
		if len(agent.Config.Goals) > 0 {
			sb.WriteString(fmt.Sprintf("  Goals: %s\n", strings.Join(agent.Config.Goals, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  Tools: %s\n", strings.Join(agent.Tools.Names(), ", ")))
	}
	return sb.String()
}

// synthesize combines agent results into a final user-facing response.
func (o *Orchestrator) synthesize(ctx context.Context, userMessage string, results []Result) (string, error) {
	// Build a summary of results for the LLM.
	var sb strings.Builder
	sb.WriteString("ORIGINAL USER REQUEST:\n")
	sb.WriteString(userMessage)
	sb.WriteString("\n\nAGENT RESULTS:\n")

	for _, r := range results {
		if r.Success {
			sb.WriteString(fmt.Sprintf("[%s] SUCCESS:\n%s\n\n", r.Agent, r.Output))
		} else {
			sb.WriteString(fmt.Sprintf("[%s] FAILED: %s\n\n", r.Agent, r.Error))
		}
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: synthesisSystemPrompt},
		{Role: llm.RoleUser, Content: sb.String()},
	}

	resp, err := o.LLMClient.Chat(ctx, messages, nil)
	if err != nil {
		debug.Warn("orchestrator", "Synthesis LLM call failed, falling back to raw results: %v", err)
		return sb.String(), nil
	}

	debug.Debug("orchestrator", "Synthesis complete, response length: %d chars", len(resp.Content))
	return resp.Content, nil
}

// rawDelegation is a flexible struct for parsing LLM output.
type rawDelegation struct {
	Reasoning      string           `json:"reasoning"`
	Delegations    []rawTaskPayload `json:"delegations"`
	DirectResponse string           `json:"direct_response,omitempty"`
}

type rawTaskPayload struct {
	Agent   string          `json:"agent"`
	Task    string          `json:"task"`
	Context json.RawMessage `json:"context,omitempty"`
}

// parseDelegation extracts the delegation JSON from the LLM response.
func parseDelegation(raw string) (*Delegation, string, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		// No JSON found  -  the LLM responded conversationally.
		// Treat the entire response as a direct answer (common with 30B models).
		cleaned := strings.TrimSpace(raw)
		if cleaned != "" {
			debug.Debug("orchestrator", "No JSON in response  -  treating as direct reply")
			return nil, cleaned, nil
		}
		return nil, "", fmt.Errorf("empty response from LLM")
	}

	var d rawDelegation
	if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
		return nil, "", fmt.Errorf("invalid delegation JSON: %w\nraw: %s", err, jsonStr)
	}

	if d.DirectResponse != "" {
		return nil, d.DirectResponse, nil
	}

	payloads := make([]TaskPayload, len(d.Delegations))
	for i, rp := range d.Delegations {
		payloads[i] = TaskPayload{
			Agent:   rp.Agent,
			Task:    rp.Task,
			Context: parseFlexibleContext(rp.Context),
		}
	}

	return &Delegation{
		Reasoning:   d.Reasoning,
		Delegations: payloads,
	}, "", nil
}

// extractJSON finds the first valid JSON object in a string.
func extractJSON(s string) string {
	// Strip Qwen3-style thinking tags first.
	s = stripThinkingTags(s)
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "{") {
		return s
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// parseFlexibleContext converts a json.RawMessage to a string.
func parseFlexibleContext(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if len(obj) == 0 {
			return ""
		}
		b, _ := json.Marshal(obj)
		return string(b)
	}

	return string(raw)
}

// isDoctorActive checks if the Doctor is currently in a healing cycle.
// Uses a simple heuristic: subscribe to doctor events and check recent state.
func (o *Orchestrator) isDoctorActive() bool {
	if o.Doctor == nil {
		return false
	}
	o.Doctor.mu.RLock()
	defer o.Doctor.mu.RUnlock()
	return o.Doctor.healing
}

// waitForDoctorClear blocks until the Doctor emits a "clear" event or timeout.
// Returns true if the Doctor cleared, false on timeout.
func (o *Orchestrator) waitForDoctorClear(ctx context.Context, timeout time.Duration) bool {
	if o.Bus == nil {
		return true
	}

	subID, ch := o.Bus.Subscribe(1, TopicDoctorClear)
	defer o.Bus.Unsubscribe(subID)

	select {
	case <-ch:
		debug.Info("orchestrator", "Doctor cleared -- resuming delegation")
		return true
	case <-ctx.Done():
		return false
	case <-time.After(timeout):
		return false
	}
}

// compressMessages creates a lightweight text recap of old messages.
// This avoids an extra LLM call (expensive for tiny models) and instead
// concatenates key points from each message into a brief summary.
// Max recap length is capped to prevent context bloat.
func compressMessages(messages []llm.Message) string {
	const maxRecapLen = 500 // keep the recap under 500 chars
	const maxPerMsg = 80    // truncate each message to 80 chars

	var sb strings.Builder
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}

		// Truncate long messages.
		if len(content) > maxPerMsg {
			content = content[:maxPerMsg] + "..."
		}

		// Compact role label.
		role := string(msg.Role)
		switch msg.Role {
		case llm.RoleUser:
			role = "User"
		case llm.RoleAssistant:
			role = "Agent"
		case llm.RoleSystem:
			continue // skip system messages from recap
		case llm.RoleTool:
			role = "Tool"
		}

		line := fmt.Sprintf("- %s: %s\n", role, content)

		// Stop if we'd exceed the cap.
		if sb.Len()+len(line) > maxRecapLen {
			sb.WriteString("- (older messages omitted)\n")
			break
		}
		sb.WriteString(line)
	}

	return sb.String()
}
