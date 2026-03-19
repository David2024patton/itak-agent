// Package tool provides a GitHub API tool powered by google/go-github.
//
// What: Lets agents interact with GitHub - search repos, read issues, create PRs.
// Why:  Agents need to manage code: file bugs, review PRs, check CI status.
//       go-github is Google's official, battle-tested Go client for the GitHub API.
// How:  Implements the Tool interface. The agent calls it with an action + params,
//       and it returns structured results. Requires a GitHub token in config.

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-github/v69/github"
)

// GitHubTool provides agent access to the GitHub API.
type GitHubTool struct {
	client *github.Client
	token  string
}

// NewGitHubTool creates a GitHub tool. If token is empty, uses unauthenticated access (rate-limited).
func NewGitHubTool(token string) *GitHubTool {
	t := &GitHubTool{token: token}

	if token != "" {
		t.client = github.NewClient(nil).WithAuthToken(token)
	} else {
		t.client = github.NewClient(nil)
	}

	return t
}

func (t *GitHubTool) Name() string { return "github" }

func (t *GitHubTool) Description() string {
	return "Interact with GitHub: search repos, list issues, create issues, get file contents, list PRs. " +
		"Actions: search_repos, list_issues, create_issue, get_file, list_prs, get_repo"
}

func (t *GitHubTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The GitHub action to perform",
				"enum":        []string{"search_repos", "list_issues", "create_issue", "get_file", "list_prs", "get_repo"},
			},
			"owner": map[string]interface{}{
				"type":        "string",
				"description": "Repository owner (username or org)",
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository name",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (for search_repos)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path within the repo (for get_file)",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Issue title (for create_issue)",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Issue body (for create_issue)",
			},
			"labels": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated labels (for create_issue)",
			},
		},
		"required": []string{"action"},
	}
}

// Execute runs the requested GitHub action and returns JSON results.
func (t *GitHubTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)

	switch action {
	case "search_repos":
		return t.searchRepos(ctx, args)
	case "list_issues":
		return t.listIssues(ctx, owner, repo)
	case "create_issue":
		return t.createIssue(ctx, owner, repo, args)
	case "get_file":
		return t.getFile(ctx, owner, repo, args)
	case "list_prs":
		return t.listPRs(ctx, owner, repo)
	case "get_repo":
		return t.getRepo(ctx, owner, repo)
	default:
		return "", fmt.Errorf("unknown github action: %s", action)
	}
}

func (t *GitHubTool) searchRepos(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required for search_repos")
	}

	result, _, err := t.client.Search.Repositories(ctx, query, &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 10},
	})
	if err != nil {
		return "", fmt.Errorf("github search: %w", err)
	}

	type repoSummary struct {
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		Stars       int    `json:"stars"`
		Language    string `json:"language"`
		URL         string `json:"url"`
	}

	var repos []repoSummary
	for _, r := range result.Repositories {
		repos = append(repos, repoSummary{
			Name:        r.GetName(),
			FullName:    r.GetFullName(),
			Description: r.GetDescription(),
			Stars:       r.GetStargazersCount(),
			Language:    r.GetLanguage(),
			URL:         r.GetHTMLURL(),
		})
	}

	b, _ := json.MarshalIndent(repos, "", "  ")
	return fmt.Sprintf("Found %d repositories:\n%s", result.GetTotal(), string(b)), nil
}

func (t *GitHubTool) listIssues(ctx context.Context, owner, repo string) (string, error) {
	if owner == "" || repo == "" {
		return "", fmt.Errorf("owner and repo are required")
	}

	issues, _, err := t.client.Issues.ListByRepo(ctx, owner, repo, &github.IssueListByRepoOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 20},
	})
	if err != nil {
		return "", fmt.Errorf("github issues: %w", err)
	}

	type issueSummary struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
		Author string `json:"author"`
		Labels string `json:"labels"`
		URL    string `json:"url"`
	}

	var list []issueSummary
	for _, i := range issues {
		var labels []string
		for _, l := range i.Labels {
			labels = append(labels, l.GetName())
		}
		list = append(list, issueSummary{
			Number: i.GetNumber(),
			Title:  i.GetTitle(),
			State:  i.GetState(),
			Author: i.GetUser().GetLogin(),
			Labels: strings.Join(labels, ", "),
			URL:    i.GetHTMLURL(),
		})
	}

	b, _ := json.MarshalIndent(list, "", "  ")
	return fmt.Sprintf("%d open issues:\n%s", len(list), string(b)), nil
}

func (t *GitHubTool) createIssue(ctx context.Context, owner, repo string, args map[string]interface{}) (string, error) {
	if owner == "" || repo == "" {
		return "", fmt.Errorf("owner and repo are required")
	}
	title, _ := args["title"].(string)
	body, _ := args["body"].(string)
	labelsStr, _ := args["labels"].(string)

	if title == "" {
		return "", fmt.Errorf("title is required for create_issue")
	}

	req := &github.IssueRequest{
		Title: &title,
		Body:  &body,
	}

	if labelsStr != "" {
		var labels []string
		for _, l := range strings.Split(labelsStr, ",") {
			labels = append(labels, strings.TrimSpace(l))
		}
		req.Labels = &labels
	}

	issue, _, err := t.client.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return "", fmt.Errorf("create issue: %w", err)
	}

	return fmt.Sprintf("Created issue #%d: %s\nURL: %s", issue.GetNumber(), issue.GetTitle(), issue.GetHTMLURL()), nil
}

func (t *GitHubTool) getFile(ctx context.Context, owner, repo string, args map[string]interface{}) (string, error) {
	if owner == "" || repo == "" {
		return "", fmt.Errorf("owner and repo are required")
	}
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required for get_file")
	}

	content, _, _, err := t.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}

	decoded, err := content.GetContent()
	if err != nil {
		return "", fmt.Errorf("decode file: %w", err)
	}

	// Truncate very large files.
	if len(decoded) > 10000 {
		decoded = decoded[:10000] + "\n... (truncated, " + fmt.Sprintf("%d", len(decoded)) + " chars total)"
	}

	return fmt.Sprintf("File: %s (%d bytes)\n\n%s", path, content.GetSize(), decoded), nil
}

func (t *GitHubTool) listPRs(ctx context.Context, owner, repo string) (string, error) {
	if owner == "" || repo == "" {
		return "", fmt.Errorf("owner and repo are required")
	}

	prs, _, err := t.client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 20},
	})
	if err != nil {
		return "", fmt.Errorf("github PRs: %w", err)
	}

	type prSummary struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Author string `json:"author"`
		Branch string `json:"branch"`
		URL    string `json:"url"`
	}

	var list []prSummary
	for _, pr := range prs {
		list = append(list, prSummary{
			Number: pr.GetNumber(),
			Title:  pr.GetTitle(),
			Author: pr.GetUser().GetLogin(),
			Branch: pr.GetHead().GetRef(),
			URL:    pr.GetHTMLURL(),
		})
	}

	b, _ := json.MarshalIndent(list, "", "  ")
	return fmt.Sprintf("%d open PRs:\n%s", len(list), string(b)), nil
}

func (t *GitHubTool) getRepo(ctx context.Context, owner, repo string) (string, error) {
	if owner == "" || repo == "" {
		return "", fmt.Errorf("owner and repo are required")
	}

	r, _, err := t.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("get repo: %w", err)
	}

	info := map[string]interface{}{
		"name":         r.GetFullName(),
		"description":  r.GetDescription(),
		"language":     r.GetLanguage(),
		"stars":        r.GetStargazersCount(),
		"forks":        r.GetForksCount(),
		"open_issues":  r.GetOpenIssuesCount(),
		"default_branch": r.GetDefaultBranch(),
		"created":      r.GetCreatedAt().Format("2006-01-02"),
		"updated":      r.GetPushedAt().Format("2006-01-02"),
		"url":          r.GetHTMLURL(),
		"topics":       r.Topics,
	}

	b, _ := json.MarshalIndent(info, "", "  ")
	return string(b), nil
}
