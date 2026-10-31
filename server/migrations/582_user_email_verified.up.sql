-- P5-R1: an address an account merely typed at sign-up is not an identity.
--
-- Instances that allow open registration create the account with this column
-- false: the account can sign in with its local password, but the address must
-- not unlock email-addressed workspace invitations or settle an invitation
-- minted for that mailbox. Existing rows (email-code and Google logins, admin
-- provisioned accounts) were verified by the flow that created them, so the
-- default keeps them working without a data migration.
ALTER TABLE "user" ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT TRUE;
