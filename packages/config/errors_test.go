package config

import "testing"

func TestErrorCodeMessage(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want string
	}{
		{CodeSuccess, "ok"},
		{CodeBadRequest, "bad request"},
		{CodeUnauthorized, "unauthorized"},
		{CodeForbidden, "forbidden"},
		{CodeNotFound, "resource not found"},
	}
	for _, tt := range tests {
		if got := tt.code.Message(); got != tt.want {
			t.Errorf("ErrorCode(%d).Message() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestErrorCodeHTTPStatus(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want int
	}{
		{CodeSuccess, 200},
		{CodeBadRequest, 400},
		{CodeUnauthorized, 401},
		{CodeForbidden, 403},
		{CodeNotFound, 404},
		{CodeConflict, 409},
		{CodeValidationFailed, 422},
		{CodeInternalError, 500},
	}
	for _, tt := range tests {
		if got := tt.code.HTTPStatus(); got != tt.want {
			t.Errorf("ErrorCode(%d).HTTPStatus() = %d, want %d", tt.code, got, tt.want)
		}
	}
}

func TestPaginationNormalize(t *testing.T) {
	tests := []struct {
		in   Pagination
		want Pagination
	}{
		{Pagination{Page: 0, PageSize: 0}, Pagination{Page: 1, PageSize: 20}},
		{Pagination{Page: 2, PageSize: 200}, Pagination{Page: 2, PageSize: 100}},
		{Pagination{Page: 3, PageSize: 10}, Pagination{Page: 3, PageSize: 10}},
	}
	for _, tt := range tests {
		p := tt.in
		p.Normalize()
		if p.Page != tt.want.Page || p.PageSize != tt.want.PageSize {
			t.Errorf("Normalize() = (%d,%d), want (%d,%d)", p.Page, p.PageSize, tt.want.Page, tt.want.PageSize)
		}
	}
}

func TestPaginationOffset(t *testing.T) {
	p := Pagination{Page: 3, PageSize: 20}
	if got := p.Offset(); got != 40 {
		t.Errorf("Offset() = %d, want 40", got)
	}
}

func TestOKResponse(t *testing.T) {
	resp := OK(map[string]string{"hello": "world"}, "req-123")
	if resp.Code != CodeSuccess {
		t.Errorf("OK Code = %d, want 0", resp.Code)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("OK RequestID = %q, want req-123", resp.RequestID)
	}
}

func TestFailResponse(t *testing.T) {
	resp := Fail(CodeBadRequest, "", "req-456")
	if resp.Code != CodeBadRequest {
		t.Errorf("Fail Code = %d, want 40000", resp.Code)
	}
	if resp.Message != "bad request" {
		t.Errorf("Fail Message = %q, want 'bad request'", resp.Message)
	}
}
