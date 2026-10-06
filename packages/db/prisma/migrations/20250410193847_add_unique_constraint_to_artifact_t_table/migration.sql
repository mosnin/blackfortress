/*
  Warnings:

  - A unique constraint covering the columns `[organizationId,evidenceId,policyId]` on the table `Artifact` will be added. If there are existing duplicate values, this will fail.

*/
-- CreateIndex
CREATE UNIQUE INDEX "Artifact_organizationId_evidenceId_policyId_key" ON "Artifact"("organizationId", "evidenceId", "policyId");


-- Delete duplicate rows before adding unique constraint
WITH duplicates AS (
  SELECT 
    "organizationId",
    "evidenceId",
    "policyId",
    "id",
    ROW_NUMBER() OVER (
      PARTITION BY "organizationId", "evidenceId", "policyId"
      ORDER BY "createdAt" ASC
    ) as row_num
  FROM "Artifact"
  WHERE "evidenceId" IS NOT NULL OR "policyId" IS NOT NULL
)
DELETE FROM "Artifact" 
WHERE "id" IN (
  SELECT "id" 
  FROM duplicates 
  WHERE "row_num" > 1
);
