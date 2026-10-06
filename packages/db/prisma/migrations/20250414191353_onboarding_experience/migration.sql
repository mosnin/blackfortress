-- CreateTable
CREATE TABLE "Onboarding" (
    "organizationId" TEXT NOT NULL,
    "policies" BOOLEAN NOT NULL DEFAULT false,
    "employees" BOOLEAN NOT NULL DEFAULT false,
    "vendors" BOOLEAN NOT NULL DEFAULT false,
    "integrations" BOOLEAN NOT NULL DEFAULT false,
    "risk" BOOLEAN NOT NULL DEFAULT false,
    "team" BOOLEAN NOT NULL DEFAULT false,

    CONSTRAINT "Onboarding_pkey" PRIMARY KEY ("organizationId")
);

-- CreateIndex
CREATE INDEX "Onboarding_organizationId_idx" ON "Onboarding"("organizationId");

-- AddForeignKey
ALTER TABLE "Onboarding" ADD CONSTRAINT "Onboarding_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- Insert onboarding records for existing organizations
INSERT INTO "Onboarding" ("organizationId")
SELECT
    id
FROM "Organization"
WHERE id NOT IN (SELECT "organizationId" FROM "Onboarding");