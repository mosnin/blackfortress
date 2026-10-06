-- CreateTable
CREATE TABLE "GlobalVendors" (
    "website" TEXT NOT NULL,
    "company_name" TEXT,
    "legal_name" TEXT,
    "company_description" TEXT,
    "company_hq_address" TEXT,
    "privacy_policy_url" TEXT,
    "terms_of_service_url" TEXT,
    "service_level_agreement_url" TEXT,
    "security_page_url" TEXT,
    "trust_page_url" TEXT,
    "security_certifications" TEXT[],
    "subprocessors" TEXT[],
    "type_of_company" TEXT,
    "approved" BOOLEAN NOT NULL DEFAULT false,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "GlobalVendors_pkey" PRIMARY KEY ("website")
);

-- CreateIndex
CREATE UNIQUE INDEX "GlobalVendors_website_key" ON "GlobalVendors"("website");

-- CreateIndex
CREATE INDEX "GlobalVendors_website_idx" ON "GlobalVendors"("website");
