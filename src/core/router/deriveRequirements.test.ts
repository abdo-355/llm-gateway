import { deriveRequirements } from '../core/router/deriveRequirements';
import { ChatCompletionRequest, RouterHints } from '../core/openai/types';

describe('deriveRequirements', () => {
  it('should default to text output', () => {
    const request: ChatCompletionRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'test',
    };
    
    const result = deriveRequirements(request);
    
    expect(result.output).toBe('text');
    expect(result.streaming).toBe('preferred');
    expect(result.tools).toBe('forbidden');
  });

  it('should detect strict json_schema requirement', () => {
    const request: ChatCompletionRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'test',
      response_format: {
        type: 'json_schema',
        json_schema: {
          name: 'test',
          strict: true,
          schema: { type: 'object' },
        },
      },
    };
    
    const result = deriveRequirements(request);
    
    expect(result.output).toBe('json_schema_strict');
  });

  it('should detect streaming requirement', () => {
    const request: ChatCompletionRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'test',
      stream: true,
    };
    
    const result = deriveRequirements(request);
    
    expect(result.streaming).toBe('required');
  });

  it('should detect tools requirement', () => {
    const request: ChatCompletionRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'test',
      tools: [
        {
          type: 'function',
          function: {
            name: 'test',
            description: 'Test function',
            parameters: { type: 'object' },
          },
        },
      ],
    };
    
    const result = deriveRequirements(request);
    
    expect(result.tools).toBe('allowed');
  });

  it('should detect required tools', () => {
    const request: ChatCompletionRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'test',
      tools: [
        {
          type: 'function',
          function: {
            name: 'test',
            description: 'Test function',
            parameters: { type: 'object' },
          },
        },
      ],
      tool_choice: 'required',
    };
    
    const result = deriveRequirements(request);
    
    expect(result.tools).toBe('required');
  });

  it('should allow router hints to override', () => {
    const request: ChatCompletionRequest = {
      messages: [{ role: 'user', content: 'Hello' }],
      model: 'test',
      stream: false,
    };
    
    const hints: RouterHints = {
      requirements: {
        output: 'json_schema_strict',
        streaming: 'required',
        tools: 'required',
      },
    };
    
    const result = deriveRequirements(request, hints);
    
    expect(result.output).toBe('json_schema_strict');
    expect(result.streaming).toBe('required');
    expect(result.tools).toBe('required');
  });
});
