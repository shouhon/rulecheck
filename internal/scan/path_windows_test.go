package scan

import (
	"errors"
	"io/fs"
	"testing"
)

func TestDriveTypeBoundary(t *testing.T) {
	// ネットワーク共有を実際に割り当てず、Windows APIが返す種別との境界を確認する。
	for _, tc := range []struct {
		kind uintptr
		want error
	}{{0, errNonLocal}, {1, fs.ErrNotExist}, {2, nil}, {3, nil}, {4, errNonLocal}, {5, nil}, {6, nil}, {99, errNonLocal}} {
		if err := checkDriveType(tc.kind); !errors.Is(err, tc.want) {
			t.Errorf("ドライブ種別%dの扱いが一致しません: %v", tc.kind, err)
		}
	}
}
