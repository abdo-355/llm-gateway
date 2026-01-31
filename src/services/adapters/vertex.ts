import {
  ChatCompletionRequest,
  ChatCompletionResponse,
  SSEChunk,
  VertexAIContent,
  VertexAIRequest,
  VertexAIResponse,
} from "../../types";
import { logger } from "../../utils/logger";

export class VertexAIAdapter {
  transformRequest(request: ChatCompletionRequest): VertexAIRequest {
    const contents: VertexAIContent[] = request.messages.map((message) => {
      let text = "";
      if (typeof message.content === "string") {
        text = message.content;
      } else if (Array.isArray(message.content)) {
        text = message.content
          .filter((part) => part.type === "text")
          .map((part) => part.text || "")
          .join("");
      }

      // Vertex AI doesn't support 'system' role, map to 'user'
      const role = message.role === "system" ? "user" : message.role;

      return {
        role,
        parts: [{ text }],
      };
    });

    const generationConfig: VertexAIRequest["generationConfig"] = {};

    if (request.temperature !== undefined) {
      generationConfig.temperature = request.temperature;
    }
    if (request.max_tokens !== undefined) {
      generationConfig.maxOutputTokens = request.max_tokens;
    }
    if (request.top_p !== undefined) {
      generationConfig.topP = request.top_p;
    }

    return {
      contents,
      generationConfig,
    };
  }

  transformResponse(
    data: VertexAIResponse,
    model: string,
    requestId: string,
  ): ChatCompletionResponse {
    // Extract text from the response (like google-vortex)
    const candidate = data.candidates?.[0];
    const text = candidate?.content?.parts?.[0]?.text || "";
    const finishReason = candidate?.finishReason || "stop";

    const openAIFinishReason = this.mapFinishReason(finishReason);

    return {
      id: requestId,
      object: "chat.completion",
      created: Math.floor(Date.now() / 1000),
      model,
      choices: [
        {
          index: 0,
          message: {
            role: "assistant",
            content: text || "No response generated",
          },
          finish_reason: openAIFinishReason,
        },
      ],
      usage: {
        prompt_tokens: data.usageMetadata?.promptTokenCount || 0,
        completion_tokens: data.usageMetadata?.candidatesTokenCount || 0,
        total_tokens: data.usageMetadata?.totalTokenCount || 0,
      },
    };
  }

  buildEndpointUrl(
    baseUrl: string,
    model: string,
    streaming: boolean = false,
  ): string {
    const action = streaming ? "streamGenerateContent" : "generateContent";

    // Global endpoint only - no project/location specific URLs
    // Ensure baseUrl ends with /v1
    const cleanBaseUrl = baseUrl.endsWith('/v1') ? baseUrl : `${baseUrl}/v1`;
    return `${cleanBaseUrl}/publishers/google/models/${model}:${action}`;
  }

  transformStreamingChunk(chunk: unknown): SSEChunk | null {
    try {
      const data = chunk as VertexAIResponse;
      const candidate = data.candidates?.[0];
      const content = candidate?.content;
      const text = content?.parts?.[0]?.text || "";
      const finishReason = candidate?.finishReason;

      if (!text && !finishReason) {
        return null;
      }

      const openAIFinishReason = finishReason
        ? this.mapFinishReason(finishReason)
        : null;

      return {
        id: `vertex-chunk-${Date.now()}`,
        object: "chat.completion.chunk",
        created: Math.floor(Date.now() / 1000),
        model: "vertex-model",
        choices: [
          {
            index: 0,
            delta: {
              content: text || null,
            },
            finish_reason: openAIFinishReason,
          },
        ],
      };
    } catch (error) {
      logger.warn({
        event: "vertex_streaming_transform_error",
        error: error instanceof Error ? error.message : String(error),
        chunk: JSON.stringify(chunk),
      });
      return null;
    }
  }

  private mapFinishReason(
    vertexReason: string,
  ): "stop" | "length" | "content_filter" | null {
    switch (vertexReason.toUpperCase()) {
      case "STOP":
        return "stop";
      case "MAX_TOKENS":
      case "LENGTH":
        return "length";
      case "SAFETY":
      case "RECITATION":
        return "content_filter";
      case "OTHER":
      default:
        return null;
    }
  }
}

export const vertexAIAdapter = new VertexAIAdapter();
