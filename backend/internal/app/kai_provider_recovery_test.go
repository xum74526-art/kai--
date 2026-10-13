package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestKAIProviderDeadlineRecoveryKeepsOriginalTask(t *testing.T) {
	svc := &Service{}
	for _, test := range []struct {
		name     string
		protocol model.ChannelInterfaceType
		taskType string
		mode     string
	}{
		{name: "AIAPIWay video", protocol: model.ChannelInterfaceAIAPIWayVideo, taskType: "canvas_video", mode: "video"},
		{name: "Bailian image", protocol: model.ChannelInterfaceBailianImage, taskType: "canvas_image", mode: "image"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := json.Marshal(canvasGenerationInput{Mode: test.mode, Config: providerConfig{BaseURL: "https://93.184.216.34", InterfaceType: string(test.protocol)}})
			if err != nil {
				t.Fatal(err)
			}
			startedAt := time.Now()
			task := model.Task{Type: test.taskType, ProviderRequestID: "existing-provider-task", StartedAt: &startedAt}
			if !svc.shouldDeferVideoProviderTask(task, string(input), context.DeadlineExceeded) {
				t.Fatal("accepted provider task must resume polling after the worker deadline")
			}
			if svc.shouldDeferVideoProviderTask(task, string(input), context.Canceled) {
				t.Fatal("explicit cancellation must not resume polling")
			}
			pending := providerStatePendingError{TaskID: task.ProviderRequestID}
			if got := svc.shouldDeferVideoProviderTask(task, string(input), pending); got != (test.mode == "video") {
				t.Fatalf("pending synchronization recovery = %v for %s", got, test.mode)
			}
			task.ProviderRequestID = ""
			if svc.shouldDeferVideoProviderTask(task, string(input), context.DeadlineExceeded) {
				t.Fatal("unaccepted requests must not be resubmitted by deadline recovery")
			}
		})
	}
}
