package service

import (
	"testing"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
)

// ========== 双内核下发决策回归测试（防 xray/sing-box 串台事故） ==========
//
// 背景：2026-08-07 VPS206 事故——辅内核 xray runtime 的独立配置版本被直接推给
// sing-box 主内核 agent，触发 sing-box 校验失败（unknown field "api"）。
// 本组测试锁定 isAuxiliaryXrayRuntimeOnServer 的判定逻辑，防止回归。

func rt(id, typ string) *model.Runtime {
	return &model.Runtime{ID: uuid.MustParse(id), RuntimeType: typ}
}

const (
	sbID = "939b19e4-e445-4a05-be7b-097a608a9463" // 主内核 sing-box（示例 VPS206）
	xrID = "34492dcb-9f5f-4b79-b87e-c7f7d4c21e33" // 辅内核 xray（示例 VPS206）
)

func TestIsAuxiliaryXrayRuntimeOnServer_DualKernel(t *testing.T) {
	// 双内核服务器：sing-box 主 + xray 辅 → xray 必须是"辅内核"（独立配置不得推送）
	runtimes := []*model.Runtime{rt(sbID, "sing-box"), rt(xrID, "xray")}
	if !isAuxiliaryXrayRuntimeOnServer(runtimes, rt(xrID, "xray")) {
		t.Fatal("dual-kernel: xray runtime must be auxiliary (sing-box paired)")
	}
	// 主内核 sing-box 不是辅内核
	if isAuxiliaryXrayRuntimeOnServer(runtimes, rt(sbID, "sing-box")) {
		t.Fatal("dual-kernel: sing-box primary must NOT be auxiliary")
	}
}

func TestIsAuxiliaryXrayRuntimeOnServer_SingleXrayServer(t *testing.T) {
	// 纯 xray 服务器（无 sing-box 配对）：xray 是唯一内核，必须允许独立推送
	runtimes := []*model.Runtime{rt(xrID, "xray")}
	if isAuxiliaryXrayRuntimeOnServer(runtimes, rt(xrID, "xray")) {
		t.Fatal("single xray server: xray must NOT be auxiliary (no sing-box paired)")
	}
}

func TestIsAuxiliaryXrayRuntimeOnServer_SingleSingboxServer(t *testing.T) {
	runtimes := []*model.Runtime{rt(sbID, "sing-box")}
	if isAuxiliaryXrayRuntimeOnServer(runtimes, rt(sbID, "sing-box")) {
		t.Fatal("single sing-box server: sing-box must NOT be auxiliary")
	}
}

func TestIsAuxiliaryXrayRuntimeOnServer_EdgeCases(t *testing.T) {
	runtimes := []*model.Runtime{rt(sbID, "sing-box"), rt(xrID, "xray")}
	// nil 目标
	if isAuxiliaryXrayRuntimeOnServer(runtimes, nil) {
		t.Fatal("nil runtime must not be auxiliary")
	}
	// nil 列表
	if isAuxiliaryXrayRuntimeOnServer(nil, rt(xrID, "xray")) {
		t.Fatal("nil runtimes list must not be auxiliary")
	}
	// 目标不在列表中（仅自身）
	alone := []*model.Runtime{rt(xrID, "xray")}
	other := rt("a0dac90e-8f82-4714-808f-022d4f490387", "xray")
	if isAuxiliaryXrayRuntimeOnServer(alone, other) {
		t.Fatal("target not in list: must not be auxiliary")
	}
	// 内核类型归一化（singbox / xray-core 别名）
	aliased := []*model.Runtime{rt(sbID, "singbox"), rt(xrID, "xray-core")}
	if !isAuxiliaryXrayRuntimeOnServer(aliased, rt(xrID, "xray")) {
		t.Fatal("normalized aliases: xray-core with singbox pair must be auxiliary")
	}
}

func TestIsAuxiliaryXrayRuntimeOnServer_NonXrayTarget(t *testing.T) {
	// 目标不是 xray（例如自定义 runtime_type）→ 不是辅内核
	runtimes := []*model.Runtime{rt(sbID, "sing-box"), rt("11111111-2222-4333-8444-555555555555", "custom")}
	if isAuxiliaryXrayRuntimeOnServer(runtimes, rt("11111111-2222-4333-8444-555555555555", "custom")) {
		t.Fatal("non-xray runtime must not be auxiliary")
	}
}
