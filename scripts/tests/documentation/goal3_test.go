package documentation_test

import (
	"strings"
	"testing"
)

func goalRows(text string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	return rows
}

func TestGoal3TaskDependenciesAndRequiredCoverage(t *testing.T) {
	tasks := map[string][]string{}
	for _, row := range goalRows(readUserGuide(t, "goal-3/tasks.md")) {
		if len(row) != 5 || row[0] == "Task" || !strings.HasPrefix(row[0], "T") {
			continue
		}
		if _, exists := tasks[row[0]]; exists {
			t.Fatalf("duplicate task %s", row[0])
		}
		tasks[row[0]] = nil
		if row[2] != "none" {
			tasks[row[0]] = strings.Split(row[2], ",")
		}
	}
	if len(tasks) == 0 {
		t.Fatal("task dependency table is missing")
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if visiting[id] {
			t.Fatalf("dependency cycle at %s", id)
		}
		if done[id] {
			return
		}
		deps, ok := tasks[id]
		if !ok {
			t.Fatalf("unknown task %s", id)
		}
		visiting[id] = true
		for _, dep := range deps {
			visit(strings.TrimSpace(dep))
		}
		visiting[id] = false
		done[id] = true
	}
	for id := range tasks {
		visit(id)
	}
	var depends func(string, string) bool
	depends = func(id, want string) bool {
		for _, dep := range tasks[id] {
			dep = strings.TrimSpace(dep)
			if dep == want || depends(dep, want) {
				return true
			}
		}
		return false
	}
	for _, edge := range [][2]string{{"T07", "T12"}, {"T12", "T20"}, {"T24", "T39A"}, {"T09", "T09A"}, {"T16", "T09A"}, {"T45", "T41"}} {
		if !depends(edge[0], edge[1]) {
			t.Errorf("%s must depend on %s", edge[0], edge[1])
		}
	}
	found := map[string]bool{}
	for _, row := range goalRows(readUserGuide(t, "goal-3/capability-admission.md")) {
		if len(row) != 11 || row[1] != "yes" {
			continue
		}
		if found[row[0]] {
			t.Fatalf("duplicate capability %s", row[0])
		}
		found[row[0]] = true
		for _, cell := range row[3:9] {
			for _, id := range strings.Split(cell, ",") {
				if _, ok := tasks[strings.TrimSpace(id)]; !ok {
					t.Errorf("%s missing task owner %q", row[0], id)
				}
			}
		}
		if row[9] == "" || row[10] == "" {
			t.Errorf("%s lacks acceptance/evidence", row[0])
		}
	}
	for _, id := range []string{"artwork-recommended", "bookmark-list-all", "bookmark-tags-all", "logical-pagination", "novel-bookmark-mutation", "stamps"} {
		if !found[id] {
			t.Errorf("required capability missing: %s", id)
		}
	}
}

func TestGoal3PublicationAuthority(t *testing.T) {
	for _, name := range []string{"plan.md", "tasks.md", "api-migration-verification.md", "upstream-contract-matrix.md", "cli-migration-matrix.md", "feasibility-report.md"} {
		body := readUserGuide(t, "goal-3/"+name)
		if !strings.Contains(body, "capability-admission.md") {
			t.Errorf("%s must link current capability authority", name)
		}
		for _, bad := range []string{"本矩阵的最终 verdict 表示 `public_ready`", "只记录已 `public_ready` 能力", "单项失败回滚对应 task，不回滚其他已确认修复", "失败能力在本 Goal 内停止、修复或移出 public implementation scope"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s retains conflicting rule: %s", name, bad)
			}
		}
	}
}
