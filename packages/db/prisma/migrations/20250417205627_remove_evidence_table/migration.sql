/*
  Warnings:

  - You are about to drop the column `evidenceId` on the `Artifact` table. All the data in the column will be lost.
  - You are about to drop the `Evidence` table. If the table is not empty, all the data it contains will be lost.

*/

-- Add temporary column to Task to store original Evidence ID
ALTER TABLE "Task" ADD COLUMN "temp_evidence_id" TEXT;

-- Migrate existing Evidence records (excluding 'not_relevant') to Tasks
-- Joins Evidence -> Artifact (via evidenceId) -> _ArtifactToControl -> Control
-- Uses DISTINCT ON(e.id) to create one Task per Evidence.
-- IMPORTANT: This runs BEFORE dropping Artifact.evidenceId
INSERT INTO "Task" (
    "title",
    "description",
    "status",
    "entityId",         -- Control.id obtained via joins
    "entityType",
    "frequency",        -- Taken from Evidence.frequency, defaulting to quarterly if NULL
    "assigneeId",
    "organizationId",
    "createdAt",
    "updatedAt",
    "lastCompletedAt",  -- Set to NOW() if Evidence was published, else NULL
    "temp_evidence_id",  -- Store original Evidence ID temporarily
    "order"
)
SELECT DISTINCT ON (e.id)
    e."name",
    e."description",
    CASE e."status"
        WHEN 'published' THEN 'done'::"TaskStatus"
        ELSE 'todo'::"TaskStatus"
    END AS status,
    c.id AS "entityId", -- Control ID from join
    'control'::"TaskEntityType",
    COALESCE(e.frequency::text, 'quarterly')::"TaskFrequency" AS frequency, -- Use Evidence frequency or default
    e."assigneeId",
    e."organizationId",
    e."createdAt",
    e."updatedAt",
    CASE WHEN e."status" = 'published' THEN NOW() ELSE NULL END AS "lastCompletedAt", -- Set based on Evidence status
    e.id                -- Original Evidence ID for linking attachments
    , row_number() OVER () - 1 AS "order"
FROM "Evidence" e
INNER JOIN "Artifact" a ON e.id = a."evidenceId" -- Join Evidence to Artifact via the old FK column
INNER JOIN "_ArtifactToControl" atc ON a.id = atc."A"  -- Join Artifact to M2M table
INNER JOIN "Control" c ON atc."B" = c.id             -- Join M2M table to Control
WHERE e."status" <> 'not_relevant'; -- Exclude evidence marked as not relevant

-- Migrate Evidence fileUrls to Attachments
INSERT INTO "Attachment" (
    "name",             -- Derived from URL
    "url",              -- From Evidence.fileUrls array
    "type",             -- Defaulting to 'document'
    "entityId",         -- Task.id found via temp_evidence_id
    "entityType",       -- 'task'
    "organizationId",
    "createdAt",        -- Set explicitly
    "updatedAt"         -- Set explicitly
)
SELECT
    substring(file_url from '[^/]*$') AS name, -- Extract filename from URL
    file_url AS url,
    'document'::"AttachmentType" AS type,
    t.id AS "entityId",
    'task'::"AttachmentEntityType" AS entityType,
    t."organizationId",
    NOW() AS "createdAt", -- Use current time
    NOW() AS "updatedAt"  -- Use current time
FROM "Evidence" e
CROSS JOIN LATERAL unnest(e."fileUrls") AS file_url -- Unnest the array of URLs
INNER JOIN "Task" t ON e.id = t."temp_evidence_id" -- Link Evidence to Task via temp column
WHERE e."status" <> 'not_relevant'; -- Ensure we only process migrated evidence

-- Drop temporary column from Task
ALTER TABLE "Task" DROP COLUMN "temp_evidence_id";

-- Delete Artifact rows of type 'evidence' as they are now represented by Tasks
DELETE FROM "Artifact" WHERE type = 'evidence';

-- Now clean up the Artifact table relating to the old evidenceId
-- DropForeignKey
ALTER TABLE "Artifact" DROP CONSTRAINT IF EXISTS "Artifact_evidenceId_fkey";

-- DropIndex
DROP INDEX IF EXISTS "Artifact_organizationId_evidenceId_policyId_key";

-- AlterTable
ALTER TABLE "Artifact" DROP COLUMN IF EXISTS "evidenceId";

-- Clean up Evidence table foreign keys before dropping
-- DropForeignKey
ALTER TABLE "Evidence" DROP CONSTRAINT IF EXISTS "Evidence_assigneeId_fkey";

-- DropForeignKey
ALTER TABLE "Evidence" DROP CONSTRAINT IF EXISTS "Evidence_organizationId_fkey";

-- Drop the Evidence table and its enum
-- DropTable
DROP TABLE "Evidence";

-- DropEnum
DROP TYPE "EvidenceStatus";

ALTER TABLE "Attachment" ALTER COLUMN "updatedAt" DROP DEFAULT;