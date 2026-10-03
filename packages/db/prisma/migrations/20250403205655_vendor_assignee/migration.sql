/*
  Warnings:

  - You are about to drop the column `ownerId` on the `Vendor` table. All the data in the column will be lost.
  - Added the required column `assigneeId` to the `Vendor` table without a default value. This is not possible if the table is not empty.

*/
-- DropForeignKey
ALTER TABLE "Vendor" DROP CONSTRAINT "Vendor_ownerId_fkey";

-- DropIndex
DROP INDEX "Vendor_ownerId_idx";

-- AlterTable
ALTER TABLE "Vendor" DROP COLUMN "ownerId",
ADD COLUMN     "assigneeId" TEXT NOT NULL;

-- CreateIndex
CREATE INDEX "Vendor_assigneeId_idx" ON "Vendor"("assigneeId");

-- AddForeignKey
ALTER TABLE "Vendor" ADD CONSTRAINT "Vendor_assigneeId_fkey" FOREIGN KEY ("assigneeId") REFERENCES "Member"("id") ON DELETE CASCADE ON UPDATE CASCADE;
