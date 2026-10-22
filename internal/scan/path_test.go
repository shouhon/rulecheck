package scan

import (
	"io/fs"
	"testing"

	"rulecheck/internal/report"
)

func TestUnsupportedLinkIsUnknown(t *testing.T) {
	// Windowsの未対応の再解析ポイントでもReadlinkは不存在を返すことがある。
	err := &fs.PathError{Op: "readlink", Path: "模擬パス", Err: fs.ErrNotExist}
	finding := pathFinding(err)
	if finding.Code != "path_unreadable" || finding.Category != report.Unknown {
		t.Fatalf("解釈できないリンクを不存在と誤認しています: %+v", finding)
	}
}
