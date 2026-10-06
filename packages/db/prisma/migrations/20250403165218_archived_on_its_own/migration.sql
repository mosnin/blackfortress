/*
  Warnings:

  - The values [archived] on the enum `PolicyStatus` will be removed. If these variants are still used in the database, this will fail.
  - You are about to drop the column `ownerId` on the `Policy` table. All the data in the column will be lost.

*/
-- AlterEnum
BEGIN;
CREATE TYPE "PolicyStatus_new" AS ENUM ('draft', 'published', 'needs_review');
ALTER TABLE "Policy" ALTER COLUMN "status" DROP DEFAULT;
ALTER TABLE "Policy" ALTER COLUMN "status" TYPE "PolicyStatus_new" USING ("status"::text::"PolicyStatus_new");
ALTER TYPE "PolicyStatus" RENAME TO "PolicyStatus_old";
ALTER TYPE "PolicyStatus_new" RENAME TO "PolicyStatus";
DROP TYPE "PolicyStatus_old";
ALTER TABLE "Policy" ALTER COLUMN "status" SET DEFAULT 'draft';
COMMIT;

-- DropForeignKey
ALTER TABLE "Policy" DROP CONSTRAINT "Policy_ownerId_fkey";

-- AlterTable
ALTER TABLE "Policy" DROP COLUMN "ownerId",
ADD COLUMN     "assigneeId" TEXT,
ADD COLUMN     "isArchived" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN     "lastArchivedAt" TIMESTAMP(3);

-- AddForeignKey
ALTER TABLE "Policy" ADD CONSTRAINT "Policy_assigneeId_fkey" FOREIGN KEY ("assigneeId") REFERENCES "Member"("id") ON DELETE SET NULL ON UPDATE CASCADE;
