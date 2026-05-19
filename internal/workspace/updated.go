package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

func UpdatedRoot(extractDir string) (string, error) {
	candidates := []string{
		filepath.Join(extractDir, "updated-docs", "files"),
		filepath.Join(extractDir, "workspace", "updated-docs", "files"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("updated docs archive missing expected updated-docs/files directory")
}
