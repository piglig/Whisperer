package memory

import "os"

// osWriteFile 是 memory_test.go 用到的最薄包装。
func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
