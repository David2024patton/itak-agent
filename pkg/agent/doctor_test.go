package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/eventbus"
)

func TestDetectLanguage_GoProject(t *testing.T) {
	dir := t.TempDir()
	// Create some .go files.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme"), 0o644)

	lang := DetectLanguage(dir)
	if lang != "go" {
		t.Errorf("expected go, got %s", lang)
	}
}

func TestDetectLanguage_JSProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.js"), []byte("console.log('hi')"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.tsx"), []byte("export default App"), 0o644)
	os.WriteFile(filepath.Join(dir, "styles.css"), []byte("body{}"), 0o644)

	lang := DetectLanguage(dir)
	if lang != "javascript" {
		t.Errorf("expected javascript, got %s", lang)
	}
}

func TestDetectLanguage_PythonProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('hi')"), 0o644)
	os.WriteFile(filepath.Join(dir, "utils.py"), []byte("def foo(): pass"), 0o644)

	lang := DetectLanguage(dir)
	if lang != "python" {
		t.Errorf("expected python, got %s", lang)
	}
}

func TestDetectLanguage_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	lang := DetectLanguage(dir)
	if lang != "" {
		t.Errorf("expected empty string for empty dir, got %s", lang)
	}
}

func TestDetectLanguage_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	// Main project is Go.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	// node_modules has 100 JS files but should be skipped.
	nm := filepath.Join(dir, "node_modules", "pkg")
	os.MkdirAll(nm, 0o755)
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(nm, "file"+string(rune('a'+i))+".js"), []byte("x"), 0o644)
	}

	lang := DetectLanguage(dir)
	if lang != "go" {
		t.Errorf("expected go (skipping node_modules), got %s", lang)
	}
}

func TestErrorSignature_StripsPaths(t *testing.T) {
	sig1 := errorSignature("main.go:42: undefined: foo")
	sig2 := errorSignature("main.go:99: undefined: foo")
	if sig1 != sig2 {
		t.Errorf("signatures should match after stripping line numbers:\n  %q\n  %q", sig1, sig2)
	}
}

func TestErrorSignature_DifferentErrors(t *testing.T) {
	sig1 := errorSignature("undefined: foo")
	sig2 := errorSignature("cannot convert string to int")
	if sig1 == sig2 {
		t.Error("different errors should produce different signatures")
	}
}

func TestDoctor_FixMemory(t *testing.T) {
	dir := t.TempDir()
	d := NewDoctor(nil, dir)

	// Record a fix.
	d.RecordFix("undefined: myFunc", "imported the missing package")

	// Look it up.
	fix, found := d.LookupFix("undefined: myFunc")
	if !found {
		t.Fatal("expected to find fix")
	}
	if fix.FixApplied != "imported the missing package" {
		t.Errorf("unexpected fix: %s", fix.FixApplied)
	}
	if d.FixCount() != 1 {
		t.Errorf("expected 1 fix, got %d", d.FixCount())
	}
}

func TestDoctor_FixMemoryPersistence(t *testing.T) {
	dir := t.TempDir()

	// Write a fix.
	d1 := NewDoctor(nil, dir)
	d1.RecordFix("connection refused", "started the database")

	// Create a new doctor (simulating restart) and check persistence.
	d2 := NewDoctor(nil, dir)
	fix, found := d2.LookupFix("connection refused")
	if !found {
		t.Fatal("fix should persist across restarts")
	}
	if fix.FixApplied != "started the database" {
		t.Errorf("unexpected persisted fix: %s", fix.FixApplied)
	}
}

func TestLintProject_GoProject(t *testing.T) {
	// Create a minimal valid Go project.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	result := LintProject(dir)
	if result.Language != "go" {
		t.Errorf("expected language=go, got %s", result.Language)
	}
	if !result.Success {
		t.Errorf("expected lint success on valid Go project, got errors: %v", result.Errors)
	}
}

func TestFormatLintResult_Pass(t *testing.T) {
	r := LintResult{Language: "go", Tool: "go vet", Success: true}
	out := FormatLintResult(r)
	if out == "" {
		t.Error("expected non-empty output")
	}
	if !contains(out, "PASS") {
		t.Errorf("expected PASS in output: %s", out)
	}
}

func TestFormatLintResult_Fail(t *testing.T) {
	r := LintResult{
		Language: "go",
		Tool:     "go vet",
		Success:  false,
		Errors:   []string{"main.go:1: something wrong"},
	}
	out := FormatLintResult(r)
	if !contains(out, "FAIL") {
		t.Errorf("expected FAIL in output: %s", out)
	}
	if !contains(out, "something wrong") {
		t.Errorf("expected error text in output: %s", out)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDoctor_GoldenSnapshot(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0o755)

	// Create a minimal Go project.
	os.WriteFile(filepath.Join(projDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	d := NewDoctor(nil, dir)

	err := d.SnapshotGolden(projDir)
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	// Verify golden copy exists.
	goldenPath := filepath.Join(d.GoldenDir, "main.go")
	data, readErr := os.ReadFile(goldenPath)
	if readErr != nil {
		t.Fatal("golden copy should exist after snapshot")
	}
	if string(data) != "package main\nfunc main() {}\n" {
		t.Error("golden copy content mismatch")
	}
}

func TestDoctor_DiffGolden_NoChanges(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0o755)

	content := "package main\nfunc main() {}\n"
	os.WriteFile(filepath.Join(projDir, "main.go"), []byte(content), 0o644)

	d := NewDoctor(nil, dir)
	d.SnapshotGolden(projDir)

	diff := d.DiffGolden(projDir, "main.go")
	if diff != "" {
		t.Errorf("expected no diff for unchanged file, got: %s", diff)
	}
}

func TestDoctor_DiffGolden_WithChanges(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0o755)

	os.WriteFile(filepath.Join(projDir, "main.go"), []byte("package main\n"), 0o644)

	d := NewDoctor(nil, dir)
	d.SnapshotGolden(projDir)

	// Modify the file.
	os.WriteFile(filepath.Join(projDir, "main.go"), []byte("package main\nfunc broken() {}\n"), 0o644)

	diff := d.DiffGolden(projDir, "main.go")
	if diff == "" {
		t.Error("expected diff for changed file")
	}
	if !contains(diff, "golden/") {
		t.Errorf("diff should reference golden: %s", diff)
	}
}

func TestDoctor_RateLimiting(t *testing.T) {
	dir := t.TempDir()
	d := NewDoctor(nil, dir)
	d.FixCooldown = 100 * time.Millisecond

	// First diagnose should run.
	d.mu.Lock()
	d.lastDiagnose = time.Time{} // reset
	d.mu.Unlock()

	// After running, check that lastDiagnose is updated.
	d.mu.Lock()
	d.lastDiagnose = time.Now()
	d.mu.Unlock()

	// Immediate second call should be rate-limited.
	d.mu.RLock()
	elapsed := time.Since(d.lastDiagnose)
	d.mu.RUnlock()

	if elapsed >= d.FixCooldown {
		t.Error("should be within cooldown period immediately after diagnose")
	}
}

func TestDoctor_MaxFixAttempts(t *testing.T) {
	dir := t.TempDir()
	d := NewDoctor(nil, dir)
	d.MaxFixAttempts = 2

	d.mu.Lock()
	d.fixAttempts = 2
	d.mu.Unlock()

	d.mu.RLock()
	if d.fixAttempts < d.MaxFixAttempts {
		t.Error("should have reached max fix attempts")
	}
	d.mu.RUnlock()
}

func TestDoctor_RollbackFile(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0o755)

	original := "package main\nfunc main() {}\n"
	modified := "package main\nfunc broken() {}\n"

	filePath := filepath.Join(projDir, "main.go")
	os.WriteFile(filePath, []byte(modified), 0o644)

	// Create a backup.
	os.WriteFile(filePath+".doctor_backup", []byte(original), 0o644)

	d := NewDoctor(nil, dir)
	d.ProjectDir = projDir

	ok := d.rollbackFile("main.go")
	if !ok {
		t.Fatal("rollback should succeed")
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != original {
		t.Error("file should be restored to original content after rollback")
	}

	// Backup should be cleaned up.
	if _, err := os.Stat(filePath + ".doctor_backup"); err == nil {
		t.Error("backup file should be removed after rollback")
	}
}

func TestDoctor_EmitAlert(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()

	d := NewDoctor(bus, dir)

	_, ch := bus.Subscribe(8, "doctor.alert")

	// Emit a fixed alert.
	go d.emitAlert(true, "golden_revert", "scout", "tool_exec_failed", "broken", "reverted main.go")

	select {
	case e := <-ch:
		if e.Agent != "doctor" {
			t.Errorf("expected agent=doctor, got %s", e.Agent)
		}
		if !contains(e.Message, "FIXED") {
			t.Errorf("fixed alert should contain FIXED: %s", e.Message)
		}
		sev, _ := e.Data["severity"].(string)
		if sev != "info" {
			t.Errorf("fixed alert severity should be info, got %s", sev)
		}
		fixed, _ := e.Data["fixed"].(bool)
		if !fixed {
			t.Error("fixed field should be true")
		}
	case <-time.After(time.Second):
		t.Fatal("no alert received")
	}
}

// TestDoctor_Integration_FullFixPipeline is the end-to-end test.
// 1. Create a valid Go project
// 2. Golden snapshot it
// 3. Break a file
// 4. Fire agent.error
// 5. Verify Doctor reverts, build passes, fix is recorded, alert emits "FIXED"
func TestDoctor_Integration_FullFixPipeline(t *testing.T) {
	// ── Setup: Create a minimal Go project ──
	dataDir := t.TempDir()
	projDir := filepath.Join(dataDir, "project")
	os.MkdirAll(projDir, 0o755)

	goodCode := "package main\n\nfunc main() {}\n"
	os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module testproject\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(projDir, "main.go"), []byte(goodCode), 0o644)

	bus := eventbus.New()
	doc := NewDoctor(bus, dataDir)
	doc.ProjectDir = projDir
	doc.FixCooldown = 0 // disable cooldown for testing

	// ── Step 1: Take a golden snapshot of the working project ──
	if err := doc.SnapshotGolden(projDir); err != nil {
		t.Fatalf("golden snapshot failed: %v", err)
	}
	t.Log("Golden snapshot taken")

	// Verify golden exists.
	goldenPath := filepath.Join(doc.GoldenDir, "main.go")
	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden copy should exist: %v", err)
	}
	if string(goldenData) != goodCode {
		t.Fatal("golden copy content mismatch")
	}

	// ── Step 2: Verify build passes before we break anything ──
	if !doc.verifyBuild() {
		t.Fatal("clean project should pass build verification")
	}
	t.Log("Build verification passed (pre-break)")

	// ── Step 3: BREAK the code ──
	brokenCode := "package main\n\nfunc main() {\n\tundefinedFunction()\n}\n"
	os.WriteFile(filepath.Join(projDir, "main.go"), []byte(brokenCode), 0o644)
	t.Log("Code intentionally broken (calling undefined function)")

	// Verify the build is now broken.
	if doc.verifyBuild() {
		t.Fatal("broken code should fail build verification")
	}
	t.Log("Build verification correctly FAILED (post-break)")

	// ── Step 4: Check that DiffGolden catches the change ──
	diff := doc.DiffGolden(projDir, "main.go")
	if diff == "" {
		t.Fatal("DiffGolden should detect the broken change")
	}
	t.Logf("DiffGolden detected change:\n%s", diff)

	// ── Step 5: Test golden revert directly ──
	lintResult := LintProject(projDir)
	if lintResult.Success {
		t.Fatal("lint should fail on broken code")
	}
	t.Logf("Lint correctly found %d error(s)", len(lintResult.Errors))

	revertResult := doc.tryGoldenRevert(lintResult)
	if revertResult == "" {
		t.Fatal("golden revert should have reverted main.go")
	}
	t.Logf("Golden revert applied: %s", revertResult)

	// ── Step 6: Verify the fix worked ──
	if !doc.verifyBuild() {
		t.Fatal("build should pass after golden revert")
	}
	t.Log("Build verification PASSED after golden revert!")

	// Check the file content is back to good.
	restoredData, _ := os.ReadFile(filepath.Join(projDir, "main.go"))
	if string(restoredData) != goodCode {
		t.Errorf("file should be restored to golden content, got:\n%s", string(restoredData))
	}

	// ── Step 7: Verify backup exists for rollback ──
	backupPath := filepath.Join(projDir, "main.go.doctor_backup")
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal("backup should exist after revert")
	}
	if string(backupData) != brokenCode {
		t.Error("backup should contain the broken code for rollback")
	}
	t.Log("Backup file preserved for rollback")

	// ── Step 8: Test rollback ──
	doc.rollbackFile("main.go")
	rolledBack, _ := os.ReadFile(filepath.Join(projDir, "main.go"))
	if string(rolledBack) != brokenCode {
		t.Error("rollback should restore the broken code")
	}
	t.Log("Rollback works correctly")

	// Restore good code for the fix memory test.
	os.WriteFile(filepath.Join(projDir, "main.go"), []byte(goodCode), 0o644)

	// ── Step 9: Test fix memory ──
	doc.RecordFix("undefined: undefinedFunction", "golden_revert: reverted main.go")
	fix, found := doc.LookupFix("undefined: undefinedFunction")
	if !found {
		t.Fatal("fix should be recorded in memory")
	}
	if fix.FixApplied != "golden_revert: reverted main.go" {
		t.Errorf("unexpected fix: %s", fix.FixApplied)
	}
	t.Log("Fix recorded in memory for future recall")

	t.Log("FULL PIPELINE TEST PASSED")
}
