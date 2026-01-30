import * as fs from 'fs';
import * as path from 'path';
import * as YAML from 'yaml';
import { ProvidersConfig, ProvidersConfigSchema, ProviderConfig } from '../providers/types';
import { CertificationsConfig, CertificationsConfigSchema, Certification } from './certifications';

export interface LoadedConfig {
  providers: ProviderConfig[];
  certifications: Certification[];
}

export function loadConfig(configDir: string = './config'): LoadedConfig {
  const providersPath = path.join(configDir, 'providers.yaml');
  const certificationsPath = path.join(configDir, 'certifications.yaml');

  if (!fs.existsSync(providersPath)) {
    throw new Error(`Providers config not found: ${providersPath}`);
  }

  if (!fs.existsSync(certificationsPath)) {
    throw new Error(`Certifications config not found: ${certificationsPath}`);
  }

  const providersRaw = YAML.parse(fs.readFileSync(providersPath, 'utf-8'));
  const certificationsRaw = YAML.parse(fs.readFileSync(certificationsPath, 'utf-8'));

  const providersResult = ProvidersConfigSchema.safeParse(providersRaw);
  if (!providersResult.success) {
    throw new Error(`Invalid providers config: ${providersResult.error.message}`);
  }

  const certificationsResult = CertificationsConfigSchema.safeParse(certificationsRaw);
  if (!certificationsResult.success) {
    throw new Error(`Invalid certifications config: ${certificationsResult.error.message}`);
  }

  return {
    providers: providersResult.data.providers,
    certifications: certificationsResult.data.certifications,
  };
}

export function loadProviderEnvVars(provider: ProviderConfig): string | undefined {
  switch (provider.auth.type) {
    case 'none':
      return undefined;
    case 'bearer_env':
    case 'header_env':
      return process.env[provider.auth.env];
    default:
      return undefined;
  }
}
