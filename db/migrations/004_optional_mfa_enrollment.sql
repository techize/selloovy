ALTER TABLE public.owners
 ADD COLUMN mfa_enrollment_session_digest bytea CHECK (octet_length(mfa_enrollment_session_digest)=32),
 ADD COLUMN mfa_enrollment_expires_at timestamptz,
 ADD CONSTRAINT mfa_enrollment_pair CHECK ((mfa_enrollment_session_digest IS NULL) = (mfa_enrollment_expires_at IS NULL));
