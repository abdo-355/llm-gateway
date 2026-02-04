import { Request, Response } from "express";
import { register } from "prom-client";

export async function metricsHandler(
  _req: Request,
  res: Response,
): Promise<void> {
  res.setHeader("Content-Type", register.contentType);
  res.end(await register.metrics());
}
