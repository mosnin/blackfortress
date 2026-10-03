-- CreateEnum
CREATE TYPE "AuditLogEntityType" AS ENUM ('organization', 'framework', 'requirement', 'control', 'policy', 'task', 'people', 'risk', 'vendor', 'tests', 'integration');

-- AlterTable
ALTER TABLE "AuditLog" ADD COLUMN     "entityId" TEXT,
ADD COLUMN     "entityType" "AuditLogEntityType";

-- CreateIndex
CREATE INDEX "AuditLog_entityType_idx" ON "AuditLog"("entityType");
