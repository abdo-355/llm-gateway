package services

import "testing"

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
