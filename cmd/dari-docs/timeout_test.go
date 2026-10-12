package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestTimeoutMinutesMustBePositive(t *testing.T) {
	for _, minutes := range []int{0, -1, int(^uint(0) >> 1)} {
		t.Run(strconv.Itoa(minutes), func(t *testing.T) {
			cmd := newCheckOptimizeCommand("check")
			cmd.SetArgs([]string{t.TempDir(), "--task", "task", "--timeout-minutes", strconv.Itoa(minutes)})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--timeout-minutes") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	opts := defaultCheckOptimizeOptions("check")
	opts.RepoArg = t.TempDir()
	opts.TaskInputs = []string{"task"}
	if err := opts.prepare(); err != nil {
		t.Fatalf("default timeout rejected: %v", err)
	}
}
