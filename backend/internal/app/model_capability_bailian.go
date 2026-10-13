package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 保留百炼渠道的尺寸边界，避免协议升级后向上游提交不支持的 4K 编辑请求。
func validateBailianImageSize(modelName string, value string, editing bool) error {
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "×", "x")))
	if value == "" || value == "auto" || strings.Contains(value, ":") {
		return nil
	}
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if value == "1k" || value == "2k" || value == "4k" {
		if !strings.HasPrefix(modelName, "wan2.7-image") {
			return errors.New("当前百炼图片模型需要使用宽x高的尺寸")
		}
		if value == "4k" && (modelName != "wan2.7-image-pro" || editing) {
			return errors.New("4K 仅支持 wan2.7-image-pro 文生图")
		}
		return nil
	}
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return errors.New("百炼图片尺寸请使用宽x高，例如 2048x2048")
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return errors.New("百炼图片尺寸必须是正整数")
	}
	if width > height*8 || height > width*8 {
		return errors.New("百炼图片宽高比不能超过 8:1")
	}
	minPixels := int64(512 * 512)
	maxPixels := int64(2048 * 2048)
	if strings.HasPrefix(modelName, "wan2.7-image") {
		minPixels = int64(768 * 768)
	}
	if modelName == "wan2.7-image-pro" && !editing {
		maxPixels = int64(4096 * 4096)
	}
	pixels := int64(width) * int64(height)
	if pixels < minPixels || pixels > maxPixels {
		return fmt.Errorf("当前百炼图片模型尺寸像素需在 %d 至 %d 之间", minPixels, maxPixels)
	}
	return nil
}
