-- AlterTable
ALTER TABLE "AuditLog" ADD COLUMN     "memberId" TEXT;

-- CreateIndex
CREATE INDEX "AuditLog_memberId_idx" ON "AuditLog"("memberId");

-- AddForeignKey
ALTER TABLE "AuditLog" ADD CONSTRAINT "AuditLog_memberId_fkey" FOREIGN KEY ("memberId") REFERENCES "Member"("id") ON DELETE CASCADE ON UPDATE CASCADE;
