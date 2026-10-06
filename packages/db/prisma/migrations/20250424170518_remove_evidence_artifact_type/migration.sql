/*
  Warnings:

  - The values [evidence] on the enum `ArtifactType` will be removed. If these variants are still used in the database, this will fail.

*/
-- AlterEnum
BEGIN;
CREATE TYPE "ArtifactType_new" AS ENUM ('policy', 'procedure', 'training');
ALTER TABLE "Artifact" ALTER COLUMN "type" TYPE "ArtifactType_new" USING ("type"::text::"ArtifactType_new");
ALTER TYPE "ArtifactType" RENAME TO "ArtifactType_old";
ALTER TYPE "ArtifactType_new" RENAME TO "ArtifactType";
DROP TYPE "ArtifactType_old";
COMMIT;
