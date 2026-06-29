package gobridge

import (
	"encoding/base64"
	"strings"

	"github.com/openAgi2/cordcode-macbridge/core"
)

// AttachmentInput 对应 unified-bridge-protocol 的 send_message.attachments[]。
// kind="image"（或 mime 为 image/*）走图片路径，其余走文件路径；base64 为原始字节的标准 base64。
type AttachmentInput struct {
	Kind     string `json:"kind"`               // "image" | "file"
	Mime     string `json:"mime"`               // e.g. "image/png"
	Filename string `json:"filename,omitempty"` // 原始文件名（可选）
	Base64   string `json:"base64,omitempty"`   // 标准 base64 编码字节
}

// splitAttachments 把 wire 附件解码成 agent 驱动需要的 image/file 切片。
// 空 base64 / 解码失败的附件被跳过（不伪造内容，由调用方暴露真实结果）。
// 之前 go-bridge 对 send_message 硬编码 sess.Send(content, nil, nil)，导致 iOS 发来的图片
// 永远到不了 agent（即使 claudecode/codex/opencode driver 都已支持图片）。
func splitAttachments(inputs []AttachmentInput) (images []core.ImageAttachment, files []core.FileAttachment) {
	for _, a := range inputs {
		if a.Base64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(a.Base64)
		if err != nil || len(data) == 0 {
			continue
		}
		isImage := a.Kind == "image" || strings.HasPrefix(strings.ToLower(a.Mime), "image/")
		if isImage {
			images = append(images, core.ImageAttachment{
				MimeType: a.Mime,
				Data:     data,
				FileName: a.Filename,
			})
		} else {
			files = append(files, core.FileAttachment{
				MimeType: a.Mime,
				Data:     data,
				FileName: a.Filename,
			})
		}
	}
	return images, files
}
