package service

import (
	"strings"
	"unicode/utf8"

	"infinite-canvas/backend/internal/model"
)

const taskClientContextIDMaxLength = 80

type createPageTaskClientContext struct {
	ConversationID string
	MessageID      string
}

// applyCreatePageTaskClientContext 仅把可用于精确恢复的创作页 metadata 写入索引列；
// 其他来源或不完整的客户端数据继续保留在 input_json，但不能扩大任务查询范围。
func applyCreatePageTaskClientContext(task *model.Task, input map[string]any) {
	if task == nil {
		return
	}
	context, ok := createPageTaskClientContextFromInput(input)
	if !ok {
		return
	}
	task.ClientContextSource = model.TaskClientContextSourceCreatePage
	task.ClientConversationID = context.ConversationID
	task.ClientMessageID = context.MessageID
}

func createPageTaskClientContextFromInput(input map[string]any) (createPageTaskClientContext, bool) {
	metadata, ok := input["metadata"].(map[string]any)
	if !ok {
		return createPageTaskClientContext{}, false
	}
	source, _ := metadata["source"].(string)
	conversationID, _ := metadata["conversationId"].(string)
	messageID, _ := metadata["messageId"].(string)
	return createPageTaskClientContextFromValues(source, conversationID, messageID)
}

func createPageTaskClientContextFromValues(source string, conversationID string, messageID string) (createPageTaskClientContext, bool) {
	if source != model.TaskClientContextSourceCreatePage {
		return createPageTaskClientContext{}, false
	}
	conversationID = strings.TrimSpace(conversationID)
	messageID = strings.TrimSpace(messageID)
	if !validTaskClientContextID(conversationID) || !validTaskClientContextID(messageID) {
		return createPageTaskClientContext{}, false
	}
	return createPageTaskClientContext{ConversationID: conversationID, MessageID: messageID}, true
}

func validTaskClientContextID(value string) bool {
	return value != "" && utf8.RuneCountInString(value) <= taskClientContextIDMaxLength
}
