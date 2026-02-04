#!/usr/bin/env ts-node

import * as dotenv from "dotenv";
dotenv.config();

import { config } from "../src/config/providers";
import { vertexAIAdapter } from "../src/services/adapters/vertex";
import { ChatCompletionRequest, ProviderConfig } from "../src/types";
import { ProviderError } from "../src/errors";
import * as fs from "fs";

// Interfaces
interface CheckResult {
  provider: string;
  model: string;
  status: "pass" | "fail" | "rate-limited" | "payment-required";
  timestamp: string;
  requestDuration: number;
  response: {
    statusCode: number;
    hasContent: boolean;
    contentLength: number;
    finishReason: string | null;
    fullResponseBody: any;
  };
  headers: Record<string, string>;
  error?: {
    code: string;
    message: string;
    retryable: boolean;
    retriesAttempted: number;
  };
}

interface CheckReport {
  timestamp: string;
  totalModels: number;
  passed: number;
  failed: number;
  rateLimited: number;
  paymentRequired: number;
  totalDuration: number;
  results: CheckResult[];
  summary: {
    byProvider: Record<
      string,
      {
        total: number;
        passed: number;
        failed: number;
        rateLimited?: number;
        paymentRequired?: number;
      }
    >;
  };
}

// Rate limiter (1 request per second per provider)
class RateLimiter {
  private lastRequestTime: number = 0;
  private minInterval: number = 1000;

  async throttle(): Promise<void> {
    const now = Date.now();
    const timeSinceLastRequest = now - this.lastRequestTime;

    if (timeSinceLastRequest < this.minInterval) {
      const waitTime = this.minInterval - timeSinceLastRequest;
      await sleep(waitTime);
    }

    this.lastRequestTime = Date.now();
  }
}

// Utility functions
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function log(provider: string, model: string, message: string): void {
  const timestamp = new Date().toISOString();
  console.log(`[${timestamp}] [${provider}] [${model}] ${message}`);
}

// Collect all headers from response
function collectAllHeaders(headers: Headers): Record<string, string> {
  const allHeaders: Record<string, string> = {};
  headers.forEach((value, key) => {
    allHeaders[key] = value;
  });
  return allHeaders;
}

// Check model with retry logic
async function checkModel(
  providerId: string,
  model: string,
  providerConfig: ProviderConfig,
  apiKey: string | undefined,
  rateLimiter: RateLimiter,
): Promise<CheckResult> {
  const startTime = Date.now();
  const timestamp = new Date().toISOString();

  let retriesAttempted = 0;
  const maxRetries = 1;

  while (true) {
    try {
      // Rate limit throttle
      await rateLimiter.throttle();

      // Make request based on provider type
      let response: Response;
      let responseBody: any;

      if (providerConfig.providerType === "vertex") {
        // Vertex AI - use adapter
        const vertexRequest = vertexAIAdapter.transformRequest({
          model: model, // Required by type but ignored by adapter
          messages: [{ role: "user", content: "Hello" }],
          temperature: 0,
        });

        const url = vertexAIAdapter.buildEndpointUrl(
          providerConfig.baseUrl,
          model,
          false,
        );

        response = await fetch(url, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            ...(apiKey && { "x-goog-api-key": apiKey }),
          },
          body: JSON.stringify(vertexRequest),
          signal: AbortSignal.timeout(60000), // 60 second timeout
        });

        if (response.ok) {
          const vertexData = await response.json();
          responseBody = vertexAIAdapter.transformResponse(
            vertexData,
            model,
            `vertex-check-${Date.now()}`,
          );
        }
      } else {
        // OpenAI-compatible providers
        const request: ChatCompletionRequest = {
          model,
          messages: [{ role: "user", content: "Hello" }],
          temperature: 0,
          stream: false,
        };

        const headers: Record<string, string> = {
          "Content-Type": "application/json",
        };

        if (apiKey) {
          if (
            providerConfig.auth?.type === "header" &&
            providerConfig.auth.headerName
          ) {
            headers[providerConfig.auth.headerName] = apiKey;
          } else {
            headers["Authorization"] = `Bearer ${apiKey}`;
          }
        }

        response = await fetch(`${providerConfig.baseUrl}/chat/completions`, {
          method: "POST",
          headers,
          body: JSON.stringify(request),
          signal: AbortSignal.timeout(60000), // 60 second timeout
        });

        if (response.ok) {
          responseBody = await response.json();
        }
      }

      const requestDuration = Date.now() - startTime;
      const allHeaders = collectAllHeaders(response.headers);

      // Handle non-OK responses
      if (!response.ok) {
        if (response.status === 429 && retriesAttempted < maxRetries) {
          retriesAttempted++;
          const retryAfter = parseInt(allHeaders["retry-after"] || "5", 10);
          log(
            providerId,
            model,
            `Rate limited, retrying in ${retryAfter}s... (attempt ${retriesAttempted})`,
          );
          await sleep(retryAfter * 1000);
          continue;
        }

        if (response.status === 402) {
          log(
            providerId,
            model,
            `status=payment-required duration=${requestDuration}ms`,
          );
          log(
            providerId,
            model,
            `error_code=402 error_message="Payment required: Account credit depleted" retryable=false`,
          );
          return {
            provider: providerId,
            model,
            status: "payment-required",
            timestamp,
            requestDuration,
            response: {
              statusCode: 402,
              hasContent: false,
              contentLength: 0,
              finishReason: null,
              fullResponseBody: null,
            },
            headers: allHeaders,
            error: {
              code: "402",
              message: "Payment required: Account credit depleted",
              retryable: false,
              retriesAttempted,
            },
          };
        }

        throw new ProviderError(
          `HTTP ${response.status}: ${await response.text()}`,
          response.status,
          response.status === 429,
        );
      }

      // Validate response
      const hasContent =
        responseBody?.choices?.[0]?.message?.content?.length > 0;
      const contentLength =
        responseBody?.choices?.[0]?.message?.content?.length || 0;
      const finishReason = responseBody?.choices?.[0]?.finish_reason || null;

      log(providerId, model, `status=pass duration=${requestDuration}ms`);
      log(
        providerId,
        model,
        `response_status=${response.status} has_content=${hasContent} content_length=${contentLength} finish_reason=${finishReason}`,
      );
      log(providerId, model, `headers=${JSON.stringify(allHeaders)}`);

      return {
        provider: providerId,
        model,
        status: "pass",
        timestamp,
        requestDuration,
        response: {
          statusCode: response.status,
          hasContent,
          contentLength,
          finishReason,
          fullResponseBody: responseBody,
        },
        headers: allHeaders,
      };
    } catch (error) {
      const requestDuration = Date.now() - startTime;

      if (error instanceof ProviderError) {
        if (error.statusCode === 429 && retriesAttempted < maxRetries) {
          retriesAttempted++;
          log(
            providerId,
            model,
            `Rate limited on error, retrying... (attempt ${retriesAttempted})`,
          );
          await sleep(5000);
          continue;
        }

        const status = error.statusCode === 429 ? "rate-limited" : "fail";
        log(
          providerId,
          model,
          `status=${status} duration=${requestDuration}ms`,
        );
        log(
          providerId,
          model,
          `error_code=${error.statusCode} error_message="${error.message}" retryable=${error.isRetryable} retries_attempted=${retriesAttempted}`,
        );

        return {
          provider: providerId,
          model,
          status,
          timestamp,
          requestDuration,
          response: {
            statusCode: error.statusCode,
            hasContent: false,
            contentLength: 0,
            finishReason: null,
            fullResponseBody: null,
          },
          headers: {},
          error: {
            code: String(error.statusCode),
            message: error.message,
            retryable: error.isRetryable,
            retriesAttempted,
          },
        };
      }

      log(providerId, model, `status=fail duration=${requestDuration}ms`);
      log(
        providerId,
        model,
        `error_code=ERROR error_message="${error instanceof Error ? error.message : String(error)}" retryable=false retries_attempted=${retriesAttempted}`,
      );

      return {
        provider: providerId,
        model,
        status: "fail",
        timestamp,
        requestDuration,
        response: {
          statusCode: 0,
          hasContent: false,
          contentLength: 0,
          finishReason: null,
          fullResponseBody: null,
        },
        headers: {},
        error: {
          code: "ERROR",
          message: error instanceof Error ? error.message : String(error),
          retryable: false,
          retriesAttempted,
        },
      };
    }
  }
}

// Check all models for a provider
async function checkProvider(provider: ProviderConfig): Promise<CheckResult[]> {
  const providerId = provider.id;
  const rateLimiter = new RateLimiter();
  const results: CheckResult[] = [];

  log(
    providerId,
    "ALL",
    `Starting checks for ${provider.models.list.length} models`,
  );

  // Get API key from environment variable
  const apiKey =
    provider.auth.type !== "none" ? process.env[provider.auth.env] : undefined;

  for (const model of provider.models.list) {
    const result = await checkModel(
      providerId,
      model,
      provider,
      apiKey,
      rateLimiter,
    );
    results.push(result);
  }

  return results;
}

// Validate all API keys exist
function validateApiKeys(): { valid: boolean; missing: string[] } {
  const missing: string[] = [];

  for (const provider of config.providers) {
    if (provider.auth.type !== "none") {
      const apiKey = process.env[provider.auth.env];
      if (!apiKey) {
        missing.push(`${provider.id} (${provider.auth.env})`);
      }
    }
  }

  return {
    valid: missing.length === 0,
    missing,
  };
}

// Main execution
async function main(): Promise<void> {
  const startTime = Date.now();
  const timestamp = new Date().toISOString();

  console.log(`[${timestamp}] Starting Model Check`);
  console.log(`[${timestamp}] Total providers: ${config.providers.length}`);

  // Validate all API keys first
  console.log(`[${timestamp}] Validating API keys...`);
  const keyValidation = validateApiKeys();

  if (!keyValidation.valid) {
    console.error(
      `[${timestamp}] ERROR: Missing API keys for the following providers:`,
    );
    for (const missing of keyValidation.missing) {
      console.error(`[${timestamp}]   - ${missing}`);
    }
    console.error(
      `[${timestamp}] Please set the required environment variables and try again.`,
    );
    console.error(`[${timestamp}] Check aborted.`);
    process.exit(1);
  }

  console.log(`[${timestamp}] All API keys validated successfully`);
  console.log(`[${timestamp}] Beginning model checks...`);
  console.log("");

  // Check all providers in parallel
  const providerResults = await Promise.all(
    config.providers.map((provider) => checkProvider(provider)),
  );

  // Flatten results
  const allResults = providerResults.flat();

  // Calculate stats
  const passed = allResults.filter((r) => r.status === "pass").length;
  const failed = allResults.filter((r) => r.status === "fail").length;
  const rateLimited = allResults.filter(
    (r) => r.status === "rate-limited",
  ).length;
  const paymentRequired = allResults.filter(
    (r) => r.status === "payment-required",
  ).length;
  const totalDuration = Date.now() - startTime;

  // Build summary by provider
  const byProvider: CheckReport["summary"]["byProvider"] = {};
  for (const provider of config.providers) {
    const providerResultsList = allResults.filter(
      (r) => r.provider === provider.id,
    );
    byProvider[provider.id] = {
      total: providerResultsList.length,
      passed: providerResultsList.filter((r) => r.status === "pass").length,
      failed: providerResultsList.filter((r) => r.status === "fail").length,
      rateLimited: providerResultsList.filter(
        (r) => r.status === "rate-limited",
      ).length,
      paymentRequired: providerResultsList.filter(
        (r) => r.status === "payment-required",
      ).length,
    };
  }

  // Build report
  const report: CheckReport = {
    timestamp,
    totalModels: allResults.length,
    passed,
    failed,
    rateLimited,
    paymentRequired,
    totalDuration,
    results: allResults,
    summary: { byProvider },
  };

  // Save report
  const reportFilename = `model-check-report-${timestamp.replace(/[:.]/g, "-")}.json`;
  fs.writeFileSync(reportFilename, JSON.stringify(report, null, 2));

  // Console summary
  const endTimestamp = new Date().toISOString();
  console.log("");
  console.log(`[${endTimestamp}] Check Complete`);
  console.log(
    `[${endTimestamp}] Results: passed=${passed} failed=${failed} rate_limited=${rateLimited} payment_required=${paymentRequired}`,
  );
  console.log(
    `[${endTimestamp}] Duration: ${(totalDuration / 1000).toFixed(1)}s`,
  );
  console.log(`[${endTimestamp}] Report: ${reportFilename}`);

  console.log(`\n=== Summary by Provider ===`);
  for (const [providerId, stats] of Object.entries(byProvider)) {
    const pct = ((stats.passed / stats.total) * 100).toFixed(0);
    let line = `${providerId}: ${stats.passed}/${stats.total} operational (${pct}%)`;
    if (stats.paymentRequired && stats.paymentRequired > 0)
      line += ` - ${stats.paymentRequired} payment required`;
    if (stats.rateLimited && stats.rateLimited > 0)
      line += ` - ${stats.rateLimited} rate limited`;
    if (stats.failed > 0) line += ` - ${stats.failed} failed`;
    console.log(line);
  }
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
