package builtins

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellDeniedCommands(t *testing.T) {
	shell := &ShellTool{
		DeniedCommands: []string{"format", "del /s /q"},
	}

	tests := []struct {
		cmd      string
		blocked  bool
	}{
		{"echo hello", false},
		{"dir", false},
		{"format c:", true},       // user-configured
		{"DEL /S /Q C:\\", true},  // user-configured, case insensitive
	}

	for _, tc := range tests {
		matched := shell.checkDenied(tc.cmd)
		if tc.blocked && matched == "" {
			t.Errorf("expected %q to be blocked, but it passed", tc.cmd)
		}
		if !tc.blocked && matched != "" {
			t.Errorf("expected %q to pass, but blocked by %q", tc.cmd, matched)
		}
	}
}

func TestShellDefaultDeniedPerOS(t *testing.T) {
	shell := &ShellTool{}

	if runtime.GOOS == "windows" {
		// Windows defaults should block these.
		if shell.checkDenied("diskpart") == "" {
			t.Error("diskpart should be blocked on Windows")
		}
		if shell.checkDenied("shutdown /s") == "" {
			t.Error("shutdown /s should be blocked on Windows")
		}
	} else {
		// Unix defaults should block these.
		if shell.checkDenied("rm -rf /") == "" {
			t.Error("rm -rf / should be blocked on Unix")
		}
		if shell.checkDenied("shutdown") == "" {
			t.Error("shutdown should be blocked on Unix")
		}
	}
}

func TestShellExecuteEcho(t *testing.T) {
	shell := &ShellTool{}
	ctx := context.Background()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	} else {
		cmd = "echo hello"
	}

	result, err := shell.Execute(ctx, map[string]interface{}{
		"command": cmd,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected output containing 'hello', got %q", result)
	}
}

func TestShellMissingCommand(t *testing.T) {
	shell := &ShellTool{}
	ctx := context.Background()

	_, err := shell.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestShellBlockedCommand(t *testing.T) {
	shell := &ShellTool{DeniedCommands: []string{"banned_cmd"}}
	ctx := context.Background()

	result, err := shell.Execute(ctx, map[string]interface{}{
		"command": "banned_cmd --force",
	})
	if err != nil {
		t.Fatalf("Execute should not return error for blocked cmd, got: %v", err)
	}
	if !strings.Contains(result, "BLOCKED") {
		t.Errorf("expected BLOCKED message, got: %s", result)
	}
}

func TestFileReadTool(t *testing.T) {
	// Create a temp file.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	fr := &FileReadTool{}
	ctx := context.Background()

	result, err := fr.Execute(ctx, map[string]interface{}{"path": path})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestFileReadMissing(t *testing.T) {
	fr := &FileReadTool{}
	ctx := context.Background()

	result, err := fr.Execute(ctx, map[string]interface{}{"path": "/nonexistent/file.txt"})
	if err != nil {
		t.Fatalf("should not error on missing file, got: %v", err)
	}
	if !strings.Contains(result, "not found") && !strings.Contains(result, "does not exist") {
		t.Errorf("expected 'file not found' message, got: %s", result)
	}
}

func TestFileWriteTool(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "subdir", "output.txt")

	fw := &FileWriteTool{}
	ctx := context.Background()

	result, err := fw.Execute(ctx, map[string]interface{}{
		"path":    path,
		"content": "test content",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !strings.Contains(result, "Wrote") {
		t.Errorf("expected 'Wrote' message, got: %s", result)
	}

	// Verify the file was created.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file was not created: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("file content mismatch: got %q", string(data))
	}
}

func TestFileWriteProtectedPath(t *testing.T) {
	tmp := t.TempDir()
	protectedDir := filepath.Join(tmp, "protected")
	os.MkdirAll(protectedDir, 0755)

	fw := &FileWriteTool{ProtectedPaths: []string{protectedDir}}
	ctx := context.Background()

	result, err := fw.Execute(ctx, map[string]interface{}{
		"path":    filepath.Join(protectedDir, "hack.txt"),
		"content": "malicious",
	})
	if err != nil {
		t.Fatalf("should not error, got: %v", err)
	}
	if !strings.Contains(result, "BLOCKED") {
		t.Errorf("expected BLOCKED message, got: %s", result)
	}

	// Verify the file was NOT created.
	if _, err := os.Stat(filepath.Join(protectedDir, "hack.txt")); err == nil {
		t.Error("file should NOT have been created in protected path")
	}
}

func TestDirListTool(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(tmp, "subdir"), 0755)

	dl := &DirListTool{}
	ctx := context.Background()

	result, err := dl.Execute(ctx, map[string]interface{}{"path": tmp})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("expected a.txt in output, got: %s", result)
	}
	if !strings.Contains(result, "subdir") {
		t.Errorf("expected subdir in output, got: %s", result)
	}
}
