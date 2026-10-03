/*
  Warnings:

  - You are about to drop the column `evidence` on the `Onboarding` table. All the data in the column will be lost.

*/
-- AlterTable
ALTER TABLE "Onboarding" DROP COLUMN "evidence",
ADD COLUMN     "tasks" BOOLEAN NOT NULL DEFAULT false;
