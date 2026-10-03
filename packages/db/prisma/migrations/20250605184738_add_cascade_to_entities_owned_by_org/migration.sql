-- DropForeignKey
ALTER TABLE "Attachment" DROP CONSTRAINT "Attachment_organizationId_fkey";

-- DropForeignKey
ALTER TABLE "Context" DROP CONSTRAINT "Context_organizationId_fkey";

-- DropForeignKey
ALTER TABLE "Onboarding" DROP CONSTRAINT "Onboarding_organizationId_fkey";

-- DropForeignKey
ALTER TABLE "Task" DROP CONSTRAINT "Task_organizationId_fkey";

-- DropForeignKey
ALTER TABLE "VendorContact" DROP CONSTRAINT "VendorContact_vendorId_fkey";

-- AddForeignKey
ALTER TABLE "Attachment" ADD CONSTRAINT "Attachment_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Context" ADD CONSTRAINT "Context_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Onboarding" ADD CONSTRAINT "Onboarding_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Task" ADD CONSTRAINT "Task_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "VendorContact" ADD CONSTRAINT "VendorContact_vendorId_fkey" FOREIGN KEY ("vendorId") REFERENCES "Vendor"("id") ON DELETE CASCADE ON UPDATE CASCADE;
