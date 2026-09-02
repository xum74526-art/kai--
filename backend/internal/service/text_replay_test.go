package service

import "testing"

func TestIsTextReplayTaskRequest(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  bool
	}{
		{"replay 布尔 true", map[string]any{"replay": true}, true},
		{"replay 字符串 true", map[string]any{"replay": "true"}, true},
		{"replay 字符串 True 大小写", map[string]any{"replay": "TRUE"}, true},
		{"replay 布尔 false", map[string]any{"replay": false}, false},
		{"无 replay 字段", map[string]any{"mode": "text"}, false},
		{"replay 空字符串", map[string]any{"replay": ""}, false},
		{"replay 数字", map[string]any{"replay": 1}, false},
		{"replay nil", map[string]any{"replay": nil}, false},
	}
	for _, tc := range cases {
		if got := isTextReplayTaskRequest(tc.input); got != tc.want {
			t.Errorf("%s: isTextReplayTaskRequest(%v) = %v, want %v", tc.name, tc.input, got, tc.want)
		}
	}
}

func TestValidateTextReplayTaskRequest(t *testing.T) {
	valid := CreateTaskRequest{Type: "text"}
	if err := validateTextReplayTaskRequest(valid, valid.Type, map[string]any{"replay": true, "mode": "text"}); err != nil {
		t.Fatalf("valid replay request error = %v", err)
	}
	cases := []struct {
		name  string
		req   CreateTaskRequest
		type_ string
		input map[string]any
	}{
		{name: "非文本任务", req: CreateTaskRequest{Type: "canvas_image"}, type_: "canvas_image", input: map[string]any{"replay": true, "mode": "text"}},
		{name: "绑定逻辑模型", req: CreateTaskRequest{Type: "text", LogicalModelID: "logical-1"}, type_: "text", input: map[string]any{"replay": true, "mode": "text"}},
		{name: "模式错误", req: valid, type_: "text", input: map[string]any{"replay": true, "mode": "image"}},
		{name: "内嵌媒体", req: valid, type_: "text", input: map[string]any{"replay": true, "mode": "text", "image": "data:image/png;base64,AA=="}},
	}
	for _, tc := range cases {
		if err := validateTextReplayTaskRequest(tc.req, tc.type_, tc.input); err == nil {
			t.Errorf("%s: expected replay validation error", tc.name)
		}
	}
}
