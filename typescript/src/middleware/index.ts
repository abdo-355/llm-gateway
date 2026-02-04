import { authMiddleware } from "./auth";
import { rateLimitMiddleware } from "./rateLimit";
import { requestIdMiddleware } from "./requestId";
import { validateMiddleware } from "./validate";
import { errorHandler } from "./errorHandler";
import { helmetMiddleware } from "./helmet";
import { corsMiddleware } from "./cors";

export const middleware = {
  auth: authMiddleware,
  rateLimit: rateLimitMiddleware,
  requestId: requestIdMiddleware,
  validate: validateMiddleware,
  errorHandler,
  helmet: helmetMiddleware,
  cors: corsMiddleware,
};

export {
  authMiddleware,
  rateLimitMiddleware,
  requestIdMiddleware,
  validateMiddleware,
  errorHandler,
  helmetMiddleware,
  corsMiddleware,
};
