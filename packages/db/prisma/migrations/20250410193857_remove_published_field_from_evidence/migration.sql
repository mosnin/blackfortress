/*
  Warnings:

  - You are about to drop the column `published` on the `Evidence` table. All the data in the column will be lost.

*/

-- Migrate data from published to status
UPDATE "Evidence" SET "status" = 'published' WHERE "published" = true;

-- AlterTable
ALTER TABLE "Evidence" DROP COLUMN "published";
