-- AlterTable
ALTER TABLE "Invitation" DROP COLUMN "role",
ADD COLUMN     "role" TEXT NOT NULL DEFAULT 'member';

-- AlterTable
ALTER TABLE "Member" ALTER COLUMN "role" TYPE TEXT USING role::TEXT;

-- Remove default value from Invitation role column
ALTER TABLE "Invitation" ALTER COLUMN "role" DROP DEFAULT;
