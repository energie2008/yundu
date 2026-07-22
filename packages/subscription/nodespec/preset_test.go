package nodespec

import (
	"testing"
	"time"
)

func TestBuildDefaultPresets(t *testing.T) {
	presets := BuildDefaultPresets()
	if len(presets) < 8 {
		t.Fatalf("expected at least 8 builtin presets, got %d", len(presets))
	}
	for _, p := range presets {
		if err := p.Validate(); err != nil {
			t.Errorf("preset %s failed validation: %v", p.ID, err)
		}
		if p.KernelCompat == "" {
			t.Errorf("preset %s has empty kernel_compat", p.ID)
		}
		if len(p.ClientSupport) == 0 {
			t.Errorf("preset %s has empty client_support", p.ID)
		}
	}
	t.Logf("loaded %d builtin presets", len(presets))
}

func TestPresetDiff(t *testing.T) {
	presets := BuildDefaultPresets()
	vlessReality := presets[0]

	spec := vlessReality.BaseSpec
	diff := DiffFromPreset(&spec, vlessReality)
	if len(diff.ModifiedFields()) != 0 {
		t.Errorf("expected no modifications, got %v", diff.ModifiedFields())
	}

	spec.Transport.Type = "ws"
	diff = DiffFromPreset(&spec, vlessReality)
	if !diff.HasModifications() {
		t.Error("expected modifications after changing transport")
	}
	modified := diff.ModifiedFields()
	found := false
	for _, f := range modified {
		if f == "transport.type" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected transport.type in modified fields, got %v", modified)
	}
}

func TestPresetRegistry(t *testing.T) {
	reg := NewPresetRegistry()
	defaults := BuildDefaultPresets()
	for _, p := range defaults {
		if err := reg.Register(p); err != nil {
			t.Fatalf("failed to register preset %s: %v", p.ID, err)
		}
	}

	xrayOnly := reg.ListCompatible("xray")
	singboxOnly := reg.ListCompatible("singbox")
	if len(xrayOnly) == 0 {
		t.Error("expected some xray-only presets (like XHTTP)")
	}
	t.Logf("xray-compatible: %d, singbox-compatible: %d", len(xrayOnly), len(singboxOnly))

	byVLESS := reg.ListByProtocol(ProtocolVLESS)
	if len(byVLESS) == 0 {
		t.Error("expected VLESS presets")
	}
	t.Logf("VLESS presets: %d", len(byVLESS))
}

func TestLoadPresetRegistry(t *testing.T) {
	reg, err := LoadPresetRegistry("../presets")
	if err != nil {
		t.Fatalf("LoadPresetRegistry failed: %v", err)
	}
	total := len(reg.List())
	if total < 10 {
		t.Errorf("expected at least 10 presets (builtin + yaml), got %d", total)
	}
	t.Logf("total presets after loading from presets/ dir: %d", total)
}

func TestKernelCompatLevels(t *testing.T) {
	now := time.Now()
	tests := []struct {
		level KernelCompatLevel
		valid bool
	}{
		{CompatBoth, true},
		{CompatXrayOnly, true},
		{CompatSingboxOnly, true},
		{CompatExperimental, true},
		{KernelCompatLevel("invalid"), false},
	}
	for _, tc := range tests {
		p := &PresetTemplate{
			ID:            "test-" + string(tc.level),
			Name:          "Test",
			Protocol:      ProtocolVLESS,
			Transport:     TransportTCP,
			Security:      SecurityNone,
			ClientSupport: []string{"test"},
			KernelCompat:  tc.level,
			BaseSpec: NodeSpec{
				Protocol: ProtocolVLESS,
				Port:     443,
			},
			UpdatedFromUpstream: now,
		}
		err := p.Validate()
		if tc.valid && err != nil {
			t.Errorf("compat %s should be valid: %v", tc.level, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("compat %s should be invalid", tc.level)
		}
	}
}

func TestPresetApplyToSpec(t *testing.T) {
	presets := BuildDefaultPresets()
	vlessGrpc := presets[0]

	spec := &NodeSpec{}
	vlessGrpc.ApplyToSpec(spec)

	if spec.Protocol != ProtocolVLESS {
		t.Errorf("expected protocol vless, got %s", spec.Protocol)
	}
	if spec.PresetID != vlessGrpc.ID {
		t.Errorf("expected preset_id %s, got %s", vlessGrpc.ID, spec.PresetID)
	}
}
