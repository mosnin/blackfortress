-- CreateEnum
CREATE TYPE "FrameworkStatus" AS ENUM ('started', 'in_progress', 'compliant');

-- AlterTable
ALTER TABLE "Trust" ADD COLUMN     "gdpr_status" "FrameworkStatus" NOT NULL DEFAULT 'started',
ADD COLUMN     "iso27001_status" "FrameworkStatus" NOT NULL DEFAULT 'started',
ADD COLUMN     "soc2_status" "FrameworkStatus" NOT NULL DEFAULT 'started';
