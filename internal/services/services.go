package services

import (
	"sync"

	"github.com/abdo-355/llm-gateway/internal/db"
)

var (
	quotaServiceInst *QuotaService
	quotaServiceOnce sync.Once

	healthServiceInst *HealthService
	healthServiceOnce sync.Once

	providerServiceInst *ProviderService
	providerServiceOnce sync.Once

	routerInst *Router
	routerOnce sync.Once
)

func GetQuotaService() *QuotaService {
	quotaServiceOnce.Do(func() {
		quotaServiceInst = NewQuotaService()
	})
	return quotaServiceInst
}

func GetHealthService() *HealthService {
	healthServiceOnce.Do(func() {
		healthServiceInst = NewHealthService()
	})
	return healthServiceInst
}

func GetProviderService() *ProviderService {
	providerServiceOnce.Do(func() {
		providerServiceInst = NewProviderService()
	})
	return providerServiceInst
}

func GetRouter() *Router {
	routerOnce.Do(func() {
		routerInst = NewRouter()
	})
	return routerInst
}

// CloseServices gracefully shuts down all services
// Called during application shutdown
func CloseServices() {
	if quotaServiceInst != nil {
		db.CloseRedis()
	}
}
