import * as fs from 'fs';
import * as path from 'path';
import * as YAML from 'yaml';
import { ProvidersConfigSchema } from './providers/types';
import { CertificationsConfigSchema } from './config/certifications';

const args = process.argv.slice(2);
const command = args[0];
const configDir = args[1] || './config';

function validateConfig(): void {
  console.log(`Validating configuration in ${configDir}...\n`);

  const providersPath = path.join(configDir, 'providers.yaml');
  const certificationsPath = path.join(configDir, 'certifications.yaml');

  let hasErrors = false;

  // Validate providers.yaml
  if (!fs.existsSync(providersPath)) {
    console.error(`❌ providers.yaml not found at ${providersPath}`);
    hasErrors = true;
  } else {
    try {
      const providersRaw = YAML.parse(fs.readFileSync(providersPath, 'utf-8'));
      const result = ProvidersConfigSchema.safeParse(providersRaw);
      
      if (result.success) {
        console.log(`✅ providers.yaml is valid`);
        console.log(`   Found ${result.data.providers.length} provider(s):`);
        for (const provider of result.data.providers) {
          console.log(`   - ${provider.id} (${provider.models.mode}: ${provider.models.mode === 'allowlist' ? (provider.models as any).allow.length : 'N/A'} models)`);
        }
      } else {
        console.error(`❌ providers.yaml is invalid:`);
        for (const error of result.error.errors) {
          console.error(`   - ${error.path.join('.')}: ${error.message}`);
        }
        hasErrors = true;
      }
    } catch (e) {
      console.error(`❌ Failed to parse providers.yaml: ${e}`);
      hasErrors = true;
    }
  }

  console.log('');

  // Validate certifications.yaml
  if (!fs.existsSync(certificationsPath)) {
    console.error(`❌ certifications.yaml not found at ${certificationsPath}`);
    hasErrors = true;
  } else {
    try {
      const certificationsRaw = YAML.parse(fs.readFileSync(certificationsPath, 'utf-8'));
      const result = CertificationsConfigSchema.safeParse(certificationsRaw);
      
      if (result.success) {
        console.log(`✅ certifications.yaml is valid`);
        console.log(`   Found ${result.data.certifications.length} certification(s)`);
        const strictCount = result.data.certifications.filter(c => c.json_schema_strict).length;
        console.log(`   - ${strictCount} certified for strict schema`);
      } else {
        console.error(`❌ certifications.yaml is invalid:`);
        for (const error of result.error.errors) {
          console.error(`   - ${error.path.join('.')}: ${error.message}`);
        }
        hasErrors = true;
      }
    } catch (e) {
      console.error(`❌ Failed to parse certifications.yaml: ${e}`);
      hasErrors = true;
    }
  }

  console.log('');

  // Check environment variables
  console.log('Checking environment variables...');
  const requiredEnvVars = [
    'GROQ_API_KEY',
    'OPENROUTER_API_KEY',
    'OPENAI_API_KEY',
  ];
  
  for (const envVar of requiredEnvVars) {
    const value = process.env[envVar];
    if (value) {
      console.log(`✅ ${envVar} is set`);
    } else {
      console.log(`⚠️  ${envVar} is not set (optional)`);
    }
  }

  console.log('');

  if (hasErrors) {
    console.error('❌ Configuration validation failed');
    process.exit(1);
  } else {
    console.log('✅ All configuration files are valid');
    process.exit(0);
  }
}

function showHelp(): void {
  console.log(`
LLM Gateway CLI

Usage:
  npm run cli -- <command> [config-dir]

Commands:
  validate-config    Validate providers.yaml and certifications.yaml
  
Examples:
  npm run cli -- validate-config
  npm run cli -- validate-config ./config
`);
}

switch (command) {
  case 'validate-config':
    validateConfig();
    break;
  case 'help':
  case '--help':
  case '-h':
  default:
    showHelp();
    break;
}
