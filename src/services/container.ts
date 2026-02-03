import { Redis } from "ioredis";
import {
  ProviderService,
  providerService as singletonProviderService,
} from "./provider";
import { QuotaService, quotaService as singletonQuotaService } from "./quota";
import {
  HealthService,
  healthService as singletonHealthService,
} from "./health";
import { RouterService } from "./router";
import { AppConfig } from "../types";

export interface ServiceContainer {
  providerService: ProviderService;
  quotaService: QuotaService;
  healthService: HealthService;
  createRouter: (config: AppConfig) => RouterService;
}

export interface ServiceContainerOptions {
  redis?: Redis;
  providerService?: ProviderService;
  quotaService?: QuotaService;
  healthService?: HealthService;
}

let container: ServiceContainer | null = null;

export function createServiceContainer(
  options: ServiceContainerOptions = {},
): ServiceContainer {
  if (container) {
    return container;
  }

  const providerService = options.providerService || singletonProviderService;
  const quotaService = options.quotaService || singletonQuotaService;
  const healthService = options.healthService || singletonHealthService;

  const createRouter = (config: AppConfig): RouterService => {
    return new RouterService(config);
  };

  container = {
    providerService,
    quotaService,
    healthService,
    createRouter,
  };

  return container;
}

export function getServiceContainer(): ServiceContainer | null {
  return container;
}

export function resetServiceContainer(): void {
  container = null;
}

export function getProviderService(): ProviderService {
  return singletonProviderService;
}

export function getQuotaService(): QuotaService {
  return singletonQuotaService;
}

export function getHealthService(): HealthService {
  return singletonHealthService;
}
