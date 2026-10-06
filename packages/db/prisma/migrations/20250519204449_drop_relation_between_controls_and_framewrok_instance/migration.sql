/*
  Warnings:

  - You are about to drop the `_ControlToFrameworkInstance` table. If the table is not empty, all the data it contains will be lost.

*/
-- DropForeignKey
ALTER TABLE "_ControlToFrameworkInstance" DROP CONSTRAINT "_ControlToFrameworkInstance_A_fkey";

-- DropForeignKey
ALTER TABLE "_ControlToFrameworkInstance" DROP CONSTRAINT "_ControlToFrameworkInstance_B_fkey";

-- DropTable
DROP TABLE "_ControlToFrameworkInstance";
