/*
  Warnings:

  - You are about to drop the column `slug` on the `FrameworkEditorPolicyTemplate` table. All the data in the column will be lost.

*/
-- DropIndex
DROP INDEX "FrameworkEditorPolicyTemplate_slug_key";

-- AlterTable
ALTER TABLE "FrameworkEditorPolicyTemplate" DROP COLUMN "slug";
