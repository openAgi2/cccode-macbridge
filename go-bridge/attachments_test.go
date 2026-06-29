package gobridge

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSplitAttachments_ImageAndFile(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4E, 0x47})
	pdf := base64.StdEncoding.EncodeToString([]byte{0x25, 0x50, 0x44, 0x46})
	inputs := []AttachmentInput{
		{Kind: "image", Mime: "image/png", Filename: "a.png", Base64: png},
		{Kind: "file", Mime: "application/pdf", Filename: "a.pdf", Base64: pdf},
	}
	images, files := splitAttachments(inputs)
	if len(images) != 1 || len(files) != 1 {
		t.Fatalf("want 1 image + 1 file, got %d images, %d files", len(images), len(files))
	}
	if images[0].MimeType != "image/png" || !bytes.Equal(images[0].Data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Errorf("unexpected image: mime=%s data=%v", images[0].MimeType, images[0].Data)
	}
	if images[0].FileName != "a.png" {
		t.Errorf("image filename not preserved: %q", images[0].FileName)
	}
	if files[0].MimeType != "application/pdf" || !bytes.Equal(files[0].Data, []byte{0x25, 0x50, 0x44, 0x46}) {
		t.Errorf("unexpected file: mime=%s data=%v", files[0].MimeType, files[0].Data)
	}
}

func TestSplitAttachments_KindInferredFromMime(t *testing.T) {
	jpg := base64.StdEncoding.EncodeToString([]byte{0xFF, 0xD8, 0xFF})
	// kind 留空，仅靠 mime=image/* 归类为图片（与 iOS wireAttachments 推导一致）。
	images, files := splitAttachments([]AttachmentInput{
		{Kind: "", Mime: "image/jpeg", Base64: jpg},
	})
	if len(images) != 1 || len(files) != 0 {
		t.Fatalf("mime image/* 应归类 image；got %d images %d files", len(images), len(files))
	}
}

func TestSplitAttachments_DropsInvalid(t *testing.T) {
	good := base64.StdEncoding.EncodeToString([]byte{0x1, 0x2})
	images, files := splitAttachments([]AttachmentInput{
		{Kind: "image", Mime: "image/png", Base64: ""},           // 空 base64 → 丢
		{Kind: "image", Mime: "image/png", Base64: "@@notb64@@"}, // 非法 base64 → 丢
		{Kind: "image", Mime: "image/png", Base64: good},         // 保留
	})
	if len(images) != 1 {
		t.Fatalf("want 1 valid image, got %d", len(images))
	}
	if len(files) != 0 {
		t.Errorf("unexpected files: %d", len(files))
	}
}

func TestSplitAttachments_NilEmpty(t *testing.T) {
	images, files := splitAttachments(nil)
	if images != nil || files != nil {
		t.Errorf("nil input should return nil slices, got images=%v files=%v", images, files)
	}
}
