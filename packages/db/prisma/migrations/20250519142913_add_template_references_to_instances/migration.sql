-- AlterTable
ALTER TABLE "Control" ADD COLUMN     "controlTemplateId" TEXT;

-- AlterTable
ALTER TABLE "Policy" ADD COLUMN     "policyTemplateId" TEXT;

-- AlterTable
ALTER TABLE "Task" ADD COLUMN     "taskTemplateId" TEXT;

-- AddForeignKey
ALTER TABLE "Control" ADD CONSTRAINT "Control_controlTemplateId_fkey" FOREIGN KEY ("controlTemplateId") REFERENCES "FrameworkEditorControlTemplate"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Policy" ADD CONSTRAINT "Policy_policyTemplateId_fkey" FOREIGN KEY ("policyTemplateId") REFERENCES "FrameworkEditorPolicyTemplate"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Task" ADD CONSTRAINT "Task_taskTemplateId_fkey" FOREIGN KEY ("taskTemplateId") REFERENCES "FrameworkEditorTaskTemplate"("id") ON DELETE SET NULL ON UPDATE CASCADE;
