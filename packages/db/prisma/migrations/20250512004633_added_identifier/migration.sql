-- DropIndex
DROP INDEX "FrameworkEditorRequirement_frameworkId_name_key";

-- AlterTable
ALTER TABLE "FrameworkEditorRequirement" ADD COLUMN     "identifier" TEXT NOT NULL DEFAULT '';
