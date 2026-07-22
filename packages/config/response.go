package config

type Response struct {
	Code      ErrorCode   `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"request_id"`
}

func OK(data interface{}, requestID string) *Response {
	return &Response{
		Code:      CodeSuccess,
		Message:   CodeSuccess.Message(),
		Data:      data,
		RequestID: requestID,
	}
}

func Fail(code ErrorCode, message string, requestID string) *Response {
	msg := message
	if msg == "" {
		msg = code.Message()
	}
	return &Response{
		Code:      code,
		Message:   msg,
		RequestID: requestID,
	}
}

type Pagination struct {
	Page     int `json:"page" form:"page"`
	PageSize int `json:"page_size" form:"page_size"`
	Total    int `json:"total"`
}

func (p *Pagination) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
}

// Offset 返回分页查询的 SQL OFFSET 值。
// 防御性归一化: 即使上层调用方忘记调用 Normalize(), 也不会产生负偏移或
// 因 PageSize 过大导致的整数溢出, 避免未受信输入穿透到 SQL OFFSET 子句。
func (p *Pagination) Offset() int {
	page := p.Page
	pageSize := p.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return (page - 1) * pageSize
}
