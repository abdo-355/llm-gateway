import { LogicalModelRegistry, LogicalModelConfig } from "../types";

/**
 * Logical Model Registry
 *
 * Maps user-facing logical model IDs to internal routing configurations.
 * Each logical model defines a set of candidate (provider, model) pairs
 * with preference weights, allowing intelligent routing with automatic failover.
 *
 * Usage in API:
 *   POST /v1/chat/completions
 *   { "model": "chat-pro", "messages": [...] }
 *
 * The gateway will:
 * 1. Look up "chat-pro" in this registry
 * 2. Generate candidates from the configured list
 * 3. Score based on weights + health + quota
 * 4. Execute with automatic failover
 */

export const logicalModels: LogicalModelRegistry = {
  // ========= General Chat / Content =========

  /**
   * chat-lite
   * Fast, cost-effective chat responses
   * Best for: Simple Q&A, high-volume chatbots, quick responses
   * Models: Small, fast 8B parameter models
   */
  "chat-lite": {
    id: "chat-lite",
    taskType: "chat",
    candidates: [
      { provider: "groq", model: "llama-3.1-8b-instant", weight: 0.5 },
      { provider: "groq", model: "qwen/qwen3-32b", weight: 0.3 },
      { provider: "mistral", model: "mistral-small-2503", weight: 0.2 },
    ],
    slo: { maxLatencyMs: 15000, maxAttempts: 2 },
  },

  /**
   * chat-pro
   * Balanced quality and speed
   * Best for: General production chat, most use cases
   * Models: 70B parameter models with good all-around performance
   */
  "chat-pro": {
    id: "chat-pro",
    taskType: "chat",
    candidates: [
      { provider: "groq", model: "llama-3.3-70b-versatile", weight: 0.5 },
      { provider: "cerebras", model: "llama-3.3-70b", weight: 0.3 },
      { provider: "mistral", model: "mistral-large-latest", weight: 0.2 },
    ],
    slo: { maxLatencyMs: 30000, maxAttempts: 3 },
  },

  /**
   * chat-max
   * Maximum reasoning capability
   * Best for: Complex reasoning, analysis, difficult problems
   * Models: 120B+ parameter models, Mixture of Experts
   */
  "chat-max": {
    id: "chat-max",
    taskType: "chat",
    candidates: [
      { provider: "groq", model: "openai/gpt-oss-120b", weight: 0.4 },
      { provider: "cerebras", model: "gpt-oss-120b", weight: 0.3 },
      { provider: "mistral", model: "open-mixtral-8x22b", weight: 0.2 },
      { provider: "vertex", model: "gemini-3-pro-preview", weight: 0.1 },
    ],
    slo: { maxLatencyMs: 40000, maxAttempts: 3 },
  },

  // ========= Analysis / Reasoning =========

  /**
   * analysis-pro
   * Deep analysis and complex reasoning
   * Best for: Research, data analysis, complex problem solving
   * Models: Largest available models with best reasoning capabilities
   */
  "analysis-pro": {
    id: "analysis-pro",
    taskType: "analysis",
    candidates: [
      { provider: "groq", model: "openai/gpt-oss-120b", weight: 0.4 },
      {
        provider: "groq",
        model: "meta-llama/llama-4-maverick-17b-128e-instruct",
        weight: 0.3,
      },
      {
        provider: "cerebras",
        model: "qwen-3-235b-a22b-instruct-2507",
        weight: 0.2,
      },
      { provider: "vertex", model: "gemini-3-pro-preview", weight: 0.1 },
    ],
    slo: { maxLatencyMs: 40000, maxAttempts: 3 },
  },

  // ========= JSON / Structured Output =========

  /**
   * json-fast
   * Fast JSON extraction and structured output
   * Best for: Quick parsing, simple structured data extraction
   * Note: Prefers speed over strict schema compliance
   */
  "json-fast": {
    id: "json-fast",
    taskType: "json_extraction",
    candidates: [
      { provider: "mistral", model: "mistral-small-2503", weight: 0.4 },
      { provider: "mistral", model: "mistral-medium", weight: 0.3 },
      { provider: "groq", model: "llama-3.1-8b-instant", weight: 0.3 },
    ],
    slo: { maxLatencyMs: 15000, maxAttempts: 2 },
    requireStrictJson: true,
  },

  /**
   * json-safe
   * Reliable JSON extraction with guaranteed schema compliance
   * Best for: Production APIs, critical structured data
   * Note: Only routes to strict-schema-certified models
   */
  "json-safe": {
    id: "json-safe",
    taskType: "json_extraction",
    candidates: [
      // Strict schema guaranteed (platform-level support)
      { provider: "mistral", model: "mistral-large-latest", weight: 0.4 },
      { provider: "vertex", model: "gemini-3-pro-preview", weight: 0.3 },
      { provider: "vertex", model: "gemini-3-flash-preview", weight: 0.2 },
      { provider: "mistral", model: "mistral-small-2503", weight: 0.1 },
    ],
    slo: { maxLatencyMs: 25000, maxAttempts: 3 },
    requireStrictJson: true,
  },

  // ========= Code Generation =========

  /**
   * code-fast
   * Quick code generation
   * Best for: Autocomplete, simple functions, rapid prototyping
   * Models: Code-specialized models optimized for speed
   */
  "code-fast": {
    id: "code-fast",
    taskType: "code",
    candidates: [
      { provider: "mistral", model: "codestral-2501", weight: 0.5 },
      { provider: "mistral", model: "codestral-2405", weight: 0.3 },
      {
        provider: "groq",
        model: "meta-llama/llama-4-scout-17b-16e-instruct",
        weight: 0.2,
      },
    ],
    slo: { maxLatencyMs: 20000, maxAttempts: 2 },
  },

  /**
   * code-pro
   * Production code generation
   * Best for: Complex algorithms, production code, code review
   * Models: Best code models with large context windows
   */
  "code-pro": {
    id: "code-pro",
    taskType: "code",
    candidates: [
      { provider: "mistral", model: "codestral-2501", weight: 0.4 },
      { provider: "mistral", model: "mistral-large-latest", weight: 0.3 },
      { provider: "groq", model: "openai/gpt-oss-120b", weight: 0.3 },
    ],
    slo: { maxLatencyMs: 30000, maxAttempts: 3 },
  },

  // ========= Tool Orchestration =========

  /**
   * tools-pro
   * Function calling and tool orchestration
   * Best for: Agents, multi-step workflows, API integrations
   * Models: Excellent tool support and reasoning
   */
  "tools-pro": {
    id: "tools-pro",
    taskType: "tool_orchestration",
    candidates: [
      { provider: "mistral", model: "mistral-large-latest", weight: 0.4 },
      { provider: "vertex", model: "gemini-3-pro-preview", weight: 0.3 },
      { provider: "mistral", model: "mistral-medium", weight: 0.3 },
    ],
    slo: { maxLatencyMs: 30000, maxAttempts: 3 },
    requireTools: true,
  },
};

/**
 * Get a logical model configuration by ID
 * @param id - The logical model ID (e.g., "chat-pro")
 * @returns The configuration or undefined if not found
 */
export function getLogicalModel(id: string): LogicalModelConfig | undefined {
  return logicalModels[id];
}

/**
 * List all available logical model IDs
 * @returns Array of logical model IDs
 */
export function listLogicalModels(): string[] {
  return Object.keys(logicalModels);
}
