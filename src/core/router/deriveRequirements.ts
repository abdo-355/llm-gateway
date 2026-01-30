import { ChatCompletionRequest, ResponseFormatSchema } from '../openai/types';
import { DerivedRequirements, RouterHints, RouterOutputRequirement } from './types';

export function deriveRequirements(
  request: ChatCompletionRequest,
  routerHints?: RouterHints
): DerivedRequirements {
  let output: RouterOutputRequirement = 'text';
  let streaming: 'required' | 'preferred' | 'forbidden' = 'preferred';
  let tools: 'required' | 'allowed' | 'forbidden' = 'forbidden';

  // Check response_format for strict schema requirement
  if (request.response_format) {
    const parsed = ResponseFormatSchema.safeParse(request.response_format);
    if (parsed.success && parsed.data.type === 'json_schema') {
      if (parsed.data.json_schema.strict === true) {
        output = 'json_schema_strict';
      }
    }
  }

  // Streaming requirement
  if (request.stream === true) {
    streaming = 'required';
  } else if (request.stream === false) {
    streaming = 'forbidden';
  }

  // Tools requirement
  if (request.tools && request.tools.length > 0) {
    if (request.tool_choice === 'required' || 
        (typeof request.tool_choice === 'object' && request.tool_choice !== null)) {
      tools = 'required';
    } else if (request.tool_choice === 'none') {
      tools = 'forbidden';
    } else {
      tools = 'allowed';
    }
  }

  // Override with router hints if explicitly set
  if (routerHints?.requirements?.output) {
    output = routerHints.requirements.output;
  }
  if (routerHints?.requirements?.streaming) {
    streaming = routerHints.requirements.streaming;
  }
  if (routerHints?.requirements?.tools) {
    tools = routerHints.requirements.tools;
  }

  return { output, streaming, tools };
}
