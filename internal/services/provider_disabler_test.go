package services

import (
	"testing"
	"time"
)

func TestProviderDisabler_DisablesStrictAuthProviderImmediately(t *testing.T) {
	disabler := NewProviderDisabler(2, []string{"mistral"})

	disabled, reason := disabler.RecordAuthFailure("mistral", "mistral-small-2506")

	if !disabled {
		t.Fatal("expected strict auth provider to be disabled")
	}
	if reason == "" {
		t.Fatal("expected disable reason")
	}
	if !disabler.IsDisabled("mistral") {
		t.Fatal("expected mistral to remain disabled")
	}
}

func TestProviderDisabler_DisablesAfterDistinctModelFailures(t *testing.T) {
	disabler := NewProviderDisabler(2, nil)

	disabled, _ := disabler.RecordAuthFailure("provider-a", "model-1")
	if disabled {
		t.Fatal("expected first model auth failure to stay below threshold")
	}

	disabled, reason := disabler.RecordAuthFailure("provider-a", "model-2")
	if !disabled {
		t.Fatal("expected second distinct model auth failure to disable provider")
	}
	if reason == "" {
		t.Fatal("expected disable reason")
	}
}

func TestProviderDisabler_DoesNotDoubleCountSameModel(t *testing.T) {
	disabler := NewProviderDisabler(2, nil)

	disabled, _ := disabler.RecordAuthFailure("provider-a", "model-1")
	if disabled {
		t.Fatal("expected first model auth failure to stay below threshold")
	}

	disabled, _ = disabler.RecordAuthFailure("provider-a", "model-1")
	if disabled {
		t.Fatal("expected repeated model auth failure to stay below distinct-model threshold")
	}
}

func TestProviderDisabler_ExpiresAfterTTL(t *testing.T) {
	disabler := NewProviderDisablerWithTTL(1, nil, 10*time.Millisecond)

	disabled, _ := disabler.RecordAuthFailure("provider-a", "model-1")
	if !disabled || !disabler.IsDisabled("provider-a") {
		t.Fatal("expected provider to be disabled initially")
	}

	time.Sleep(20 * time.Millisecond)

	if disabler.IsDisabled("provider-a") {
		t.Fatal("expected provider to be automatically re-enabled after TTL")
	}
	if reason := disabler.DisabledReason("provider-a"); reason != "" {
		t.Fatalf("expected empty disabled reason after TTL, got: %q", reason)
	}
}

func TestProviderDisabler_EnableProvider(t *testing.T) {
	disabler := NewProviderDisabler(1, nil)

	disabler.RecordAuthFailure("provider-a", "model-1")
	if !disabler.IsDisabled("provider-a") {
		t.Fatal("expected provider to be disabled")
	}

	disabler.EnableProvider("provider-a")
	if disabler.IsDisabled("provider-a") {
		t.Fatal("expected provider to be enabled after EnableProvider call")
	}
}

