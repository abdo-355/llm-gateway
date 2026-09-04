package services

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthDisableThreshold = 2
	defaultAuthDisableTTL       = 10 * time.Minute
)

type disabledEntry struct {
	reason string
	expiry time.Time
}

type ProviderDisabler struct {
	mu                   sync.Mutex
	authDisableThreshold int
	authDisableTTL       time.Duration
	strictAuthProviders  map[string]struct{}
	authFailures         map[string]map[string]struct{}
	disabledReasons      map[string]disabledEntry
}

func NewDefaultProviderDisabler() *ProviderDisabler {
	return NewProviderDisablerWithTTL(defaultAuthDisableThreshold, configuredStrictAuthProviders(), configuredAuthDisableTTL())
}

func NewProviderDisabler(authDisableThreshold int, strictAuthProviders []string) *ProviderDisabler {
	return NewProviderDisablerWithTTL(authDisableThreshold, strictAuthProviders, defaultAuthDisableTTL)
}

func NewProviderDisablerWithTTL(authDisableThreshold int, strictAuthProviders []string, ttl time.Duration) *ProviderDisabler {
	if authDisableThreshold < 1 {
		authDisableThreshold = defaultAuthDisableThreshold
	}
	if ttl <= 0 {
		ttl = defaultAuthDisableTTL
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
		authDisableTTL:       ttl,
		strictAuthProviders:  strict,
		authFailures:         make(map[string]map[string]struct{}),
		disabledReasons:      make(map[string]disabledEntry),
	}
}

func configuredStrictAuthProviders() []string {
	raw := strings.TrimSpace(os.Getenv("STRICT_AUTH_PROVIDERS"))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func configuredAuthDisableTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("AUTH_DISABLE_TTL_SECONDS"))
	if raw == "" {
		return defaultAuthDisableTTL
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return defaultAuthDisableTTL
}

func (d *ProviderDisabler) RecordAuthFailure(providerID, model string) (bool, string) {
	if d == nil || providerID == "" {
		return false, ""
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	if entry, ok := d.disabledReasons[providerID]; ok {
		if !entry.expiry.IsZero() && now.After(entry.expiry) {
			delete(d.disabledReasons, providerID)
			delete(d.authFailures, providerID)
		} else {
			return true, entry.reason
		}
	}

	if _, ok := d.strictAuthProviders[providerID]; ok {
		reason := "strict auth provider returned auth failure"
		d.disabledReasons[providerID] = disabledEntry{
			reason: reason,
			expiry: now.Add(d.authDisableTTL),
		}
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
		d.disabledReasons[providerID] = disabledEntry{
			reason: reason,
			expiry: now.Add(d.authDisableTTL),
		}
		return true, reason
	}

	return false, ""
}

func (d *ProviderDisabler) IsDisabled(providerID string) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.disabledReasons[providerID]
	if !ok {
		return false
	}
	if !entry.expiry.IsZero() && time.Now().After(entry.expiry) {
		delete(d.disabledReasons, providerID)
		delete(d.authFailures, providerID)
		return false
	}
	return true
}

func (d *ProviderDisabler) DisabledReason(providerID string) string {
	if d == nil {
		return ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.disabledReasons[providerID]
	if !ok {
		return ""
	}
	if !entry.expiry.IsZero() && time.Now().After(entry.expiry) {
		delete(d.disabledReasons, providerID)
		delete(d.authFailures, providerID)
		return ""
	}
	return entry.reason
}

func (d *ProviderDisabler) EnableProvider(providerID string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.disabledReasons, providerID)
	delete(d.authFailures, providerID)
}
