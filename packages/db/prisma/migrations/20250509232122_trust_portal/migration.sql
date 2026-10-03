-- CreateEnum
CREATE TYPE "TrustStatus" AS ENUM ('draft', 'published');

-- CreateTable
CREATE TABLE "Trust" (
    "organizationId" TEXT NOT NULL,
    "status" "TrustStatus" NOT NULL DEFAULT 'draft',
    "email" TEXT,
    "privacyPolicy" TEXT,
    "soc2" BOOLEAN NOT NULL DEFAULT false,
    "iso27001" BOOLEAN NOT NULL DEFAULT false,
    "gdpr" BOOLEAN NOT NULL DEFAULT false,

    CONSTRAINT "Trust_pkey" PRIMARY KEY ("status","organizationId")
);

-- CreateIndex
CREATE INDEX "Trust_organizationId_idx" ON "Trust"("organizationId");

-- CreateIndex
CREATE UNIQUE INDEX "Trust_organizationId_key" ON "Trust"("organizationId");

-- AddForeignKey
ALTER TABLE "Trust" ADD CONSTRAINT "Trust_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;
