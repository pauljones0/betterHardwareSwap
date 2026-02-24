package processor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// loadFixture loads a JSON file from the test/fixtures directory.
func loadFixture(filename string, v interface{}) error {
	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)
	// We are in internal/processor/, so go up two levels to reach project root.
	path := filepath.Join(basepath, "..", "..", "test", "fixtures", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
