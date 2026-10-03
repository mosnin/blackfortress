/*
  Warnings:

  - A unique constraint covering the columns `[organizationId]` on the table `Trust` will be added. If there are existing duplicate values, this will fail.

*/
-- DropIndex
DROP INDEX "Trust_organizationId_friendlyUrl_key";

-- CreateIndex
CREATE UNIQUE INDEX "Trust_organizationId_key" ON "Trust"("organizationId");
