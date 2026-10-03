-- AlterTable
ALTER TABLE "Attachment" ADD COLUMN     "commentId" TEXT;

-- CreateIndex
CREATE INDEX "Attachment_entityId_entityType_idx" ON "Attachment"("entityId", "entityType");

-- AddForeignKey
ALTER TABLE "Attachment" ADD CONSTRAINT "Attachment_commentId_fkey" FOREIGN KEY ("commentId") REFERENCES "Comment"("id") ON DELETE SET NULL ON UPDATE CASCADE;
