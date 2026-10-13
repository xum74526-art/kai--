package protocol

import (
	"context"
	"strings"
	"testing"
)

func TestKAIAIAPIWayPreservesMetadataAndInlineImage(t *testing.T) {
	adapter := officialPackageAdapter(t, "kai-aiapiway-video.yingce-plugin", "aiapiway-video")
	request := GenerationRequest{
		Model: "doubao-seedance-2-0-260128", Prompt: "镜头缓慢推进", Duration: 6,
		AspectRatio: "16:9", Resolution: "1080p", GenerateAudio: true,
		Images: []MediaReference{{URL: "/api/resources/local/file", DataURL: "data:image/png;base64,QUJD", Role: "first_frame"}},
		Audios: []MediaReference{{URL: "https://media.example/voice.mp3"}},
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	body := manifestTestBody(t, spec)
	if spec.Path != "/v1/video/generations" || body["seconds"] != nil || body["resolution"] != nil {
		t.Fatalf("AIAPIWay must retain its metadata contract: %#v", body)
	}
	metadata := body["metadata"].(map[string]any)
	if metadata["duration"] != float64(6) || metadata["resolution"] != "1080p" || metadata["generate_audio"] != true {
		t.Fatalf("metadata = %#v", metadata)
	}
	content := metadata["content"].([]any)
	first := content[0].(map[string]any)
	if len(content) != 2 || first["role"] != "first_frame" || first["image_url"].(map[string]any)["url"] != "data:image/png;base64,QUJD" {
		t.Fatalf("media roles or inline image lost: %#v", content)
	}
	state, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: "existing-task"}, []byte(`{"data":{"status":"FAILURE","fail_reason":"Invalid token"}}`))
	if err != nil || state.Status != StatusFailed || state.Message != "Invalid token" {
		t.Fatalf("failure = %#v, %v", state, err)
	}
}

func TestKAIAIAPIWayRejectsAudioOnlyAndUnsupported4K(t *testing.T) {
	adapter := officialPackageAdapter(t, "kai-aiapiway-video.yingce-plugin", "aiapiway-video")
	request := GenerationRequest{Model: "doubao-seedance-2-0-260128", Prompt: "镜头", Duration: 6, Resolution: "720p", Audios: []MediaReference{{URL: "https://media.example/voice.mp3"}}}
	if _, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request}); err == nil || !strings.Contains(err.Error(), "不能单独") {
		t.Fatalf("audio-only error = %v", err)
	}
	request.Audios = nil
	request.Resolution = "4K"
	if _, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request}); err == nil || !strings.Contains(err.Error(), "filter-off") {
		t.Fatalf("4K error = %v", err)
	}
	request.Model += "-filter-off"
	if _, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request}); err != nil {
		t.Fatal(err)
	}
}

func TestKAIBailianKeepsCustomSizeAndImageTaskContract(t *testing.T) {
	adapter := officialPackageAdapter(t, "kai-bailian-image.yingce-plugin", "bailian-image")
	request := GenerationRequest{Model: "qwen-image-3.0-pro", Prompt: "角色肖像", Instructions: "电影质感", AspectRatio: "2688x1536", ImageCount: 2}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/api/v1/services/aigc/image-generation/generation" || !spec.OriginPath || spec.Headers["X-DashScope-Async"] != "enable" {
		t.Fatalf("native endpoint contract = %#v", spec)
	}
	body := manifestTestBody(t, spec)
	parameters := body["parameters"].(map[string]any)
	if parameters["size"] != "2688*1536" || parameters["n"] != float64(2) {
		t.Fatalf("parameters = %#v", parameters)
	}
	if _, exists := parameters["prompt_extend"]; exists {
		t.Fatal("normal generations must preserve provider quality defaults")
	}
	content := dashscopeQwenImageContent(t, body)
	if content[0].(map[string]any)["text"] != "电影质感\n\n角色肖像" {
		t.Fatalf("instructions were lost: %#v", content)
	}
	request.Extra = map[string]any{"connectionTest": true}
	spec, err = adapter.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	parameters = manifestTestBody(t, spec)["parameters"].(map[string]any)
	if parameters["prompt_extend"] != false || parameters["enable_thinking"] != false {
		t.Fatalf("connection test parameters = %#v", parameters)
	}
	state, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: "image-task"}, []byte(`{"output":{"task_status":"SUCCEEDED","choices":[{"message":{"content":[{"image":"https://media.example/a.png"}]}},{"message":{"content":[{"image":"data:image/png;base64,QUJD"}]}}]}}`))
	if err != nil || state.Status != StatusSucceeded || state.Result == nil || len(state.Result.Images) != 2 {
		t.Fatalf("result = %#v, %v", state, err)
	}
	if state.Result.Images[1].DataURL != "data:image/png;base64,QUJD" {
		t.Fatalf("inline result = %#v", state.Result.Images[1])
	}
}
