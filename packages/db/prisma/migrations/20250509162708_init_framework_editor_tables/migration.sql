/*
  Warnings:

  - Added the required column `version` to the `FrameworkEditorFramework` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "FrameworkEditorFramework" ADD COLUMN     "version" TEXT NOT NULL;

-- CreateTable
CREATE TABLE "FrameworkEditorVideo" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frk_vi'::text),
    "title" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "youtubeId" TEXT NOT NULL,
    "url" TEXT NOT NULL,

    CONSTRAINT "FrameworkEditorVideo_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "FrameworkEditorRequirement" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frk_rq'::text),
    "frameworkId" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,

    CONSTRAINT "FrameworkEditorRequirement_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "FrameworkEditorPolicyTemplate" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frk_pt'::text),
    "slug" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "frequency" "Frequency" NOT NULL,
    "department" "Departments" NOT NULL,
    "content" JSONB NOT NULL,

    CONSTRAINT "FrameworkEditorPolicyTemplate_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "FrameworkEditorTaskTemplate" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frk_tt'::text),
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "frequency" "Frequency" NOT NULL,
    "department" "Departments" NOT NULL,

    CONSTRAINT "FrameworkEditorTaskTemplate_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "FrameworkEditorControlTemplate" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frk_ct'::text),
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,

    CONSTRAINT "FrameworkEditorControlTemplate_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "_FrameworkEditorControlTemplateToFrameworkEditorPolicyTemplate" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorPolicyT_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateTable
CREATE TABLE "_FrameworkEditorControlTemplateToFrameworkEditorRequirement" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorRequire_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateTable
CREATE TABLE "_FrameworkEditorControlTemplateToFrameworkEditorTaskTemplate" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorTaskTem_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateIndex
CREATE UNIQUE INDEX "FrameworkEditorRequirement_frameworkId_name_key" ON "FrameworkEditorRequirement"("frameworkId", "name");

-- CreateIndex
CREATE UNIQUE INDEX "FrameworkEditorPolicyTemplate_slug_key" ON "FrameworkEditorPolicyTemplate"("slug");

-- CreateIndex
CREATE INDEX "_FrameworkEditorControlTemplateToFrameworkEditorPolicyT_B_index" ON "_FrameworkEditorControlTemplateToFrameworkEditorPolicyTemplate"("B");

-- CreateIndex
CREATE INDEX "_FrameworkEditorControlTemplateToFrameworkEditorRequire_B_index" ON "_FrameworkEditorControlTemplateToFrameworkEditorRequirement"("B");

-- CreateIndex
CREATE INDEX "_FrameworkEditorControlTemplateToFrameworkEditorTaskTem_B_index" ON "_FrameworkEditorControlTemplateToFrameworkEditorTaskTemplate"("B");

-- AddForeignKey
ALTER TABLE "FrameworkEditorRequirement" ADD CONSTRAINT "FrameworkEditorRequirement_frameworkId_fkey" FOREIGN KEY ("frameworkId") REFERENCES "FrameworkEditorFramework"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_FrameworkEditorControlTemplateToFrameworkEditorPolicyTemplate" ADD CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorPolicyTe_A_fkey" FOREIGN KEY ("A") REFERENCES "FrameworkEditorControlTemplate"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_FrameworkEditorControlTemplateToFrameworkEditorPolicyTemplate" ADD CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorPolicyTe_B_fkey" FOREIGN KEY ("B") REFERENCES "FrameworkEditorPolicyTemplate"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_FrameworkEditorControlTemplateToFrameworkEditorRequirement" ADD CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorRequirem_A_fkey" FOREIGN KEY ("A") REFERENCES "FrameworkEditorControlTemplate"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_FrameworkEditorControlTemplateToFrameworkEditorRequirement" ADD CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorRequirem_B_fkey" FOREIGN KEY ("B") REFERENCES "FrameworkEditorRequirement"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_FrameworkEditorControlTemplateToFrameworkEditorTaskTemplate" ADD CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorTaskTemp_A_fkey" FOREIGN KEY ("A") REFERENCES "FrameworkEditorControlTemplate"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_FrameworkEditorControlTemplateToFrameworkEditorTaskTemplate" ADD CONSTRAINT "_FrameworkEditorControlTemplateToFrameworkEditorTaskTemp_B_fkey" FOREIGN KEY ("B") REFERENCES "FrameworkEditorTaskTemplate"("id") ON DELETE CASCADE ON UPDATE CASCADE;
