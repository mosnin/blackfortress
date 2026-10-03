/*
  Warnings:

  - A unique constraint covering the columns `[friendlyUrl]` on the table `Trust` will be added. If there are existing duplicate values, this will fail.
  - A unique constraint covering the columns `[organizationId,friendlyUrl]` on the table `Trust` will be added. If there are existing duplicate values, this will fail.

*/
-- DropIndex
DROP INDEX "Trust_organizationId_key";

-- AlterTable
ALTER TABLE "Trust" ADD COLUMN     "friendlyUrl" TEXT;

-- CreateIndex
CREATE UNIQUE INDEX "Trust_friendlyUrl_key" ON "Trust"("friendlyUrl");

-- CreateIndex
CREATE INDEX "Trust_friendlyUrl_idx" ON "Trust"("friendlyUrl");

-- CreateIndex
CREATE UNIQUE INDEX "Trust_organizationId_friendlyUrl_key" ON "Trust"("organizationId", "friendlyUrl");
