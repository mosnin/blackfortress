/*
  Warnings:

  - You are about to drop the `Artifact` table. If the table is not empty, all the data it contains will be lost.
  - You are about to drop the `_ArtifactToControl` table. If the table is not empty, all the data it contains will be lost.

*/

-- CreateTable
CREATE TABLE "_ControlToPolicy" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_ControlToPolicy_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateIndex
CREATE INDEX "_ControlToPolicy_B_index" ON "_ControlToPolicy"("B");

-- AddForeignKey
ALTER TABLE "_ControlToPolicy" ADD CONSTRAINT "_ControlToPolicy_A_fkey" FOREIGN KEY ("A") REFERENCES "Control"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_ControlToPolicy" ADD CONSTRAINT "_ControlToPolicy_B_fkey" FOREIGN KEY ("B") REFERENCES "Policy"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- Manually copy relationships from Artifacts (that were linked to Policies) to Controls
-- into the new _ControlToPolicy table.
-- "A" in _ControlToPolicy is ControlId (from _ArtifactToControl."B")
-- "B" in _ControlToPolicy is PolicyId (from Artifact."policyId")
INSERT INTO "_ControlToPolicy" ("A", "B")
SELECT ATC."B", ART."policyId"
FROM "_ArtifactToControl" AS ATC
JOIN "Artifact" AS ART ON ATC."A" = ART."id"
WHERE ART."policyId" IS NOT NULL
ON CONFLICT ("A", "B") DO NOTHING;

-- DropForeignKey
ALTER TABLE "Artifact" DROP CONSTRAINT "Artifact_organizationId_fkey";

-- DropForeignKey
ALTER TABLE "Artifact" DROP CONSTRAINT "Artifact_policyId_fkey";

-- DropForeignKey
ALTER TABLE "_ArtifactToControl" DROP CONSTRAINT "_ArtifactToControl_A_fkey";

-- DropForeignKey
ALTER TABLE "_ArtifactToControl" DROP CONSTRAINT "_ArtifactToControl_B_fkey";

-- DropTable
DROP TABLE "Artifact";

-- DropTable
DROP TABLE "_ArtifactToControl";

-- DropEnum
DROP TYPE "ArtifactType";
