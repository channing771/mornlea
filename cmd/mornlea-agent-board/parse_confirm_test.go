package main

import (
	"testing"
)

// TestParseConfirmRequest 以 E-13-approval.json 的实际字段为准。
func TestParseConfirmRequest(t *testing.T) {
	data := []byte(`{
		"id": "E-13-approval",
		"title": "E-13 record-only 报告短设计批准",
		"category": "bounded",
		"kind": "approval",
		"question": "是否批准按该短设计实施？",
		"design": "",
		"status": "answered",
		"createdAt": "2026-08-25T02:40:28Z",
		"repliedAt": "2026-08-25T02:40:54.040Z"
	}`)
	req, err := parseConfirmRequest(data)
	if err != nil {
		t.Fatalf("解析请求失败: %v", err)
	}
	if req.ID != "E-13-approval" || req.Kind != "approval" || req.Category != "bounded" {
		t.Errorf("ID=%q Kind=%q Category=%q", req.ID, req.Kind, req.Category)
	}
	if req.Title != "E-13 record-only 报告短设计批准" {
		t.Errorf("Title = %q", req.Title)
	}
	if req.Status != "answered" {
		t.Errorf("Status = %q", req.Status)
	}
	if req.CreatedAt == "" || req.RepliedAt == "" {
		t.Errorf("CreatedAt/RepliedAt 不应为空")
	}
}

// TestParseConfirmRequestWithSupersededBy 锁定 supersededBy 字段解析。
func TestParseConfirmRequestWithSupersededBy(t *testing.T) {
	data := []byte(`{"id":"E-13-approval1","status":"answered","supersededBy":"E-13-approval2"}`)
	req, err := parseConfirmRequest(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.SupersededBy != "E-13-approval2" {
		t.Errorf("SupersededBy = %q", req.SupersededBy)
	}
}

// TestParseConfirmReply 以 E-13-approval.reply.json 的实际字段为准。
func TestParseConfirmReply(t *testing.T) {
	data := []byte(`{
		"id": "E-13-approval",
		"action": "approve",
		"text": "批准",
		"repliedAt": "2026-08-25T02:40:54.040Z"
	}`)
	reply, err := parseConfirmReply(data)
	if err != nil {
		t.Fatalf("解析回复失败: %v", err)
	}
	if reply.ID != "E-13-approval" || reply.Action != "approve" || reply.Text != "批准" {
		t.Errorf("ID=%q Action=%q Text=%q", reply.ID, reply.Action, reply.Text)
	}
	if reply.RepliedAt == "" {
		t.Errorf("RepliedAt 不应为空")
	}
}

// TestParseConfirmRequestInvalid 锁定非法 JSON 返回错误。
func TestParseConfirmRequestInvalid(t *testing.T) {
	if _, err := parseConfirmRequest([]byte("{not json")); err == nil {
		t.Errorf("非法 JSON 应返回错误")
	}
}
