OpenRouter has been completely removed from the LLM Gateway.

## Changes Made:

1. **Removed OpenRouter Provider** (src/config/providers.ts)
   - Removed entire provider configuration block (7 models)
   - Removed from providers array

2. **Removed OpenRouter Certifications** (src/config/providers.ts)
   - Removed all 7 OpenRouter entries from certifications array

## Remaining Providers (4):
1. **Groq** - 9 models
2. **Cerebras** - 6 models  
3. **Mistral** - 18 models
4. **Vertex** - 2 models

## Total Models: 35 (down from 42)

## TypeScript Compilation: ✅ Passes

The gateway now only uses the 4 reliable providers with working models.