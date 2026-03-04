package task

import (
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewList(t *testing.T) {
	l := NewList("test-1", "Build a website")

	if l.ID != "test-1" {
		t.Errorf("ID = %q, want 'test-1'", l.ID)
	}
	if l.UserRequest != "Build a website" {
		t.Errorf("UserRequest = %q", l.UserRequest)
	}
	if len(l.Items) != 0 {
		t.Errorf("Items = %d, want 0", len(l.Items))
	}
}

func TestAddItem(t *testing.T) {
	l := NewList("t1", "test request")

	l.AddItem("step-1", "Create project", "coder")
	l.AddItem("step-2", "Write tests", "coder")

	if len(l.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(l.Items))
	}
	if l.Items[0].Agent != "coder" {
		t.Errorf("Agent = %q, want 'coder'", l.Items[0].Agent)
	}
	if l.Items[0].Status != StatusPending {
		t.Errorf("Status = %q, want 'pending'", l.Items[0].Status)
	}
}

func TestStartComplete(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "do thing", "worker")

	l.Start("s1")
	if l.Items[0].Status != StatusRunning {
		t.Errorf("Status after Start = %q, want 'running'", l.Items[0].Status)
	}
	if l.Items[0].StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}

	l.Complete("s1", "done!")
	if l.Items[0].Status != StatusDone {
		t.Errorf("Status after Complete = %q, want 'done'", l.Items[0].Status)
	}
	if l.Items[0].Output != "done!" {
		t.Errorf("Output = %q", l.Items[0].Output)
	}
	if l.Items[0].DoneAt.IsZero() {
		t.Error("DoneAt should be set")
	}
}

func TestFail(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "do thing", "worker")

	l.Start("s1")
	l.Fail("s1", "something broke")

	if l.Items[0].Status != StatusFailed {
		t.Errorf("Status = %q, want 'failed'", l.Items[0].Status)
	}
	if l.Items[0].Error != "something broke" {
		t.Errorf("Error = %q", l.Items[0].Error)
	}
}

func TestSkip(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "optional step", "worker")

	l.Skip("s1")
	if l.Items[0].Status != StatusSkipped {
		t.Errorf("Status = %q, want 'skipped'", l.Items[0].Status)
	}
}

func TestProgress(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "step 1", "a")
	l.AddItem("s2", "step 2", "b")
	l.AddItem("s3", "step 3", "c")

	done, total := l.Progress()
	if done != 0 || total != 3 {
		t.Errorf("Progress = %d/%d, want 0/3", done, total)
	}

	l.Complete("s1", "ok")
	l.Skip("s2")

	done, total = l.Progress()
	if done != 2 || total != 3 {
		t.Errorf("Progress = %d/%d, want 2/3", done, total)
	}
}

func TestIsComplete(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "step 1", "a")
	l.AddItem("s2", "step 2", "b")

	if l.IsComplete() {
		t.Error("Should not be complete")
	}

	l.Complete("s1", "ok")
	l.Fail("s2", "oops")

	if !l.IsComplete() {
		t.Error("Should be complete (done + failed = all)")
	}
}

func TestSubTasks(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "main task", "coder")

	sub, err := l.AddSubTask("s1", "s1.1", "sub step", "coder")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != StatusPending {
		t.Error("Sub-task should start pending")
	}

	// Sub-task shows in progress count.
	done, total := l.Progress()
	if total != 2 {
		t.Errorf("Total = %d, want 2 (1 parent + 1 sub)", total)
	}

	// Complete sub-task.
	l.Complete("s1.1", "sub done")
	done, _ = l.Progress()
	if done != 1 {
		t.Errorf("Done = %d, want 1", done)
	}
}

func TestSubTask_NotFound(t *testing.T) {
	l := NewList("t1", "test")
	_, err := l.AddSubTask("nonexistent", "s1.1", "sub", "a")
	if err == nil {
		t.Error("Should error for nonexistent parent")
	}
}

func TestOnUpdate(t *testing.T) {
	var count int64
	l := NewList("t1", "test")
	l.OnUpdate(func(_ *List) {
		atomic.AddInt64(&count, 1)
	})

	l.AddItem("s1", "step", "a")  // +1
	l.Start("s1")                  // +1
	l.Complete("s1", "ok")         // +1

	if atomic.LoadInt64(&count) != 3 {
		t.Errorf("Update count = %d, want 3", count)
	}
}

func TestSummary(t *testing.T) {
	l := NewList("t1", "Build app")
	l.AddItem("s1", "Create project", "coder")
	l.AddItem("s2", "Write tests", "tester")

	l.Complete("s1", "ok")

	s := l.Summary()
	if !strings.Contains(s, "Build app") {
		t.Error("Summary should contain request")
	}
	if !strings.Contains(s, "[x]") {
		t.Error("Summary should contain done icon")
	}
	if !strings.Contains(s, "[ ]") {
		t.Error("Summary should contain pending icon")
	}
}

func TestPendingItems(t *testing.T) {
	l := NewList("t1", "test")
	l.AddItem("s1", "done step", "a")
	l.AddItem("s2", "pending step", "b")
	l.AddItem("s3", "pending step 2", "c")

	l.Complete("s1", "ok")

	pending := l.PendingItems()
	if len(pending) != 2 {
		t.Errorf("Pending = %d, want 2", len(pending))
	}
}

func TestNotFound(t *testing.T) {
	l := NewList("t1", "test")

	if err := l.Start("nonexistent"); err == nil {
		t.Error("Start should error for nonexistent item")
	}
	if err := l.Complete("nonexistent", ""); err == nil {
		t.Error("Complete should error for nonexistent item")
	}
	if err := l.Fail("nonexistent", ""); err == nil {
		t.Error("Fail should error for nonexistent item")
	}
	if err := l.Skip("nonexistent"); err == nil {
		t.Error("Skip should error for nonexistent item")
	}
}

// --- Tracker Tests ---

func TestTracker_Create(t *testing.T) {
	tr := NewTracker(10)

	l := tr.Create("Build a website")
	if l == nil {
		t.Fatal("Create returned nil")
	}
	if l.UserRequest != "Build a website" {
		t.Error("UserRequest mismatch")
	}
}

func TestTracker_Active(t *testing.T) {
	tr := NewTracker(10)

	tr.Create("task 1")
	tr.Create("task 2")

	active := tr.Active()
	if len(active) != 2 {
		t.Errorf("Active = %d, want 2", len(active))
	}
}

func TestTracker_Archive(t *testing.T) {
	tr := NewTracker(10)

	l := tr.Create("task 1")
	id := l.ID

	err := tr.Archive(id)
	if err != nil {
		t.Fatal(err)
	}

	active := tr.Active()
	if len(active) != 0 {
		t.Errorf("Active after archive = %d, want 0", len(active))
	}

	hist := tr.History(10)
	if len(hist) != 1 {
		t.Errorf("History = %d, want 1", len(hist))
	}
}

func TestTracker_ArchiveNotFound(t *testing.T) {
	tr := NewTracker(10)
	err := tr.Archive("nonexistent")
	if err == nil {
		t.Error("Archive should error for nonexistent list")
	}
}

func TestTracker_Stats(t *testing.T) {
	tr := NewTracker(10)

	l := tr.Create("task 1")
	l.AddItem("s1", "step 1", "a")
	l.AddItem("s2", "step 2", "b")
	l.Complete("s1", "ok")

	stats := tr.Stats()
	if stats.ActiveLists != 1 {
		t.Errorf("ActiveLists = %d, want 1", stats.ActiveLists)
	}
	if stats.TotalItems != 2 {
		t.Errorf("TotalItems = %d, want 2", stats.TotalItems)
	}
	if stats.CompletedItems != 1 {
		t.Errorf("CompletedItems = %d, want 1", stats.CompletedItems)
	}
}

func TestTracker_HistoryTrim(t *testing.T) {
	tr := NewTracker(3)

	for i := 0; i < 5; i++ {
		l := tr.Create("task")
		tr.Archive(l.ID)
	}

	hist := tr.History(10)
	if len(hist) != 3 {
		t.Errorf("History = %d, want 3 (trimmed)", len(hist))
	}
}

func TestTracker_OnUpdate(t *testing.T) {
	var count int64
	tr := NewTracker(10)
	tr.OnUpdate(func(_ *List) {
		atomic.AddInt64(&count, 1)
	})

	l := tr.Create("task")
	l.AddItem("s1", "step", "a")

	if atomic.LoadInt64(&count) != 1 {
		t.Errorf("Update count = %d, want 1", count)
	}
}
