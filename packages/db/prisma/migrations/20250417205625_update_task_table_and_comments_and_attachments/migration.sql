/*
  Warnings:

  - The values [open,closed] on the enum `TaskStatus` will be removed. If these variants are still used in the database, this will fail.
  - You are about to drop the column `dueDate` on the `Task` table. All the data in the column will be lost.
  - You are about to drop the column `relatedId` on the `Task` table. All the data in the column will be lost.
  - You are about to drop the column `relatedType` on the `Task` table. All the data in the column will be lost.
  - Added the required column `entityType` to the `Comment` table without a default value. This is not possible if the table is not empty.
  - Added the required column `entityId` to the `Task` table without a default value. This is not possible if the table is not empty.
  - Added the required column `entityType` to the `Task` table without a default value. This is not possible if the table is not empty.
  - Added the required column `frequency` to the `Task` table without a default value. This is not possible if the table is not empty.

*/
-- CreateEnum
CREATE TYPE "AttachmentEntityType" AS ENUM ('task', 'vendor', 'risk', 'comment');

-- CreateEnum
CREATE TYPE "AttachmentType" AS ENUM ('image', 'video', 'audio', 'document', 'other');

-- CreateEnum
CREATE TYPE "CommentEntityType" AS ENUM ('task', 'vendor', 'risk', 'policy');

-- CreateEnum
CREATE TYPE "TaskFrequency" AS ENUM ('daily', 'weekly', 'monthly', 'quarterly', 'yearly');

-- CreateEnum
CREATE TYPE "TaskEntityType" AS ENUM ('control', 'vendor', 'risk');

-- AlterEnum
BEGIN;
CREATE TYPE "TaskStatus_new" AS ENUM ('todo', 'in_progress', 'done');
ALTER TABLE "Task" ALTER COLUMN "status" DROP DEFAULT;
ALTER TABLE "Task" ALTER COLUMN "status" TYPE "TaskStatus_new" USING ("status"::text::"TaskStatus_new");
ALTER TYPE "TaskStatus" RENAME TO "TaskStatus_old";
ALTER TYPE "TaskStatus_new" RENAME TO "TaskStatus";
DROP TYPE "TaskStatus_old";
ALTER TABLE "Task" ALTER COLUMN "status" SET DEFAULT 'todo';
COMMIT;

-- DropIndex
DROP INDEX "Task_relatedId_idx";

-- DropIndex
DROP INDEX "Task_relatedId_organizationId_idx";

-- AlterTable
ALTER TABLE "Comment" ADD COLUMN     "entityType" "CommentEntityType" NOT NULL;

-- AlterTable
ALTER TABLE "Task" DROP COLUMN "dueDate",
DROP COLUMN "relatedId",
DROP COLUMN "relatedType",
ADD COLUMN     "entityId" TEXT NOT NULL,
ADD COLUMN     "entityType" "TaskEntityType" NOT NULL,
ADD COLUMN     "frequency" "TaskFrequency" NOT NULL,
ADD COLUMN     "lastCompletedAt" TIMESTAMP(3),
ADD COLUMN     "department" "Departments" DEFAULT 'none',
ADD COLUMN     "order" INTEGER NOT NULL DEFAULT 0,
ALTER COLUMN "status" SET DEFAULT 'todo';

-- DropEnum
DROP TYPE "TaskType";

-- CreateTable
CREATE TABLE "Attachment" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('att'::text),
    "name" TEXT NOT NULL,
    "url" TEXT NOT NULL,
    "type" "AttachmentType" NOT NULL,
    "entityId" TEXT NOT NULL,
    "entityType" "AttachmentEntityType" NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "organizationId" TEXT NOT NULL,

    CONSTRAINT "Attachment_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "Task_entityId_idx" ON "Task"("entityId");

-- CreateIndex
CREATE INDEX "Task_entityId_organizationId_idx" ON "Task"("entityId", "organizationId");

-- AddForeignKey
ALTER TABLE "Attachment" ADD CONSTRAINT "Attachment_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
