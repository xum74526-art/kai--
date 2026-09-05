package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestApplyCreatePageTaskClientContextOnlyMaterializesValidMetadata(t *testing.T) {
	valid := model.Task{}
	applyCreatePageTaskClientContext(&valid, map[string]any{
		"metadata": map[string]any{
			"source":         model.TaskClientContextSourceCreatePage,
			"conversationId": " conversation-1 ",
			"messageId":      " message-1 ",
		},
	})
	if valid.ClientContextSource != model.TaskClientContextSourceCreatePage || valid.ClientConversationID != "conversation-1" || valid.ClientMessageID != "message-1" {
		t.Fatalf("valid metadata was not materialized: %#v", valid)
	}

	invalid := model.Task{}
	applyCreatePageTaskClientContext(&invalid, map[string]any{
		"metadata": map[string]any{
			"source":         model.TaskClientContextSourceCreatePage,
			"conversationId": "conversation-1",
		},
	})
	if invalid.ClientContextSource != "" || invalid.ClientConversationID != "" || invalid.ClientMessageID != "" {
		t.Fatalf("incomplete metadata must not be materialized: %#v", invalid)
	}
}

func TestTaskSummaryUsesMaterializedCreatePageContext(t *testing.T) {
	summary := taskSummaryForOutput(model.Task{
		ClientContextSource:  model.TaskClientContextSourceCreatePage,
		ClientConversationID: "conversation-1",
		ClientMessageID:      "message-1",
		InputJSON:            `{"metadata":{"source":"create-page","conversationId":"conversation-1","messageId":"message-1","batchIndex":2,"batchCount":4}}`,
	})
	if summary.ClientContext == nil || summary.ClientContext.ConversationID != "conversation-1" || summary.ClientContext.MessageID != "message-1" {
		t.Fatalf("TaskSummary did not use materialized client context: %#v", summary.ClientContext)
	}
	if summary.ClientContext.BatchIndex != 2 || summary.ClientContext.BatchCount != 4 {
		t.Fatalf("matching batch metadata was not retained: %#v", summary.ClientContext)
	}
}

func TestTaskSummaryRejectsMismatchedMaterializedBatchMetadata(t *testing.T) {
	summary := taskSummaryForOutput(model.Task{
		ClientContextSource:  model.TaskClientContextSourceCreatePage,
		ClientConversationID: "conversation-1",
		ClientMessageID:      "message-1",
		InputJSON:            `{"metadata":{"source":"create-page","conversationId":"foreign-conversation","messageId":"foreign-message","batchIndex":9,"batchCount":9}}`,
	})
	if summary.ClientContext == nil || summary.ClientContext.ConversationID != "conversation-1" || summary.ClientContext.MessageID != "message-1" {
		t.Fatalf("TaskSummary did not preserve materialized client context: %#v", summary.ClientContext)
	}
	if summary.ClientContext.BatchIndex != 0 || summary.ClientContext.BatchCount != 0 {
		t.Fatalf("mismatched JSON metadata must not override materialized context: %#v", summary.ClientContext)
	}
}

func TestTaskSummaryFallsBackToLegacyCreatePageMetadata(t *testing.T) {
	summary := taskSummaryForOutput(model.Task{
		InputJSON: `{"metadata":{"source":"create-page","conversationId":"conversation-1","messageId":"message-1","batchIndex":2,"batchCount":4}}`,
	})
	if summary.ClientContext == nil || summary.ClientContext.ConversationID != "conversation-1" || summary.ClientContext.MessageID != "message-1" || summary.ClientContext.BatchIndex != 2 || summary.ClientContext.BatchCount != 4 {
		t.Fatalf("legacy JSON fallback is invalid: %#v", summary.ClientContext)
	}
}

func TestNormalizedCreatePageTaskQueryRequiresContextPair(t *testing.T) {
	for _, options := range []TaskListOptions{
		{ConversationID: "conversation-1"},
		{MessageID: "message-1"},
		{ConversationID: strings.Repeat("c", taskClientContextIDMaxLength+1), MessageID: "message-1"},
	} {
		_, _, _, err := normalizedCreatePageTaskQuery(options)
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Status != http.StatusBadRequest {
			t.Fatalf("options %#v error = %v, want public bad request", options, err)
		}
	}

	conversationID, messageID, exact, err := normalizedCreatePageTaskQuery(TaskListOptions{ConversationID: " conversation-1 ", MessageID: " message-1 "})
	if err != nil || !exact || conversationID != "conversation-1" || messageID != "message-1" {
		t.Fatalf("valid exact query = (%q, %q, %v, %v)", conversationID, messageID, exact, err)
	}
}

func TestCreatePageTaskQueryLimitHasHardMaximum(t *testing.T) {
	if got := createPageTaskQueryLimit(0); got != createPageTaskQueryMaxLimit {
		t.Fatalf("default exact query limit = %d, want %d", got, createPageTaskQueryMaxLimit)
	}
	if got := createPageTaskQueryLimit(createPageTaskQueryMaxLimit + 1); got != createPageTaskQueryMaxLimit {
		t.Fatalf("clamped exact query limit = %d, want %d", got, createPageTaskQueryMaxLimit)
	}
	if got := createPageTaskQueryLimit(15); got != 15 {
		t.Fatalf("explicit exact query limit = %d, want 15", got)
	}
}
