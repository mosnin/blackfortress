-- AlterTable
ALTER TABLE "Policy" ADD COLUMN     "approverId" TEXT;

-- AddForeignKey
ALTER TABLE "Policy" ADD CONSTRAINT "Policy_approverId_fkey" FOREIGN KEY ("approverId") REFERENCES "Member"("id") ON DELETE SET NULL ON UPDATE CASCADE;
