package main

import (
	"bufio"
	"os"
	"strings"
)

func readTasksFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tasks []string
	var cur []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			if len(cur) > 0 {
				tasks = append(tasks, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		tasks = append(tasks, strings.Join(cur, "\n"))
	}
	return tasks, s.Err()
}
