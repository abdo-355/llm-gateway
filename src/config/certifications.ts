import { z } from 'zod';

export const CertificationSchema = z.object({
  provider: z.string(),
  model: z.string(),
  json_schema_strict: z.boolean(),
  tested_at: z.string().datetime(),
  notes: z.string().optional(),
});

export const CertificationsConfigSchema = z.object({
  certifications: z.array(CertificationSchema),
});

export type Certification = z.infer<typeof CertificationSchema>;
export type CertificationsConfig = z.infer<typeof CertificationsConfigSchema>;
