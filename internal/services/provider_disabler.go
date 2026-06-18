package services

import (
	"os"
	"strings"
	"sync"
)

const defaultAuthDisableThreshold = 2

type ProviderDisabler struct {
	mu                   sync.RWMutex
	authDisableThreshold int
	strictAuthProviders  map[string]struct{}
	authFailures         map[string]map[string]struct{}
	disabledReasons      map[string]string
}

func NewDefaultProviderDisabler() *ProviderDisabler {
	return NewProviderDisabler(defaultAuthDisableThreshold, configuredStrictAuthProviders())
}

func NewProviderDisabler(authDisableThreshold int, strictAuthProviders []string) *ProviderDisabler {
	if authDisableThreshold < 1 {
		authDisableThreshold = defaultAuthDisableThreshold
	}

	strict := make(map[string]struct{}, len(strictAuthProviders))
	for _, providerID := range strictAuthProviders {
		providerID = strings.TrimSpace(providerID)
		if providerID == "" {
			continue
		}
		strict[providerID] = struct{}{}
	}

	return &ProviderDisabler{
		authDisableThreshold: authDisableThreshold,
		strictAuthProviders:  strict,
		authFailures:         make(map[string]map[string]struct{}),
		disabledReasons:      make(map[string]string),
	}
}

func configuredStrictAuthProviders() []string {
	raw := strings.TrimSpace(os.Getenv("STRICT_AUTH_PROVIDERS"))
	if raw == "" {
		return []string{"mistral"}
	}
	return strings.Split(raw, ",")
}

func (d *ProviderDisabler) RecordAuthFailure(providerID, model string) (bool, string) {
	if d == nil || providerID == "" {
		return false, ""
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if reason, ok := d.disabledReasons[providerID]; ok {
		return true, reason
	}

	if _, ok := d.strictAuthProviders[providerID]; ok {
		reason := "strict auth provider returned auth failure"
		d.disabledReasons[providerID] = reason
		return true, reason
	}

	models := d.authFailures[providerID]
	if models == nil {
		models = make(map[string]struct{})
		d.authFailures[providerID] = models
	}
	models[model] = struct{}{}

	if len(models) >= d.authDisableThreshold {
		reason := "auth failures across multiple models"
		d.disabledReasons[providerID] = reason
		return true, reason
	}

	return false, ""
}

func (d *ProviderDisabler) IsDisabled(providerID string) bool {
	if d == nil {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.disabledReasons[providerID]
	return ok
}

func (d *ProviderDisabler) DisabledReason(providerID string) string {
	if d == nil {
		return ""
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.disabledReasons[providerID]
}
