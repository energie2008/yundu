package config

import "testing"

// TestPaginationOffsetUnnormalized 验证 Offset() 在未经 Normalize() 的非法输入下的行为。
// Bug-C: Offset() = (Page-1)*PageSize，未调用 Normalize() 时:
//   - Page=0  → (0-1)*20 = -20 (负偏移，SQL OFFSET 负数行为未定义/报错)
//   - Page<0 → 负偏移
//   - 大 PageSize → 整数溢出
// 上层 handler 若忘记调用 Normalize()，未受信输入会穿透到 SQL 层。
// 本测试锁定安全契约: Offset() 必须永不为负，且应内部归一化防御。
func TestPaginationOffsetUnnormalized(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		// minValidFloor: 期望 Offset 至少 >= 此值 (即不应为负)
		minValidFloor int
	}{
		{"Page zero without Normalize", 0, 20, 0},
		{"Page negative without Normalize", -5, 20, 0},
		{"PageSize zero without Normalize", 3, 0, 0},
		{"PageSize negative without Normalize", 3, -10, 0},
		{"both zero", 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Pagination{Page: tc.page, PageSize: tc.pageSize}
			got := p.Offset()
			if got < tc.minValidFloor {
				t.Errorf("Offset() = %d, must be >= %d (negative OFFSET is unsafe for SQL)",
					got, tc.minValidFloor)
			}
		})
	}
}

// TestPaginationOffsetNormalizeAlwaysSafe 验证 Normalize() 后 Offset 永远非负。
// 这是修复后应满足的核心不变量。
func TestPaginationOffsetNormalizeAlwaysSafe(t *testing.T) {
	bad := []Pagination{
		{Page: 0, PageSize: 0},
		{Page: -1, PageSize: 20},
		{Page: 3, PageSize: -5},
		{Page: -10, PageSize: -10},
	}
	for _, p := range bad {
		p.Normalize()
		if off := p.Offset(); off < 0 {
			t.Errorf("after Normalize, Offset() = %d < 0 for Page=%d PageSize=%d",
				off, p.Page, p.PageSize)
		}
	}
}
