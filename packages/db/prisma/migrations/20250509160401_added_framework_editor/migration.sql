-- CreateTable
CREATE TABLE "FrameworkEditorFramework" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frk'::text),
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,

    CONSTRAINT "FrameworkEditorFramework_pkey" PRIMARY KEY ("id")
);
