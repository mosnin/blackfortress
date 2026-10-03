-- CreateExtension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create function to generate prefixed CUID with sortable timestamp (compact)
CREATE OR REPLACE FUNCTION generate_prefixed_cuid(prefix text)
RETURNS text AS $$
DECLARE
    timestamp_hex text;
    random_hex text;
BEGIN
    -- Generate timestamp component (seconds since epoch) as hex
    timestamp_hex = LOWER(TO_HEX(EXTRACT(EPOCH FROM NOW())::BIGINT));

    -- Generate 8 random bytes and encode as hex (16 characters)
    -- Ensure we call the function from the correct schema if pgcrypto is installed elsewhere
    random_hex = encode(gen_random_bytes(8), 'hex');

    -- Combine prefix, timestamp, and random hex string
    RETURN prefix || '_' || timestamp_hex || random_hex;
END;
$$ LANGUAGE plpgsql;


-- CreateEnum
CREATE TYPE "ArtifactType" AS ENUM ('policy', 'evidence', 'procedure', 'training');

-- CreateEnum
CREATE TYPE "Role" AS ENUM ('owner', 'admin', 'auditor', 'employee');

-- CreateEnum
CREATE TYPE "EvidenceStatus" AS ENUM ('draft', 'published', 'not_relevant');

-- CreateEnum
CREATE TYPE "FrameworkId" AS ENUM ('soc2');

-- CreateEnum
CREATE TYPE "PolicyStatus" AS ENUM ('draft', 'published', 'needs_review', 'archived');

-- CreateEnum
CREATE TYPE "RequirementId" AS ENUM ('soc2_CC1', 'soc2_CC2', 'soc2_CC3', 'soc2_CC4', 'soc2_CC5', 'soc2_CC6', 'soc2_CC7', 'soc2_CC8', 'soc2_CC9', 'soc2_A1', 'soc2_C1', 'soc2_PI1', 'soc2_P1');

-- CreateEnum
CREATE TYPE "RiskTreatmentType" AS ENUM ('accept', 'avoid', 'mitigate', 'transfer');

-- CreateEnum
CREATE TYPE "RiskCategory" AS ENUM ('customer', 'governance', 'operations', 'other', 'people', 'regulatory', 'reporting', 'resilience', 'technology', 'vendor_management');

-- CreateEnum
CREATE TYPE "RiskStatus" AS ENUM ('open', 'pending', 'closed', 'archived');

-- CreateEnum
CREATE TYPE "Departments" AS ENUM ('none', 'admin', 'gov', 'hr', 'it', 'itsm', 'qms');

-- CreateEnum
CREATE TYPE "Frequency" AS ENUM ('monthly', 'quarterly', 'yearly');

-- CreateEnum
CREATE TYPE "Likelihood" AS ENUM ('very_unlikely', 'unlikely', 'possible', 'likely', 'very_likely');

-- CreateEnum
CREATE TYPE "Impact" AS ENUM ('insignificant', 'minor', 'moderate', 'major', 'severe');

-- CreateEnum
CREATE TYPE "TaskStatus" AS ENUM ('open', 'closed');

-- CreateEnum
CREATE TYPE "TaskType" AS ENUM ('vendor', 'risk');

-- CreateEnum
CREATE TYPE "VendorCategory" AS ENUM ('cloud', 'infrastructure', 'software_as_a_service', 'finance', 'marketing', 'sales', 'hr', 'other');

-- CreateEnum
CREATE TYPE "VendorStatus" AS ENUM ('not_assessed', 'in_progress', 'assessed');

-- CreateTable
CREATE TABLE "Artifact" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('art'::text),
    "type" "ArtifactType" NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "organizationId" TEXT NOT NULL,
    "evidenceId" TEXT,
    "policyId" TEXT,

    CONSTRAINT "Artifact_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "User" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('usr'::text),
    "name" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "emailVerified" BOOLEAN NOT NULL,
    "image" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "lastLogin" TIMESTAMP(3),

    CONSTRAINT "User_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "EmployeeTrainingVideoCompletion" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('evc'::text),
    "completedAt" TIMESTAMP(3),
    "videoId" TEXT NOT NULL,
    "memberId" TEXT NOT NULL,

    CONSTRAINT "EmployeeTrainingVideoCompletion_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Session" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('ses'::text),
    "expiresAt" TIMESTAMP(3) NOT NULL,
    "token" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "ipAddress" TEXT,
    "userAgent" TEXT,
    "userId" TEXT NOT NULL,
    "activeOrganizationId" TEXT,

    CONSTRAINT "Session_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Account" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('acc'::text),
    "accountId" TEXT NOT NULL,
    "providerId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "accessToken" TEXT,
    "refreshToken" TEXT,
    "idToken" TEXT,
    "accessTokenExpiresAt" TIMESTAMP(3),
    "refreshTokenExpiresAt" TIMESTAMP(3),
    "scope" TEXT,
    "password" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Account_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Verification" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('ver'::text),
    "identifier" TEXT NOT NULL,
    "value" TEXT NOT NULL,
    "expiresAt" TIMESTAMP(3) NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Verification_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Member" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('mem'::text),
    "organizationId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "role" "Role" NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "department" "Departments" NOT NULL DEFAULT 'none',
    "isActive" BOOLEAN NOT NULL DEFAULT true,

    CONSTRAINT "Member_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Invitation" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('inv'::text),
    "organizationId" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "role" "Role",
    "status" TEXT NOT NULL,
    "expiresAt" TIMESTAMP(3) NOT NULL,
    "inviterId" TEXT NOT NULL,

    CONSTRAINT "Invitation_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Control" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('ctl'::text),
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "lastReviewDate" TIMESTAMP(3),
    "nextReviewDate" TIMESTAMP(3),
    "organizationId" TEXT NOT NULL,

    CONSTRAINT "Control_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Evidence" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('evd'::text),
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "published" BOOLEAN NOT NULL DEFAULT false,
    "isNotRelevant" BOOLEAN NOT NULL DEFAULT false,
    "additionalUrls" TEXT[],
    "fileUrls" TEXT[],
    "frequency" "Frequency",
    "department" "Departments" NOT NULL DEFAULT 'none',
    "status" "EvidenceStatus" DEFAULT 'draft',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "lastPublishedAt" TIMESTAMP(3),
    "assigneeId" TEXT,
    "organizationId" TEXT NOT NULL,

    CONSTRAINT "Evidence_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "FrameworkInstance" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('frm'::text),
    "organizationId" TEXT NOT NULL,
    "frameworkId" "FrameworkId" NOT NULL,

    CONSTRAINT "FrameworkInstance_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Integration" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('int'::text),
    "name" TEXT NOT NULL,
    "integrationId" TEXT NOT NULL,
    "settings" JSONB NOT NULL,
    "userSettings" JSONB NOT NULL,
    "organizationId" TEXT NOT NULL,
    "lastRunAt" TIMESTAMP(3),

    CONSTRAINT "Integration_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "IntegrationResult" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('itr'::text),
    "title" TEXT,
    "description" TEXT,
    "remediation" TEXT,
    "status" TEXT,
    "severity" TEXT,
    "resultDetails" JSONB,
    "completedAt" TIMESTAMP(3) DEFAULT CURRENT_TIMESTAMP,
    "integrationId" TEXT NOT NULL,
    "organizationId" TEXT NOT NULL,
    "assignedUserId" TEXT,

    CONSTRAINT "IntegrationResult_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Organization" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('org'::text),
    "name" TEXT NOT NULL,
    "slug" TEXT NOT NULL,
    "logo" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL,
    "metadata" TEXT,

    CONSTRAINT "Organization_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Policy" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('pol'::text),
    "name" TEXT NOT NULL,
    "description" TEXT,
    "organizationId" TEXT NOT NULL,
    "status" "PolicyStatus" NOT NULL DEFAULT 'draft',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "content" JSONB[],
    "frequency" "Frequency",
    "lastPublishedAt" TIMESTAMP(3),
    "isRequiredToSign" BOOLEAN NOT NULL DEFAULT false,
    "signedBy" TEXT[] DEFAULT ARRAY[]::TEXT[],
    "ownerId" TEXT,
    "department" "Departments",
    "reviewDate" TIMESTAMP(3),

    CONSTRAINT "Policy_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "RequirementMap" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('req'::text),
    "requirementId" "RequirementId" NOT NULL,
    "controlId" TEXT NOT NULL,
    "frameworkInstanceId" TEXT NOT NULL,

    CONSTRAINT "RequirementMap_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Risk" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('rsk'::text),
    "title" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "category" "RiskCategory" NOT NULL,
    "department" "Departments",
    "status" "RiskStatus" NOT NULL DEFAULT 'open',
    "likelihood" "Likelihood" NOT NULL DEFAULT 'very_unlikely',
    "impact" "Impact" NOT NULL DEFAULT 'insignificant',
    "residualLikelihood" "Likelihood" NOT NULL DEFAULT 'very_unlikely',
    "residualImpact" "Impact" NOT NULL DEFAULT 'insignificant',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "treatmentStrategyDescription" TEXT,
    "treatmentStrategy" "RiskTreatmentType" NOT NULL DEFAULT 'accept',
    "organizationId" TEXT NOT NULL,
    "ownerId" TEXT,

    CONSTRAINT "Risk_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "ApiKey" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('apk'::text),
    "name" TEXT NOT NULL,
    "key" TEXT NOT NULL,
    "salt" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "expiresAt" TIMESTAMP(3),
    "lastUsedAt" TIMESTAMP(3),
    "isActive" BOOLEAN NOT NULL DEFAULT true,
    "organizationId" TEXT NOT NULL,

    CONSTRAINT "ApiKey_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "AuditLog" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('aud'::text),
    "timestamp" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "userId" TEXT NOT NULL,
    "organizationId" TEXT NOT NULL,
    "data" JSONB NOT NULL,

    CONSTRAINT "AuditLog_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Task" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('tsk'::text),
    "title" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "dueDate" TIMESTAMP(3) NOT NULL,
    "status" "TaskStatus" NOT NULL DEFAULT 'open',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "relatedId" TEXT NOT NULL,
    "relatedType" "TaskType" NOT NULL,
    "userId" TEXT NOT NULL,
    "organizationId" TEXT NOT NULL,

    CONSTRAINT "Task_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Vendor" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('vnd'::text),
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "category" "VendorCategory" NOT NULL DEFAULT 'other',
    "status" "VendorStatus" NOT NULL DEFAULT 'not_assessed',
    "inherentProbability" "Likelihood" NOT NULL DEFAULT 'very_unlikely',
    "inherentImpact" "Impact" NOT NULL DEFAULT 'insignificant',
    "residualProbability" "Likelihood" NOT NULL DEFAULT 'very_unlikely',
    "residualImpact" "Impact" NOT NULL DEFAULT 'insignificant',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "organizationId" TEXT NOT NULL,
    "ownerId" TEXT,

    CONSTRAINT "Vendor_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "VendorContact" (
    "id" TEXT NOT NULL DEFAULT generate_prefixed_cuid('vct'::text),
    "vendorId" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "phone" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "VendorContact_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "_ArtifactToControl" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_ArtifactToControl_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateTable
CREATE TABLE "_ControlToFrameworkInstance" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_ControlToFrameworkInstance_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateIndex
CREATE UNIQUE INDEX "User_email_key" ON "User"("email");

-- CreateIndex
CREATE INDEX "EmployeeTrainingVideoCompletion_memberId_idx" ON "EmployeeTrainingVideoCompletion"("memberId");

-- CreateIndex
CREATE UNIQUE INDEX "EmployeeTrainingVideoCompletion_memberId_videoId_key" ON "EmployeeTrainingVideoCompletion"("memberId", "videoId");

-- CreateIndex
CREATE UNIQUE INDEX "Session_token_key" ON "Session"("token");

-- CreateIndex
CREATE INDEX "Control_organizationId_idx" ON "Control"("organizationId");

-- CreateIndex
CREATE INDEX "Evidence_organizationId_idx" ON "Evidence"("organizationId");

-- CreateIndex
CREATE UNIQUE INDEX "FrameworkInstance_organizationId_frameworkId_key" ON "FrameworkInstance"("organizationId", "frameworkId");

-- CreateIndex
CREATE UNIQUE INDEX "Integration_name_key" ON "Integration"("name");

-- CreateIndex
CREATE INDEX "Integration_organizationId_idx" ON "Integration"("organizationId");

-- CreateIndex
CREATE INDEX "IntegrationResult_integrationId_idx" ON "IntegrationResult"("integrationId");

-- CreateIndex
CREATE UNIQUE INDEX "Organization_slug_key" ON "Organization"("slug");

-- CreateIndex
CREATE INDEX "Organization_slug_idx" ON "Organization"("slug");

-- CreateIndex
CREATE INDEX "Policy_organizationId_idx" ON "Policy"("organizationId");

-- CreateIndex
CREATE INDEX "RequirementMap_requirementId_frameworkInstanceId_idx" ON "RequirementMap"("requirementId", "frameworkInstanceId");

-- CreateIndex
CREATE UNIQUE INDEX "RequirementMap_controlId_frameworkInstanceId_requirementId_key" ON "RequirementMap"("controlId", "frameworkInstanceId", "requirementId");

-- CreateIndex
CREATE INDEX "Risk_organizationId_idx" ON "Risk"("organizationId");

-- CreateIndex
CREATE INDEX "Risk_ownerId_idx" ON "Risk"("ownerId");

-- CreateIndex
CREATE INDEX "Risk_category_idx" ON "Risk"("category");

-- CreateIndex
CREATE INDEX "Risk_status_idx" ON "Risk"("status");

-- CreateIndex
CREATE UNIQUE INDEX "ApiKey_key_key" ON "ApiKey"("key");

-- CreateIndex
CREATE INDEX "ApiKey_organizationId_idx" ON "ApiKey"("organizationId");

-- CreateIndex
CREATE INDEX "ApiKey_key_idx" ON "ApiKey"("key");

-- CreateIndex
CREATE INDEX "AuditLog_userId_idx" ON "AuditLog"("userId");

-- CreateIndex
CREATE INDEX "AuditLog_organizationId_idx" ON "AuditLog"("organizationId");

-- CreateIndex
CREATE INDEX "Task_relatedId_idx" ON "Task"("relatedId");

-- CreateIndex
CREATE INDEX "Task_relatedId_organizationId_idx" ON "Task"("relatedId", "organizationId");

-- CreateIndex
CREATE INDEX "Vendor_organizationId_idx" ON "Vendor"("organizationId");

-- CreateIndex
CREATE INDEX "Vendor_ownerId_idx" ON "Vendor"("ownerId");

-- CreateIndex
CREATE INDEX "Vendor_category_idx" ON "Vendor"("category");

-- CreateIndex
CREATE INDEX "VendorContact_vendorId_idx" ON "VendorContact"("vendorId");

-- CreateIndex
CREATE INDEX "_ArtifactToControl_B_index" ON "_ArtifactToControl"("B");

-- CreateIndex
CREATE INDEX "_ControlToFrameworkInstance_B_index" ON "_ControlToFrameworkInstance"("B");

-- AddForeignKey
ALTER TABLE "Artifact" ADD CONSTRAINT "Artifact_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Artifact" ADD CONSTRAINT "Artifact_evidenceId_fkey" FOREIGN KEY ("evidenceId") REFERENCES "Evidence"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Artifact" ADD CONSTRAINT "Artifact_policyId_fkey" FOREIGN KEY ("policyId") REFERENCES "Policy"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "EmployeeTrainingVideoCompletion" ADD CONSTRAINT "EmployeeTrainingVideoCompletion_memberId_fkey" FOREIGN KEY ("memberId") REFERENCES "Member"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Session" ADD CONSTRAINT "Session_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Account" ADD CONSTRAINT "Account_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Member" ADD CONSTRAINT "Member_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Member" ADD CONSTRAINT "Member_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Invitation" ADD CONSTRAINT "Invitation_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Invitation" ADD CONSTRAINT "Invitation_inviterId_fkey" FOREIGN KEY ("inviterId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Control" ADD CONSTRAINT "Control_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Evidence" ADD CONSTRAINT "Evidence_assigneeId_fkey" FOREIGN KEY ("assigneeId") REFERENCES "Member"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Evidence" ADD CONSTRAINT "Evidence_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "FrameworkInstance" ADD CONSTRAINT "FrameworkInstance_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Integration" ADD CONSTRAINT "Integration_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "IntegrationResult" ADD CONSTRAINT "IntegrationResult_assignedUserId_fkey" FOREIGN KEY ("assignedUserId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "IntegrationResult" ADD CONSTRAINT "IntegrationResult_integrationId_fkey" FOREIGN KEY ("integrationId") REFERENCES "Integration"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Policy" ADD CONSTRAINT "Policy_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Policy" ADD CONSTRAINT "Policy_ownerId_fkey" FOREIGN KEY ("ownerId") REFERENCES "User"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "RequirementMap" ADD CONSTRAINT "RequirementMap_controlId_fkey" FOREIGN KEY ("controlId") REFERENCES "Control"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "RequirementMap" ADD CONSTRAINT "RequirementMap_frameworkInstanceId_fkey" FOREIGN KEY ("frameworkInstanceId") REFERENCES "FrameworkInstance"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Risk" ADD CONSTRAINT "Risk_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Risk" ADD CONSTRAINT "Risk_ownerId_fkey" FOREIGN KEY ("ownerId") REFERENCES "User"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ApiKey" ADD CONSTRAINT "ApiKey_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "AuditLog" ADD CONSTRAINT "AuditLog_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "AuditLog" ADD CONSTRAINT "AuditLog_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Task" ADD CONSTRAINT "Task_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Task" ADD CONSTRAINT "Task_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Vendor" ADD CONSTRAINT "Vendor_organizationId_fkey" FOREIGN KEY ("organizationId") REFERENCES "Organization"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Vendor" ADD CONSTRAINT "Vendor_ownerId_fkey" FOREIGN KEY ("ownerId") REFERENCES "User"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "VendorContact" ADD CONSTRAINT "VendorContact_vendorId_fkey" FOREIGN KEY ("vendorId") REFERENCES "Vendor"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_ArtifactToControl" ADD CONSTRAINT "_ArtifactToControl_A_fkey" FOREIGN KEY ("A") REFERENCES "Artifact"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_ArtifactToControl" ADD CONSTRAINT "_ArtifactToControl_B_fkey" FOREIGN KEY ("B") REFERENCES "Control"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_ControlToFrameworkInstance" ADD CONSTRAINT "_ControlToFrameworkInstance_A_fkey" FOREIGN KEY ("A") REFERENCES "Control"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_ControlToFrameworkInstance" ADD CONSTRAINT "_ControlToFrameworkInstance_B_fkey" FOREIGN KEY ("B") REFERENCES "FrameworkInstance"("id") ON DELETE CASCADE ON UPDATE CASCADE;
