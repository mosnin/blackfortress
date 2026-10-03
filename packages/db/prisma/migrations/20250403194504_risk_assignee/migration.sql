/*
  Warnings:

  - You are about to drop the column `ownerId` on the `Risk` table. All the data in the column will be lost.

*/
-- DropForeignKey
ALTER TABLE "Risk" DROP CONSTRAINT "Risk_ownerId_fkey";

-- DropIndex
DROP INDEX "Risk_ownerId_idx";

-- AlterTable
ALTER TABLE "Risk" DROP COLUMN "ownerId",
ADD COLUMN     "assigneeId" TEXT;

-- AddForeignKey
ALTER TABLE "Risk" ADD CONSTRAINT "Risk_assigneeId_fkey" FOREIGN KEY ("assigneeId") REFERENCES "Member"("id") ON DELETE SET NULL ON UPDATE CASCADE;
